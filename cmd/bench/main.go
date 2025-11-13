package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	addr        = flag.String("addr", "localhost:8080", "Server address")
	duration    = flag.Duration("duration", 30*time.Second, "Benchmark duration")
	concurrency = flag.Int("concurrency", 10, "Number of concurrent clients")
	readRatio   = flag.Float64("read-ratio", 0.8, "Read ratio (0.0-1.0)")
	keyCount    = flag.Int("key-count", 10000, "Total number of unique keys")
	hotKeyRatio = flag.Float64("hot-key-ratio", 0.2, "Hot key ratio (80/20 pattern)")
)

// KeySizeDistribution: 70% small, 20% medium, 10% large
type KeySizeDistribution struct {
	Small  float64 // <= 1KB
	Medium float64 // 1KB - 10KB
	Large  float64 // 10KB - 100KB
}

var keySizeDist = KeySizeDistribution{
	Small:  0.7,
	Medium: 0.2,
	Large:  0.1,
}

type Stats struct {
	reads       int64
	writes      int64
	deletes     int64
	errors      int64
	readLatency int64 // nanoseconds
	writeLatency int64
}

func main() {
	flag.Parse()

	log.Printf("Benchmark Configuration:")
	log.Printf("  Server: %s", *addr)
	log.Printf("  Duration: %v", *duration)
	log.Printf("  Concurrency: %d", *concurrency)
	log.Printf("  Read Ratio: %.2f", *readRatio)
	log.Printf("  Key Count: %d", *keyCount)
	log.Printf("  Hot Key Ratio: %.2f", *hotKeyRatio)

	// Pre-populate some keys
	log.Println("Pre-populating keys...")
	if err := prepopulate(*addr, *keyCount/10); err != nil {
		log.Fatalf("Prepopulation failed: %v", err)
	}

	// Run benchmark
	log.Println("Starting benchmark...")
	stats := runBenchmark()

	// Print results
	printResults(stats)
}

func prepopulate(addr string, count int) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	for i := 0; i < count; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := generateValue()
		cmd := fmt.Sprintf("write %s|%s\r", key, value)

		if _, err := writer.WriteString(cmd); err != nil {
			return err
		}
		writer.Flush()

		// Read response
		if _, err := reader.ReadString('\r'); err != nil {
			return err
		}

		if i%1000 == 0 {
			log.Printf("  Prepopulated %d keys", i)
		}
	}

	log.Printf("  Prepopulated %d keys", count)
	return nil
}

func runBenchmark() *Stats {
	stats := &Stats{}
	var wg sync.WaitGroup

	stopCh := make(chan struct{})

	// Start workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go worker(i, stats, stopCh, &wg)
	}

	// Run for duration
	time.Sleep(*duration)
	close(stopCh)

	wg.Wait()
	return stats
}

func worker(id int, stats *Stats, stopCh chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Printf("Worker %d: connection failed: %v", id, err)
		return
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))

	for {
		select {
		case <-stopCh:
			return
		default:
			// Decide operation
			if rng.Float64() < *readRatio {
				// Read operation
				key := selectKey(rng)
				start := time.Now()

				cmd := fmt.Sprintf("read %s\r", key)
				if _, err := writer.WriteString(cmd); err != nil {
					atomic.AddInt64(&stats.errors, 1)
					continue
				}
				writer.Flush()

				if _, err := reader.ReadString('\r'); err != nil {
					atomic.AddInt64(&stats.errors, 1)
					continue
				}

				latency := time.Since(start).Nanoseconds()
				atomic.AddInt64(&stats.reads, 1)
				atomic.AddInt64(&stats.readLatency, latency)
			} else {
				// Write operation (90% writes, 10% deletes)
				if rng.Float64() < 0.9 {
					key := selectKey(rng)
					value := generateValue()
					start := time.Now()

					cmd := fmt.Sprintf("write %s|%s\r", key, value)
					if _, err := writer.WriteString(cmd); err != nil {
						atomic.AddInt64(&stats.errors, 1)
						continue
					}
					writer.Flush()

					if _, err := reader.ReadString('\r'); err != nil {
						atomic.AddInt64(&stats.errors, 1)
						continue
					}

					latency := time.Since(start).Nanoseconds()
					atomic.AddInt64(&stats.writes, 1)
					atomic.AddInt64(&stats.writeLatency, latency)
				} else {
					// Delete operation
					key := selectKey(rng)
					start := time.Now()

					cmd := fmt.Sprintf("delete %s\r", key)
					if _, err := writer.WriteString(cmd); err != nil {
						atomic.AddInt64(&stats.errors, 1)
						continue
					}
					writer.Flush()

					if _, err := reader.ReadString('\r'); err != nil {
						atomic.AddInt64(&stats.errors, 1)
						continue
					}

					latency := time.Since(start).Nanoseconds()
					atomic.AddInt64(&stats.deletes, 1)
					atomic.AddInt64(&stats.writeLatency, latency)
				}
			}
		}
	}
}

// selectKey implements 80/20 access pattern
func selectKey(rng *rand.Rand) string {
	hotKeyCount := int(float64(*keyCount) * *hotKeyRatio)

	if rng.Float64() < 0.8 {
		// 80% of accesses go to 20% of keys (hot keys)
		keyNum := rng.Intn(hotKeyCount)
		return fmt.Sprintf("key-%d", keyNum)
	}

	// 20% of accesses go to 80% of keys (cold keys)
	keyNum := hotKeyCount + rng.Intn(*keyCount-hotKeyCount)
	return fmt.Sprintf("key-%d", keyNum)
}

// generateValue generates a value based on size distribution
func generateValue() string {
	rng := rand.Float64()
	var size int

	if rng < keySizeDist.Small {
		// Small: 100 bytes to 1KB
		size = 100 + rand.Intn(924)
	} else if rng < keySizeDist.Small+keySizeDist.Medium {
		// Medium: 1KB to 10KB
		size = 1024 + rand.Intn(9*1024)
	} else {
		// Large: 10KB to 100KB
		size = 10*1024 + rand.Intn(90*1024)
	}

	// Generate random string
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, size)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func printResults(stats *Stats) {
	reads := atomic.LoadInt64(&stats.reads)
	writes := atomic.LoadInt64(&stats.writes)
	deletes := atomic.LoadInt64(&stats.deletes)
	errors := atomic.LoadInt64(&stats.errors)
	readLatency := atomic.LoadInt64(&stats.readLatency)
	writeLatency := atomic.LoadInt64(&stats.writeLatency)

	totalOps := reads + writes + deletes
	durationSec := duration.Seconds()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("BENCHMARK RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\nOperations:\n")
	fmt.Printf("  Total Operations: %d\n", totalOps)
	fmt.Printf("  Reads:            %d (%.1f%%)\n", reads, float64(reads)/float64(totalOps)*100)
	fmt.Printf("  Writes:           %d (%.1f%%)\n", writes, float64(writes)/float64(totalOps)*100)
	fmt.Printf("  Deletes:          %d (%.1f%%)\n", deletes, float64(deletes)/float64(totalOps)*100)
	fmt.Printf("  Errors:           %d\n", errors)

	fmt.Printf("\nThroughput:\n")
	fmt.Printf("  Total:            %.2f ops/sec\n", float64(totalOps)/durationSec)
	fmt.Printf("  Reads:            %.2f ops/sec\n", float64(reads)/durationSec)
	fmt.Printf("  Writes:           %.2f ops/sec\n", float64(writes)/durationSec)

	fmt.Printf("\nLatency (Average):\n")
	if reads > 0 {
		avgReadLatency := time.Duration(readLatency / reads)
		fmt.Printf("  Read:             %v\n", avgReadLatency)
	}
	if writes+deletes > 0 {
		avgWriteLatency := time.Duration(writeLatency / (writes + deletes))
		fmt.Printf("  Write:            %v\n", avgWriteLatency)
	}

	fmt.Println(strings.Repeat("=", 60))
}
