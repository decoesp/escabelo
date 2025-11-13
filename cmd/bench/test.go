//go:build simplebench

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func getServerAddr() string {
	if addr := os.Getenv("SERVER_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:8080"
}

type Stats struct {
	totalOps     uint64
	successOps   uint64
	failedOps    uint64
	latencies    []time.Duration
	latenciesMux sync.Mutex
	startTime    time.Time
}

func (s *Stats) recordLatency(duration time.Duration, success bool) {
	atomic.AddUint64(&s.totalOps, 1)
	if success {
		atomic.AddUint64(&s.successOps, 1)
	} else {
		atomic.AddUint64(&s.failedOps, 1)
	}

	s.latenciesMux.Lock()
	s.latencies = append(s.latencies, duration)
	s.latenciesMux.Unlock()
}

func (s *Stats) calculatePercentile(p float64) time.Duration {
	s.latenciesMux.Lock()
	defer s.latenciesMux.Unlock()

	if len(s.latencies) == 0 {
		return 0
	}

	sorted := make([]time.Duration, len(s.latencies))
	copy(sorted, s.latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := int(float64(len(sorted)) * p)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func (s *Stats) printStats() {
	elapsed := time.Since(s.startTime).Seconds()

	fmt.Println("\n=== Benchmark Results ===")
	fmt.Printf("Total Operations:    %d\n", atomic.LoadUint64(&s.totalOps))
	fmt.Printf("Successful:          %d\n", atomic.LoadUint64(&s.successOps))
	fmt.Printf("Failed:              %d\n", atomic.LoadUint64(&s.failedOps))
	fmt.Printf("Duration:            %.2fs\n", elapsed)
	fmt.Printf("Throughput:          %.2f ops/sec\n", float64(atomic.LoadUint64(&s.totalOps))/elapsed)
	fmt.Printf("P50 Latency:         %v\n", s.calculatePercentile(0.50))
	fmt.Printf("P95 Latency:         %v\n", s.calculatePercentile(0.95))
	fmt.Printf("P99 Latency:         %v\n", s.calculatePercentile(0.99))
	fmt.Printf("Max Latency:         %v\n", s.calculatePercentile(1.0))
}

func generateValue(size int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, size)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

func getValueSize() int {
	r := rand.Intn(100)
	if r < 70 {
		// 70% small keys (<= 1KB)
		return rand.Intn(1024) + 1
	} else if r < 90 {
		// 20% medium keys (1KB - 10KB)
		return rand.Intn(9*1024) + 1024
	} else {
		// 10% large keys (10KB - 100KB)
		return rand.Intn(90*1024) + 10*1024
	}
}

func sendCommand(command string) (string, error) {
	conn, err := net.DialTimeout("tcp", getServerAddr(), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	// Protocol uses \r as separator, not \n
	_, err = conn.Write([]byte(command + "\r"))
	if err != nil {
		return "", err
	}

	buf := make([]byte, 128*1024) // 128KB buffer for large responses
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	// Remove \r from response
	response := string(buf[:n])
	if len(response) > 0 && response[len(response)-1] == '\r' {
		response = response[:len(response)-1]
	}

	return response, nil
}

func benchmarkWrites(numOps int, concurrency int, stats *Stats) {
	var wg sync.WaitGroup
	opsPerWorker := numOps / concurrency

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < opsPerWorker; j++ {
				key := fmt.Sprintf("key-%d:%d", workerID, j)
				value := generateValue(getValueSize())
				command := fmt.Sprintf("write %s|%s", key, value)

				start := time.Now()
				resp, err := sendCommand(command)
				latency := time.Since(start)

				success := (err == nil) && strings.HasPrefix(resp, "success")
				stats.recordLatency(latency, success)
			}
		}(i)
	}

	wg.Wait()
}

func benchmarkReads(numOps int, concurrency int, stats *Stats, totalWrites int) {
	var wg sync.WaitGroup
	opsPerWorker := numOps / concurrency

	// 80/20 rule: 80% of reads hit 20% of keys
	hotKeys := totalWrites / 5

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < opsPerWorker; j++ {
				var key string
				if rand.Intn(100) < 80 {
					// 80% access hot keys
					keyWorker := rand.Intn(concurrency)
					keyIndex := rand.Intn(hotKeys / concurrency)
					key = fmt.Sprintf("key-%d:%d", keyWorker, keyIndex)
				} else {
					// 20% access cold keys
					keyWorker := rand.Intn(concurrency)
					keyIndex := rand.Intn(totalWrites / concurrency)
					key = fmt.Sprintf("key_%d_%d", keyWorker, keyIndex)
				}

				command := fmt.Sprintf("read %s", key)

				start := time.Now()
				_, err := sendCommand(command)
				latency := time.Since(start)

				stats.recordLatency(latency, err == nil)
			}
		}(i)
	}

	wg.Wait()
}

func benchmarkMixed(numOps int, concurrency int, stats *Stats, totalWrites int) {
	var wg sync.WaitGroup
	opsPerWorker := numOps / concurrency

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < opsPerWorker; j++ {
				var command string

				if rand.Intn(100) < 70 {
					// 70% reads
					keyWorker := rand.Intn(concurrency)
					keyIndex := rand.Intn(totalWrites / concurrency)
					key := fmt.Sprintf("key_%d_%d", keyWorker, keyIndex)
					command = fmt.Sprintf("read %s", key)
				} else {
					// 30% writes
					key := fmt.Sprintf("key-%d:%d.mixed", workerID, j)
					value := generateValue(getValueSize())
					command = fmt.Sprintf("write %s|%s", key, value)
				}

				start := time.Now()
				_, err := sendCommand(command)
				latency := time.Since(start)

				stats.recordLatency(latency, err == nil)
			}
		}(i)
	}

	wg.Wait()
}

func main() {
	numOps := flag.Int("ops", 10000, "Number of operations to perform")
	concurrency := flag.Int("c", 10, "Number of concurrent workers")
	mode := flag.String("mode", "write", "Benchmark mode: write, read, or mixed")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	fmt.Printf("Starting benchmark: mode=%s, ops=%d, concurrency=%d\n", *mode, *numOps, *concurrency)

	stats := &Stats{
		latencies: make([]time.Duration, 0, *numOps),
		startTime: time.Now(),
	}

	switch *mode {
	case "write":
		fmt.Println("\n=== Write Benchmark ===")
		benchmarkWrites(*numOps, *concurrency, stats)
	case "read":
		fmt.Println("\n=== Read Benchmark ===")
		fmt.Println("Note: Run write benchmark first to populate data")
		benchmarkReads(*numOps, *concurrency, stats, *numOps)
	case "mixed":
		fmt.Println("\n=== Mixed Benchmark (70% read / 30% write) ===")
		benchmarkMixed(*numOps, *concurrency, stats, *numOps)
	default:
		fmt.Printf("Unknown mode: %s\n", *mode)
		return
	}

	stats.printStats()

	// Check server status
	fmt.Println("\n=== Server Status ===")
	status, err := sendCommand("status")
	if err != nil {
		fmt.Printf("Failed to get status: %v\n", err)
	} else {
		fmt.Println(status)
	}
}
