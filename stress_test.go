package streamutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Adjust duration based on CI environment
	duration := 2 * time.Second
	if os.Getenv("CI") == "true" {
		duration = 500 * time.Millisecond
	}

	tests := []struct {
		name       string
		goroutines int
		duration   time.Duration
		dataSize   int
	}{
		{"LowConcurrency", 10, duration, 1024 * 1024},
		{"MediumConcurrency", 50, duration, 1024 * 1024},
		{"HighConcurrency", 100, duration, 1024 * 1024},
		{"ExtremeConcurrency", 500, duration, 1024 * 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.duration)
			defer cancel()

			var (
				totalBytes int64
				totalOps   int64
				errors     int64
				wg         sync.WaitGroup
				start      = time.Now()
			)

			data := make([]byte, tt.dataSize)
			_, _ = rand.Read(data)

			for i := 0; i < tt.goroutines; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					for {
						select {
						case <-ctx.Done():
							return
						default:
							if err := runStressOperation(data); err != nil {
								atomic.AddInt64(&errors, 1)
							} else {
								atomic.AddInt64(&totalOps, 1)
								atomic.AddInt64(&totalBytes, int64(len(data)))
							}
						}
					}
				}(i)
			}

			wg.Wait()
			elapsed := time.Since(start)

			ops := atomic.LoadInt64(&totalOps)
			bytes := atomic.LoadInt64(&totalBytes)
			errs := atomic.LoadInt64(&errors)

			throughput := float64(bytes) / elapsed.Seconds() / (1024 * 1024)
			opsPerSec := float64(ops) / elapsed.Seconds()

			t.Logf("Stress test completed:")
			t.Logf("  Goroutines: %d", tt.goroutines)
			t.Logf("  Duration: %v", elapsed)
			t.Logf("  Total operations: %d", ops)
			t.Logf("  Total bytes: %d", bytes)
			t.Logf("  Throughput: %.2f MB/s", throughput)
			t.Logf("  Operations/sec: %.2f", opsPerSec)
			t.Logf("  Errors: %d", errs)

			if errs > 0 {
				t.Errorf("Stress test had %d errors", errs)
			}
		})
	}
}

func runStressOperation(data []byte) error {
	// Use crypto/rand to generate a random operation
	var b [1]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return err
	}
	operation := int(b[0]) % 4

	switch operation {
	case 0: // Reader with hash
		cb := NewHashCallback("sha256")
		reader := Reader(bytes.NewReader(data), cb)
		_, err := io.Copy(io.Discard, reader)
		return err

	case 1: // Writer with size
		cb := NewSizeCallback()
		writer := Writer(io.Discard, cb)
		_, err := writer.Write(data)
		return err

	case 2: // TeeReader
		cb := NewSizeCallback()
		reader := TeeReader(bytes.NewReader(data), io.Discard, cb)
		_, err := io.Copy(io.Discard, reader)
		return err

	case 3: // Multiple callbacks
		cb1 := NewSizeCallback()
		cb2 := NewHashCallback("md5")
		cb3 := NewHashCallback("sha1")
		reader := Reader(bytes.NewReader(data), cb1, cb2, cb3)
		_, err := io.Copy(io.Discard, reader)
		return err

	default:
		return fmt.Errorf("unknown operation: %d", operation)
	}
}

func TestMemoryLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak detection in short mode")
	}

	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	startAlloc := m.Alloc
	startObjects := m.Mallocs - m.Frees

	// Run intensive operations
	data := make([]byte, 1024*1024) // 1MB
	_, _ = rand.Read(data)

	iterations := 500
	if os.Getenv("CI") == "true" {
		iterations = 50 // Reduce iterations in CI
	}

	for i := 0; i < iterations; i++ {
		cb1 := NewSizeCallback()
		cb2 := NewHashCallback("sha256")
		cb3 := NewMultiHashCallback("md5", "sha1")

		reader := Reader(bytes.NewReader(data), cb1, cb2, cb3)
		_, _ = io.Copy(io.Discard, reader)

		writer := Writer(io.Discard, cb1, cb2)
		_, _ = writer.Write(data)

		if bw, ok := writer.(*BufferedWriter); ok {
			_ = bw.Close()
		}
	}

	// Force garbage collection
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.ReadMemStats(&m)

	endAlloc := m.Alloc
	endObjects := m.Mallocs - m.Frees

	allocGrowth := float64(endAlloc-startAlloc) / float64(startAlloc) * 100
	objectGrowth := float64(endObjects-startObjects) / float64(startObjects) * 100

	t.Logf("Memory stats:")
	t.Logf("  Start alloc: %d bytes", startAlloc)
	t.Logf("  End alloc: %d bytes", endAlloc)
	t.Logf("  Alloc growth: %.2f%%", allocGrowth)
	t.Logf("  Start objects: %d", startObjects)
	t.Logf("  End objects: %d", endObjects)
	t.Logf("  Object growth: %.2f%%", objectGrowth)

	// Allow some reasonable growth but flag potential leaks
	if allocGrowth > 50 {
		t.Logf("WARNING: Memory allocation grew by %.2f%%, possible memory leak", allocGrowth)
	}
	if objectGrowth > 50 {
		t.Logf("WARNING: Object count grew by %.2f%%, possible memory leak", objectGrowth)
	}
}

func TestThroughputUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping throughput test in short mode")
	}

	scenarios := []struct {
		name     string
		dataSize int
		readers  int
		writers  int
		duration time.Duration
	}{
		{"Balanced", 1024 * 1024, 10, 10, getDuration(3 * time.Second)},
		{"ReadHeavy", 1024 * 1024, 30, 5, getDuration(3 * time.Second)},
		{"WriteHeavy", 1024 * 1024, 5, 30, getDuration(3 * time.Second)},
		{"LargeData", 10 * 1024 * 1024, 5, 5, getDuration(3 * time.Second)},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), scenario.duration)
			defer cancel()

			var (
				readBytes  int64
				writeBytes int64
				readOps    int64
				writeOps   int64
				wg         sync.WaitGroup
			)

			data := make([]byte, scenario.dataSize)
			_, _ = rand.Read(data)

			// Start readers
			for i := 0; i < scenario.readers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						default:
							cb := NewMultiHashCallback("md5", "sha256")
							reader := Reader(bytes.NewReader(data), cb)
							n, _ := io.Copy(io.Discard, reader)
							atomic.AddInt64(&readBytes, n)
							atomic.AddInt64(&readOps, 1)
						}
					}
				}()
			}

			// Start writers
			for i := 0; i < scenario.writers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						default:
							cb := NewSizeCallback()
							writer := Writer(io.Discard, cb)
							n, _ := writer.Write(data)
							atomic.AddInt64(&writeBytes, int64(n))
							atomic.AddInt64(&writeOps, 1)
						}
					}
				}()
			}

			start := time.Now()
			wg.Wait()
			elapsed := time.Since(start)

			rBytes := atomic.LoadInt64(&readBytes)
			wBytes := atomic.LoadInt64(&writeBytes)
			rOps := atomic.LoadInt64(&readOps)
			wOps := atomic.LoadInt64(&writeOps)

			readThroughput := float64(rBytes) / elapsed.Seconds() / (1024 * 1024)
			writeThroughput := float64(wBytes) / elapsed.Seconds() / (1024 * 1024)

			t.Logf("Throughput test results for %s:", scenario.name)
			t.Logf("  Read throughput: %.2f MB/s (%d ops)", readThroughput, rOps)
			t.Logf("  Write throughput: %.2f MB/s (%d ops)", writeThroughput, wOps)
			t.Logf("  Total throughput: %.2f MB/s", readThroughput+writeThroughput)
		})
	}
}

func TestLatencyDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency test in short mode")
	}

	operations := 5000
	if os.Getenv("CI") == "true" {
		operations = 500 // Reduce operations in CI
	}
	dataSizes := []int{1024, 10 * 1024, 100 * 1024, 1024 * 1024}

	for _, size := range dataSizes {
		t.Run(fmt.Sprintf("size=%dKB", size/1024), func(t *testing.T) {
			data := make([]byte, size)
			_, _ = rand.Read(data)

			latencies := make([]time.Duration, operations)

			for i := 0; i < operations; i++ {
				start := time.Now()

				cb := NewMultiHashCallback("md5", "sha256")
				reader := Reader(bytes.NewReader(data), cb)
				_, _ = io.Copy(io.Discard, reader)

				latencies[i] = time.Since(start)
			}

			// Calculate percentiles
			sortDurations(latencies)
			p50 := latencies[len(latencies)*50/100]
			p90 := latencies[len(latencies)*90/100]
			p95 := latencies[len(latencies)*95/100]
			p99 := latencies[len(latencies)*99/100]

			var total time.Duration
			for _, l := range latencies {
				total += l
			}
			avg := total / time.Duration(len(latencies))

			t.Logf("Latency distribution for %dKB:", size/1024)
			t.Logf("  Average: %v", avg)
			t.Logf("  P50: %v", p50)
			t.Logf("  P90: %v", p90)
			t.Logf("  P95: %v", p95)
			t.Logf("  P99: %v", p99)
		})
	}
}

func sortDurations(durations []time.Duration) {
	for i := 0; i < len(durations); i++ {
		for j := i + 1; j < len(durations); j++ {
			if durations[i] > durations[j] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}
}

// getDuration returns a shorter duration when running in CI
func getDuration(d time.Duration) time.Duration {
	if os.Getenv("CI") == "true" {
		return d / 5 // Reduce duration to 1/5 in CI
	}
	return d
}
