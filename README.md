# Escabelo - High-Performance Key-Value Store

Escabelo is a high-performance, LSM-tree based key-value database written in Go, designed for the Pizzaria Bate-Papo technical challenge.

## 🚀 Features

- **LSM-Tree Architecture**: Memtable + SST files with background compaction
- **Write-Ahead Log (WAL)**: Durability and crash recovery
- **Background Compaction**: Automatic merging of SST files
- **TCP Protocol**: Custom text-based protocol for operations
- **80/20 Workload Optimization**: Optimized for hot key access patterns
- **Configurable**: Flexible configuration via command-line flags
- **Benchmarking Tools**: Built-in benchmark client with realistic workloads

## 📋 Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ TCP
       ▼
┌─────────────────────────────────────┐
│         TCP Server                  │
│  (Protocol Parser + Handler)        │
└──────────┬──────────────────────────┘
           │
           ▼
┌─────────────────────────────────────┐
│         Storage Engine              │
│                                     │
│  ┌──────────────┐                  │
│  │   MemTable   │ (Active)         │
│  └──────────────┘                  │
│         │                           │
│         ▼                           │
│  ┌──────────────┐                  │
│  │ Immutable MT │ (Flush Queue)    │
│  └──────────────┘                  │
│         │                           │
│         ▼                           │
│  ┌──────────────┐                  │
│  │  SST Files   │ (L0, L1, ...)    │
│  └──────────────┘                  │
│         │                           │
│         ▼                           │
│  ┌──────────────┐                  │
│  │  Compactor   │ (Background)     │
│  └──────────────┘                  │
│                                     │
│  ┌──────────────┐                  │
│  │     WAL      │ (Durability)     │
│  └──────────────┘                  │
└─────────────────────────────────────┘
```

## 🛠️ Installation

### Prerequisites

- Go 1.23.3 or higher
- Make (optional, for convenience)

### Build

```bash
# Clone the repository
git clone <repository-url>
cd escabelo

# Install dependencies
make install-deps

# Build server and benchmark tool
make build-all
```

## 🎯 Usage

### Starting the Server

```bash
# Default settings (port 8080, 64MB memtable)
make run

# Custom settings
./bin/escabelo \
  -port=8080 \
  -data-dir=./data \
  -memtable-size=67108864 \
  -compaction-interval=5m \
  -wal-sync-interval=1s
```

### Configuration Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | 8080 | TCP port to listen on |
| `-data-dir` | ./data | Directory for data storage |
| `-memtable-size` | 67108864 | Max memtable size (64MB) |
| `-compaction-interval` | 5m | Background compaction interval |
| `-wal-sync-interval` | 1s | WAL sync to disk interval |

## 📡 Protocol

Escabelo uses a simple text-based protocol over TCP. Commands are separated by `\r`.

### Commands

#### Write
```
write <key>|<value>\r
Response: success\r or error: <message>\r
```

#### Read
```
read <key>\r
Response: <value>\r or error: key not found\r
```

#### Delete
```
delete <key>\r
Response: success\r or error: key not found\r
```

#### Status
```
status\r
Response: well going our operation
writes=<n> reads=<n> deletes=<n> flushes=<n> memtable_size=<n> sst_count=<n> wal_size=<n>\r
```

#### Keys
```
keys\r
Response: <key1>\r<key2>\r<key3>\r...
```

#### Prefix Scan
```
reads <prefix>\r
Response: <value1>\r<value2>\r<value3>\r...
```

### Key Format

Keys must match: `([a-z] | [A-Z] | [0-9] | "." | "-" | ":")+`

- Maximum key size: 100KB
- Valid characters: alphanumeric, dot, hyphen, colon

## 📊 Benchmarking

### Running Benchmarks

```bash
# Standard benchmark (30s, 10 clients, 80% reads)
make bench

# Intensive benchmark (60s, 50 clients)
make bench-intensive

# Write-heavy benchmark (20% reads, 80% writes)
make bench-write

# Custom benchmark
./bin/bench \
  -addr=localhost:8080 \
  -duration=60s \
  -concurrency=20 \
  -read-ratio=0.8 \
  -key-count=50000
```

### Benchmark Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | localhost:8080 | Server address |
| `-duration` | 30s | Benchmark duration |
| `-concurrency` | 10 | Number of concurrent clients |
| `-read-ratio` | 0.8 | Read ratio (0.0-1.0) |
| `-key-count` | 10000 | Total unique keys |
| `-hot-key-ratio` | 0.2 | Hot key ratio (80/20 pattern) |

### Workload Characteristics

The benchmark simulates realistic workloads:

- **80/20 Access Pattern**: 80% of requests target 20% of keys (hot keys)
- **Key Size Distribution**:
  - 70% small keys (≤ 1KB)
  - 20% medium keys (1KB - 10KB)
  - 10% large keys (10KB - 100KB)
- **Operation Mix**: Configurable read/write ratio
- **Concurrent Clients**: Simulates multiple simultaneous connections

## 🧪 Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Quick integration test
make quick-test
```

## 🏗️ Project Structure

```
escabelo/
├── cmd/
│   ├── escabelo/          # Main server application
│   │   └── main.go
│   └── bench/             # Benchmark client
│       └── main.go
├── internal/
│   ├── engine/            # Storage engine (LSM-tree)
│   │   ├── engine.go      # Main engine
│   │   ├── memtable.go    # In-memory table
│   │   ├── sst.go         # SSTable management
│   │   ├── wal.go         # Write-ahead log
│   │   └── compactor.go   # Background compaction
│   └── server/            # TCP server
│       ├── server.go      # Connection handling
│       └── protocol.go    # Protocol parser
├── data/                  # Data directory (created at runtime)
├── Makefile              # Build automation
├── go.mod                # Go module definition
└── README.md             # This file
```

## 🔧 Development

### Code Formatting

```bash
make fmt
```

### Cleaning

```bash
# Remove build artifacts and data
make clean

# Remove only data directory
make clean-data
```

## 📈 Performance Characteristics

### Write Performance

- **Memtable Writes**: O(1) average case (hash map)
- **WAL Append**: Sequential writes, buffered I/O
- **Flush to SST**: Background, non-blocking

### Read Performance

- **Hot Keys**: O(1) from memtable (in-memory)
- **Cold Keys**: O(log n) with sparse indexing
- **Worst Case**: Sequential scan of SST file

### Space Efficiency

- **Compaction**: Removes deleted keys and old versions
- **SST Format**: Efficient binary encoding
- **Sparse Indexing**: Reduces memory overhead

## 🛡️ Durability & Recovery

### Write-Ahead Log

- All writes are logged before being applied to memtable
- WAL is synced to disk periodically (configurable)
- On crash, WAL is replayed to restore state

### Crash Recovery

1. Server starts
2. WAL is replayed
3. Memtable is reconstructed
4. Existing SST files are loaded
5. Server is ready for requests

### Data Integrity

- Atomic writes via WAL
- Consistent state after crash
- No data loss for committed writes (after WAL sync)

## 📊 Metrics & Monitoring

The `status` command provides real-time metrics:

- **Writes**: Total write operations
- **Reads**: Total read operations
- **Deletes**: Total delete operations
- **Flushes**: Number of memtable flushes
- **Memtable Size**: Current memtable size in bytes
- **SST Count**: Number of SST files
- **WAL Size**: Current WAL file size

## 🎓 Technical Details

### LSM-Tree Implementation

Escabelo implements a simplified LSM-tree (Log-Structured Merge-Tree):

1. **Writes** go to WAL, then memtable
2. When memtable is full, it becomes **immutable**
3. Immutable memtables are **flushed** to SST files
4. **Background compaction** merges SST files
5. **Reads** check memtable first, then SST files (newest to oldest)

### Compaction Strategy

- **Trigger**: Runs periodically (configurable interval)
- **Strategy**: Merge oldest 4 SST files when count > 4
- **Process**: 
  - Read all entries from selected SSTs
  - Keep newest version of each key
  - Remove tombstones (deleted keys)
  - Write merged SST
  - Delete old SSTs

### SSTable Format

Binary format for efficient storage:

```
Entry: [timestamp:8][deleted:1][keyLen:4][key:N][valueLen:4][value:M]
```

- **Timestamp**: 8 bytes (int64)
- **Deleted**: 1 byte (tombstone flag)
- **Key Length**: 4 bytes (uint32)
- **Key**: Variable length
- **Value Length**: 4 bytes (uint32)
- **Value**: Variable length

## 🤝 Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Format code: `make fmt`
6. Submit a pull request

## 📝 License

[Add your license here]

## 🙏 Acknowledgments

Built for the Pizzaria Bate-Papo technical challenge, inspired by:

- RocksDB (Facebook)
- LevelDB (Google)
- Bitcask (Basho)

---

**Made with ❤️ for high-performance key-value storage**
