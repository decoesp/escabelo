package engine

import (
	"sync"
	"time"
)

// Entry represents a key-value pair with metadata
type Entry struct {
	Key       string
	Value     []byte
	Timestamp int64
	Deleted   bool
}

// MemTable is an in-memory sorted structure (skip list or tree-based)
type MemTable struct {
	mu      sync.RWMutex
	data    map[string]*Entry
	size    int64 // approximate size in bytes
	maxSize int64
}

// NewMemTable creates a new memtable with a size limit
func NewMemTable(maxSize int64) *MemTable {
	return &MemTable{
		data:    make(map[string]*Entry),
		maxSize: maxSize,
	}
}

// Put adds or updates a key-value pair
func (m *MemTable) Put(key string, value []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := &Entry{
		Key:       key,
		Value:     value,
		Timestamp: time.Now().UnixNano(),
		Deleted:   false,
	}

	// Update size tracking
	if old, exists := m.data[key]; exists {
		m.size -= int64(len(old.Key) + len(old.Value))
	}
	m.size += int64(len(key) + len(value))

	m.data[key] = entry
}

// Get retrieves a value by key
func (m *MemTable) Get(key string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists || entry.Deleted {
		return nil, false
	}
	return entry.Value, true
}

// Delete marks a key as deleted (tombstone)
func (m *MemTable) Delete(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.data[key]
	if !exists || entry.Deleted {
		return false
	}

	entry.Deleted = true
	entry.Timestamp = time.Now().UnixNano()
	return true
}

// Keys returns all non-deleted keys
func (m *MemTable) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k, entry := range m.data {
		if !entry.Deleted {
			keys = append(keys, k)
		}
	}
	return keys
}

// PrefixScan returns all values with keys starting with prefix
func (m *MemTable) PrefixScan(prefix string) [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var values [][]byte
	for k, entry := range m.data {
		if !entry.Deleted && len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			values = append(values, entry.Value)
		}
	}
	return values
}

// Size returns the approximate size in bytes
func (m *MemTable) Size() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size
}

// IsFull checks if memtable has reached its size limit
func (m *MemTable) IsFull() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size >= m.maxSize
}

// Entries returns all entries for flushing to SST
func (m *MemTable) Entries() []*Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]*Entry, 0, len(m.data))
	for _, entry := range m.data {
		entries = append(entries, entry)
	}
	return entries
}

// Clear removes all entries (used after flush)
func (m *MemTable) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]*Entry)
	m.size = 0
}
