package engine

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SSTable represents a sorted string table (immutable on-disk segment)
type SSTable struct {
	ID       int64
	FilePath string
	Index    map[string]int64 // sparse index: key -> file offset
	MinKey   string
	MaxKey   string
	Size     int64
}

// SSTManager manages multiple SST files
type SSTManager struct {
	mu       sync.RWMutex
	sstables []*SSTable
	dataDir  string
	nextID   int64
}

// NewSSTManager creates a new SST manager
func NewSSTManager(dataDir string) (*SSTManager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	manager := &SSTManager{
		sstables: make([]*SSTable, 0),
		dataDir:  dataDir,
		nextID:   1,
	}

	// Load existing SST files
	if err := manager.loadExistingSSTables(); err != nil {
		return nil, err
	}

	return manager, nil
}

// loadExistingSSTables scans the data directory for existing SST files
func (sm *SSTManager) loadExistingSSTables() error {
	files, err := os.ReadDir(sm.dataDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sst") {
			path := filepath.Join(sm.dataDir, file.Name())
			sst, err := sm.loadSSTable(path)
			if err != nil {
				return fmt.Errorf("failed to load SST %s: %w", path, err)
			}
			sm.sstables = append(sm.sstables, sst)
			if sst.ID >= sm.nextID {
				sm.nextID = sst.ID + 1
			}
		}
	}

	// Sort by ID (newest first for reads)
	sort.Slice(sm.sstables, func(i, j int) bool {
		return sm.sstables[i].ID > sm.sstables[j].ID
	})

	return nil
}

// loadSSTable loads an SST file and builds its index
func (sm *SSTManager) loadSSTable(path string) (*SSTable, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Parse ID from filename (e.g., "000001.sst")
	name := filepath.Base(path)
	var id int64
	fmt.Sscanf(name, "%d.sst", &id)

	sst := &SSTable{
		ID:       id,
		FilePath: path,
		Index:    make(map[string]int64),
		Size:     stat.Size(),
	}

	// Build sparse index by reading the file
	reader := bufio.NewReader(file)
	var offset int64
	var firstKey, lastKey string
	entryCount := 0

	for {
		startOffset := offset

		// Read entry: timestamp(8) + deleted(1) + keyLen(4) + key + valueLen(4) + value
		var timestamp int64
		if err := binary.Read(reader, binary.LittleEndian, &timestamp); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		offset += 8

		// Read deleted flag (not used in index building)
		_, err = reader.ReadByte()
		if err != nil {
			return nil, err
		}
		offset += 1

		var keyLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &keyLen); err != nil {
			return nil, err
		}
		offset += 4

		keyBytes := make([]byte, keyLen)
		if _, err := io.ReadFull(reader, keyBytes); err != nil {
			return nil, err
		}
		offset += int64(keyLen)
		key := string(keyBytes)

		var valueLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &valueLen); err != nil {
			return nil, err
		}
		offset += 4

		if _, err := reader.Discard(int(valueLen)); err != nil {
			return nil, err
		}
		offset += int64(valueLen)

		// Track first and last keys
		if entryCount == 0 {
			firstKey = key
		}
		lastKey = key

		// Add to sparse index (every 10th entry or so)
		if entryCount%10 == 0 {
			sst.Index[key] = startOffset
		}

		entryCount++
	}

	sst.MinKey = firstKey
	sst.MaxKey = lastKey

	return sst, nil
}

// Flush writes a memtable to a new SST file
func (sm *SSTManager) Flush(entries []*Entry) error {
	if len(entries) == 0 {
		return nil
	}

	sm.mu.Lock()
	id := sm.nextID
	sm.nextID++
	sm.mu.Unlock()

	// Sort entries by key
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	filename := fmt.Sprintf("%06d.sst", id)
	path := filepath.Join(sm.dataDir, filename)

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	sst := &SSTable{
		ID:       id,
		FilePath: path,
		Index:    make(map[string]int64),
		MinKey:   entries[0].Key,
		MaxKey:   entries[len(entries)-1].Key,
	}

	var offset int64
	for i, entry := range entries {
		startOffset := offset

		// Write: timestamp(8) + deleted(1) + keyLen(4) + key + valueLen(4) + value
		if err := binary.Write(writer, binary.LittleEndian, entry.Timestamp); err != nil {
			return err
		}
		offset += 8

		deleted := byte(0)
		if entry.Deleted {
			deleted = 1
		}
		if err := writer.WriteByte(deleted); err != nil {
			return err
		}
		offset += 1

		keyLen := uint32(len(entry.Key))
		if err := binary.Write(writer, binary.LittleEndian, keyLen); err != nil {
			return err
		}
		offset += 4

		if _, err := writer.Write([]byte(entry.Key)); err != nil {
			return err
		}
		offset += int64(keyLen)

		valueLen := uint32(len(entry.Value))
		if err := binary.Write(writer, binary.LittleEndian, valueLen); err != nil {
			return err
		}
		offset += 4

		if _, err := writer.Write(entry.Value); err != nil {
			return err
		}
		offset += int64(valueLen)

		// Sparse index
		if i%10 == 0 {
			sst.Index[entry.Key] = startOffset
		}
	}

	sst.Size = offset

	sm.mu.Lock()
	sm.sstables = append([]*SSTable{sst}, sm.sstables...)
	sm.mu.Unlock()

	return nil
}

// Get searches for a key across all SST files (newest first)
func (sm *SSTManager) Get(key string) ([]byte, bool, error) {
	sm.mu.RLock()
	sstables := make([]*SSTable, len(sm.sstables))
	copy(sstables, sm.sstables)
	sm.mu.RUnlock()

	for _, sst := range sstables {
		// Check if key is in range
		if key < sst.MinKey || key > sst.MaxKey {
			continue
		}

		value, found, err := sm.getFromSST(sst, key)
		if err != nil {
			return nil, false, err
		}
		if found {
			return value, true, nil
		}
	}

	return nil, false, nil
}

// getFromSST searches for a key in a specific SST file
func (sm *SSTManager) getFromSST(sst *SSTable, key string) ([]byte, bool, error) {
	file, err := os.Open(sst.FilePath)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	// Find starting offset from sparse index
	var startOffset int64
	for indexKey, offset := range sst.Index {
		if indexKey <= key {
			if offset > startOffset {
				startOffset = offset
			}
		}
	}

	if _, err := file.Seek(startOffset, 0); err != nil {
		return nil, false, err
	}

	reader := bufio.NewReader(file)

	// Scan from startOffset
	for {
		var timestamp int64
		if err := binary.Read(reader, binary.LittleEndian, &timestamp); err != nil {
			if err == io.EOF {
				break
			}
			return nil, false, err
		}

		var deleted byte
		deleted, err = reader.ReadByte()
		if err != nil {
			return nil, false, err
		}

		var keyLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &keyLen); err != nil {
			return nil, false, err
		}

		keyBytes := make([]byte, keyLen)
		if _, err := io.ReadFull(reader, keyBytes); err != nil {
			return nil, false, err
		}
		entryKey := string(keyBytes)

		var valueLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &valueLen); err != nil {
			return nil, false, err
		}

		valueBytes := make([]byte, valueLen)
		if _, err := io.ReadFull(reader, valueBytes); err != nil {
			return nil, false, err
		}

		if entryKey == key {
			if deleted == 1 {
				return nil, false, nil // tombstone
			}
			return valueBytes, true, nil
		}

		if entryKey > key {
			break // passed the key
		}
	}

	return nil, false, nil
}

// GetAllSSTables returns a copy of all SST files
func (sm *SSTManager) GetAllSSTables() []*SSTable {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sstables := make([]*SSTable, len(sm.sstables))
	copy(sstables, sm.sstables)
	return sstables
}

// RemoveSSTable removes an SST file from the manager and deletes it
func (sm *SSTManager) RemoveSSTable(sst *SSTable) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, s := range sm.sstables {
		if s.ID == sst.ID {
			sm.sstables = append(sm.sstables[:i], sm.sstables[i+1:]...)
			return os.Remove(sst.FilePath)
		}
	}
	return nil
}
