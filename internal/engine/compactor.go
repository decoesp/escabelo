package engine

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"
)

// Compactor handles background compaction of SST files
type Compactor struct {
	sstManager *SSTManager
	interval   time.Duration
	stopCh     chan struct{}
}

// NewCompactor creates a new compactor
func NewCompactor(sstManager *SSTManager, interval time.Duration) *Compactor {
	return &Compactor{
		sstManager: sstManager,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the background compaction process
func (c *Compactor) Start() {
	go c.run()
}

// Stop stops the compaction process
func (c *Compactor) Stop() {
	close(c.stopCh)
}

// run is the main compaction loop
func (c *Compactor) run() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.compact(); err != nil {
				log.Printf("Compaction error: %v", err)
			}
		case <-c.stopCh:
			return
		}
	}
}

// compact performs a compaction cycle
func (c *Compactor) compact() error {
	sstables := c.sstManager.GetAllSSTables()

	// Simple strategy: merge oldest SSTs if we have more than 4
	if len(sstables) <= 4 {
		return nil
	}

	// Take the 4 oldest SSTs
	toMerge := sstables[len(sstables)-4:]

	log.Printf("Compacting %d SST files...", len(toMerge))

	// Merge entries
	mergedEntries, err := c.mergeSSTs(toMerge)
	if err != nil {
		return fmt.Errorf("merge failed: %w", err)
	}

	// Write merged SST
	if err := c.sstManager.Flush(mergedEntries); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	// Remove old SSTs
	for _, sst := range toMerge {
		if err := c.sstManager.RemoveSSTable(sst); err != nil {
			log.Printf("Failed to remove SST %d: %v", sst.ID, err)
		}
	}

	log.Printf("Compaction complete: merged %d files into 1", len(toMerge))
	return nil
}

// mergeSSTs merges multiple SST files, keeping the newest version of each key
func (c *Compactor) mergeSSTs(sstables []*SSTable) ([]*Entry, error) {
	// Map to hold the latest entry for each key
	entryMap := make(map[string]*Entry)

	for _, sst := range sstables {
		entries, err := c.readAllEntries(sst)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			existing, exists := entryMap[entry.Key]
			if !exists || entry.Timestamp > existing.Timestamp {
				entryMap[entry.Key] = entry
			}
		}
	}

	// Convert map to slice and remove tombstones
	var result []*Entry
	for _, entry := range entryMap {
		if !entry.Deleted {
			result = append(result, entry)
		}
	}

	// Sort by key
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})

	return result, nil
}

// readAllEntries reads all entries from an SST file
func (c *Compactor) readAllEntries(sst *SSTable) ([]*Entry, error) {
	file, err := os.Open(sst.FilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var entries []*Entry

	for {
		var timestamp int64
		if err := binary.Read(reader, binary.LittleEndian, &timestamp); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		var deleted byte
		deleted, err = reader.ReadByte()
		if err != nil {
			return nil, err
		}

		var keyLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &keyLen); err != nil {
			return nil, err
		}

		keyBytes := make([]byte, keyLen)
		if _, err := io.ReadFull(reader, keyBytes); err != nil {
			return nil, err
		}

		var valueLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &valueLen); err != nil {
			return nil, err
		}

		valueBytes := make([]byte, valueLen)
		if _, err := io.ReadFull(reader, valueBytes); err != nil {
			return nil, err
		}

		entry := &Entry{
			Key:       string(keyBytes),
			Value:     valueBytes,
			Timestamp: timestamp,
			Deleted:   deleted == 1,
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
