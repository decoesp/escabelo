package main

import (
	"escabelo/internal/engine"
	"escabelo/internal/server"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	port               = flag.String("port", "8080", "TCP port to listen on")
	dataDir            = flag.String("data-dir", "./data", "Directory for data storage")
	memtableSize       = flag.Int64("memtable-size", 64*1024*1024, "Max memtable size in bytes (default 64MB)")
	compactionInterval = flag.Duration("compaction-interval", 5*time.Minute, "Compaction interval")
	walSyncInterval    = flag.Duration("wal-sync-interval", 100*time.Millisecond, "WAL sync interval")
)

func main() {
	flag.Parse()

	log.Printf("Starting Escabelo Key-Value Store")
	log.Printf("Configuration:")
	log.Printf("  Port: %s", *port)
	log.Printf("  Data Directory: %s", *dataDir)
	log.Printf("  Memtable Size: %d bytes", *memtableSize)
	log.Printf("  Compaction Interval: %v", *compactionInterval)
	log.Printf("  WAL Sync Interval: %v", *walSyncInterval)

	// Create engine
	engineConfig := engine.Config{
		DataDir:            *dataDir,
		MemTableMaxSize:    *memtableSize,
		CompactionInterval: *compactionInterval,
		WALSyncInterval:    *walSyncInterval,
	}

	eng, err := engine.NewEngine(engineConfig)
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}
	defer eng.Close()

	// Create server
	addr := fmt.Sprintf(":%s", *port)
	srv := server.NewServer(addr, eng)

	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh
	log.Println("Shutting down...")

	if err := srv.Stop(); err != nil {
		log.Printf("Server stop error: %v", err)
	}

	log.Println("Shutdown complete")
}
