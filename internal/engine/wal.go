package engine

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// WAL (Write-Ahead Log) provides durability
type WAL struct {
	mu         sync.Mutex
	file       *os.File
	writer     *bufio.Writer
	filePath   string
	bufSize    int
	pendingOps int32 // atomic counter for pending operations
}

// WALEntry represents a log entry
type WALEntry struct {
	OpType    byte // 1=Put, 2=Delete
	Key       string
	Value     []byte
	Timestamp int64
}

const (
	OpTypePut    byte = 1
	OpTypeDelete byte = 2
)

// NewWAL creates or opens a WAL file
func NewWAL(dataDir string) (*WAL, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(dataDir, "wal.log")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	bufSize := 256 * 1024 // 256KB buffer for better throughput
	return &WAL{
		file:     file,
		writer:   bufio.NewWriterSize(file, bufSize),
		filePath: filePath,
		bufSize:  bufSize,
	}, nil
}

// Append writes an entry to the WAL
func (w *WAL) Append(entry *WALEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Format: opType(1) + timestamp(8) + keyLen(4) + key + valueLen(4) + value
	if err := w.writer.WriteByte(entry.OpType); err != nil {
		return err
	}

	if err := binary.Write(w.writer, binary.LittleEndian, entry.Timestamp); err != nil {
		return err
	}

	keyLen := uint32(len(entry.Key))
	if err := binary.Write(w.writer, binary.LittleEndian, keyLen); err != nil {
		return err
	}

	if _, err := w.writer.Write([]byte(entry.Key)); err != nil {
		return err
	}

	valueLen := uint32(len(entry.Value))
	if err := binary.Write(w.writer, binary.LittleEndian, valueLen); err != nil {
		return err
	}

	if _, err := w.writer.Write(entry.Value); err != nil {
		return err
	}

	// Group commit: only flush if buffer is nearly full
	// This allows batching many writes together for better throughput
	// The periodic syncer will handle durability
	if w.writer.Buffered() >= w.bufSize-4096 {
		return w.writer.Flush()
	}

	return nil
}

// Replay reads all entries from the WAL and applies them to a memtable
func (w *WAL) Replay() ([]*WALEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Seek to beginning
	if _, err := w.file.Seek(0, 0); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(w.file)
	var entries []*WALEntry

	for {
		entry := &WALEntry{}

		// Read opType
		opType, err := reader.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entry.OpType = opType

		// Read timestamp
		if err := binary.Read(reader, binary.LittleEndian, &entry.Timestamp); err != nil {
			return nil, err
		}

		// Read key
		var keyLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &keyLen); err != nil {
			return nil, err
		}

		keyBytes := make([]byte, keyLen)
		if _, err := io.ReadFull(reader, keyBytes); err != nil {
			return nil, err
		}
		entry.Key = string(keyBytes)

		// Read value
		var valueLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &valueLen); err != nil {
			return nil, err
		}

		entry.Value = make([]byte, valueLen)
		if _, err := io.ReadFull(reader, entry.Value); err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// Truncate clears the WAL (after successful flush to SST)
func (w *WAL) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.file.Truncate(0); err != nil {
		return err
	}

	if _, err := w.file.Seek(0, 0); err != nil {
		return err
	}

	w.writer.Reset(w.file)
	return nil
}

// Close closes the WAL file
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}

// Sync forces a sync to disk
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Sync()
}

// Size returns the current size of the WAL file
func (w *WAL) Size() (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	stat, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

// Rotate creates a new WAL file and returns the old one's path
func (w *WAL) Rotate() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Flush and close current file
	if err := w.writer.Flush(); err != nil {
		return "", err
	}
	if err := w.file.Close(); err != nil {
		return "", err
	}

	// Rename old file
	oldPath := w.filePath
	stat, err := w.file.Stat()
	if err != nil {
		return "", err
	}
	backupPath := fmt.Sprintf("%s.%d", w.filePath, stat.ModTime().Unix())

	// Create new file
	file, err := os.OpenFile(oldPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return "", err
	}

	w.file = file
	w.writer = bufio.NewWriter(file)

	return backupPath, nil
}
