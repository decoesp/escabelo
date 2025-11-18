package engine

import (
	"fmt"
	"sync"
	"time"
)

// Config holds engine configuration
type Config struct {
	DataDir            string
	MemTableMaxSize    int64
	CompactionInterval time.Duration
	WALSyncInterval    time.Duration
}

// Engine is the main LSM-tree storage engine
type Engine struct {
	mu sync.RWMutex

	// Active memtable
	memtable *MemTable

	// Immutable memtables waiting to be flushed
	immutableMemtables []*MemTable

	// SST manager
	sstManager *SSTManager

	// WAL for durability
	wal *WAL

	// Background compactor
	compactor *Compactor

	// Configuration
	config Config

	// Flush channel
	flushCh chan struct{}
	stopCh  chan struct{}

	// Stats
	stats *Stats
}

// Stats holds engine statistics
type Stats struct {
	mu            sync.RWMutex
	Writes        int64
	Reads         int64
	Deletes       int64
	Flushes       int64
	Compactions   int64
	MemTableSize  int64
	SSTCount      int64
	WALSize       int64
	TotalDataSize int64
}

// NewEngine creates a new storage engine
func NewEngine(config Config) (*Engine, error) {
	// Create WAL
	wal, err := NewWAL(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create WAL: %w", err)
	}

	// Create SST manager
	sstManager, err := NewSSTManager(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create SST manager: %w", err)
	}

	// Create engine
	engine := &Engine{
		memtable:           NewMemTable(config.MemTableMaxSize),
		immutableMemtables: make([]*MemTable, 0),
		sstManager:         sstManager,
		wal:                wal,
		config:             config,
		flushCh:            make(chan struct{}, 1),
		stopCh:             make(chan struct{}),
		stats:              &Stats{},
	}

	// Recover from WAL
	if err := engine.recover(); err != nil {
		return nil, fmt.Errorf("recovery failed: %w", err)
	}

	// Start background workers
	engine.compactor = NewCompactor(sstManager, config.CompactionInterval)
	engine.compactor.Start()

	go engine.flusher()
	go engine.walSyncer()

	return engine, nil
}

// recover replays the WAL to restore state
func (e *Engine) recover() error {
	entries, err := e.wal.Replay()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		switch entry.OpType {
		case OpTypePut:
			e.memtable.Put(entry.Key, entry.Value)
		case OpTypeDelete:
			e.memtable.Delete(entry.Key)
		}
	}

	return nil
}

// Put writes a key-value pair
func (e *Engine) Put(key string, value []byte) error {
	// Validate key size (max 100KB)
	if len(key) > 100*1024 {
		return fmt.Errorf("key too large: %d bytes (max 100KB)", len(key))
	}

	// Write to WAL first (durability)
	walEntry := &WALEntry{
		OpType:    OpTypePut,
		Key:       key,
		Value:     value,
		Timestamp: time.Now().UnixNano(),
	}
	if err := e.wal.Append(walEntry); err != nil {
		return fmt.Errorf("WAL append failed: %w", err)
	}

	// Write to memtable
	e.mu.Lock()
	e.memtable.Put(key, value)
	e.stats.mu.Lock()
	e.stats.Writes++
	e.stats.MemTableSize = e.memtable.Size()
	e.stats.mu.Unlock()

	// Check if memtable is full
	if e.memtable.IsFull() {
		e.rotateMemTable()
	}
	e.mu.Unlock()

	return nil
}

// Get retrieves a value by key
func (e *Engine) Get(key string) ([]byte, bool, error) {
	e.stats.mu.Lock()
	e.stats.Reads++
	e.stats.mu.Unlock()

	// Check active memtable
	e.mu.RLock()
	if value, found := e.memtable.Get(key); found {
		e.mu.RUnlock()
		return value, true, nil
	}

	// Check immutable memtables
	for _, mt := range e.immutableMemtables {
		if value, found := mt.Get(key); found {
			e.mu.RUnlock()
			return value, true, nil
		}
	}
	e.mu.RUnlock()

	// Check SST files
	value, found, err := e.sstManager.Get(key)
	if err != nil {
		return nil, false, fmt.Errorf("SST lookup failed: %w", err)
	}

	return value, found, nil
}

// Delete removes a key
func (e *Engine) Delete(key string) (bool, error) {
	// Check if key exists
	_, exists, err := e.Get(key)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	// Write to WAL
	walEntry := &WALEntry{
		OpType:    OpTypeDelete,
		Key:       key,
		Timestamp: time.Now().UnixNano(),
	}
	if err := e.wal.Append(walEntry); err != nil {
		return false, fmt.Errorf("WAL append failed: %w", err)
	}

	// Write tombstone to memtable
	e.mu.Lock()
	deleted := e.memtable.Delete(key)
	e.stats.mu.Lock()
	e.stats.Deletes++
	e.stats.mu.Unlock()

	if e.memtable.IsFull() {
		e.rotateMemTable()
	}
	e.mu.Unlock()

	return deleted, nil
}

// Keys returns all keys
func (e *Engine) Keys() ([]string, error) {
	keySet := make(map[string]bool)

	// Get from memtable
	e.mu.RLock()
	for _, key := range e.memtable.Keys() {
		keySet[key] = true
	}

	// Get from immutable memtables
	for _, mt := range e.immutableMemtables {
		for _, key := range mt.Keys() {
			keySet[key] = true
		}
	}
	e.mu.RUnlock()

	// Get from SST files (simplified - would need full scan)
	// For now, just return memtable keys

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}

	return keys, nil
}

// PrefixScan returns all values with keys starting with prefix
func (e *Engine) PrefixScan(prefix string) ([][]byte, error) {
	valueMap := make(map[string][]byte)

	// Get from memtable
	e.mu.RLock()
	for _, value := range e.memtable.PrefixScan(prefix) {
		// Store with a unique identifier
		valueMap[string(value)] = value
	}

	// Get from immutable memtables
	for _, mt := range e.immutableMemtables {
		for _, value := range mt.PrefixScan(prefix) {
			valueMap[string(value)] = value
		}
	}
	e.mu.RUnlock()

	// Convert to slice
	values := make([][]byte, 0, len(valueMap))
	for _, value := range valueMap {
		values = append(values, value)
	}

	return values, nil
}

// rotateMemTable moves the current memtable to immutable list
func (e *Engine) rotateMemTable() {
	e.immutableMemtables = append(e.immutableMemtables, e.memtable)
	e.memtable = NewMemTable(e.config.MemTableMaxSize)

	// Trigger flush
	select {
	case e.flushCh <- struct{}{}:
	default:
	}
}

// flusher handles background flushing of immutable memtables
func (e *Engine) flusher() {
	for {
		select {
		case <-e.flushCh:
			e.flush()
		case <-e.stopCh:
			return
		}
	}
}

// flush writes immutable memtables to SST files
func (e *Engine) flush() {
	e.mu.Lock()
	if len(e.immutableMemtables) == 0 {
		e.mu.Unlock()
		return
	}

	// Take the oldest immutable memtable
	mt := e.immutableMemtables[0]
	e.immutableMemtables = e.immutableMemtables[1:]
	shouldTruncateWAL := len(e.immutableMemtables) == 0
	e.mu.Unlock()

	// Flush to SST
	entries := mt.Entries()
	if err := e.sstManager.Flush(entries); err != nil {
		fmt.Printf("Flush failed: %v\n", err)
		return
	}

	// Only truncate WAL when all immutable memtables have been flushed
	// This prevents data loss if server crashes while flushing
	if shouldTruncateWAL {
		if err := e.wal.Truncate(); err != nil {
			fmt.Printf("WAL truncate failed: %v\n", err)
		}
	}

	e.stats.mu.Lock()
	e.stats.Flushes++
	e.stats.SSTCount = int64(len(e.sstManager.GetAllSSTables()))
	e.stats.mu.Unlock()
}

// walSyncer periodically syncs WAL to disk
func (e *Engine) walSyncer() {
	ticker := time.NewTicker(e.config.WALSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := e.wal.Sync(); err != nil {
				fmt.Printf("WAL sync failed: %v\n", err)
			}
		case <-e.stopCh:
			return
		}
	}
}

// GetStats returns current engine statistics
func (e *Engine) GetStats() Stats {
	e.stats.mu.RLock()
	defer e.stats.mu.RUnlock()

	stats := *e.stats

	// Update dynamic stats
	e.mu.RLock()
	stats.MemTableSize = e.memtable.Size()
	stats.SSTCount = int64(len(e.sstManager.GetAllSSTables()))
	e.mu.RUnlock()

	walSize, _ := e.wal.Size()
	stats.WALSize = walSize

	return stats
}

// Close shuts down the engine gracefully
func (e *Engine) Close() error {
	close(e.stopCh)

	// Stop compactor
	e.compactor.Stop()

	// Flush remaining memtables
	e.mu.Lock()
	for _, mt := range e.immutableMemtables {
		entries := mt.Entries()
		if err := e.sstManager.Flush(entries); err != nil {
			fmt.Printf("Final flush failed: %v\n", err)
		}
	}

	// Flush active memtable
	entries := e.memtable.Entries()
	if err := e.sstManager.Flush(entries); err != nil {
		fmt.Printf("Final flush failed: %v\n", err)
	}
	e.mu.Unlock()

	// Close WAL
	return e.wal.Close()
}
