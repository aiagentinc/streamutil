package streamutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestScalability tests how performance scales with different parameters
func TestScalability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scalability test in short mode")
	}

	tests := []struct {
		name           string
		dataSizes      []int
		concurrencies  []int
		callbackCounts []int
	}{
		{
			name:           "DataSizeScaling",
			dataSizes:      []int{1024, 10240, 102400, 1048576, 10485760},
			concurrencies:  []int{1},
			callbackCounts: []int{1},
		},
		{
			name:           "ConcurrencyScaling",
			dataSizes:      []int{1048576}, // 1MB
			concurrencies:  []int{1, 2, 4, 8, 16, 32},
			callbackCounts: []int{1},
		},
		{
			name:           "CallbackScaling",
			dataSizes:      []int{1048576}, // 1MB
			concurrencies:  []int{1},
			callbackCounts: []int{1, 2, 5, 10, 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([][]float64, len(tt.dataSizes))

			for i, dataSize := range tt.dataSizes {
				results[i] = make([]float64, len(tt.concurrencies)*len(tt.callbackCounts))
				data := make([]byte, dataSize)
				_, _ = rand.Read(data)

				resultIdx := 0
				for _, concurrency := range tt.concurrencies {
					for _, callbackCount := range tt.callbackCounts {
						throughput := measureThroughput(data, concurrency, callbackCount, 2*time.Second)
						results[i][resultIdx] = throughput
						resultIdx++

						t.Logf("%s - Size: %dKB, Concurrency: %d, Callbacks: %d, Throughput: %.2f MB/s",
							tt.name, dataSize/1024, concurrency, callbackCount, throughput)
					}
				}
			}

			// Analyze scaling efficiency
			if len(results) > 1 {
				for i := 1; i < len(results); i++ {
					scalingFactor := float64(tt.dataSizes[i]) / float64(tt.dataSizes[i-1])
					throughputRatio := results[i][0] / results[i-1][0]
					efficiency := throughputRatio / scalingFactor * 100

					t.Logf("Scaling efficiency from %dKB to %dKB: %.2f%%",
						tt.dataSizes[i-1]/1024, tt.dataSizes[i]/1024, efficiency)
				}
			}
		})
	}
}

func measureThroughput(data []byte, concurrency int, callbackCount int, baseDuration time.Duration) float64 {
	// Adjust duration for CI
	duration := baseDuration
	if os.Getenv("CI") == "true" {
		duration = baseDuration / 5
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var totalBytes int64
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			callbacks := make([]ReadCallback, callbackCount)
			for j := 0; j < callbackCount; j++ {
				if j%2 == 0 {
					callbacks[j] = NewSizeCallback()
				} else {
					callbacks[j] = NewHashCallback("md5")
				}
			}

			for {
				select {
				case <-ctx.Done():
					return
				default:
					reader := Reader(bytes.NewReader(data), callbacks...)
					n, _ := io.Copy(io.Discard, reader)
					atomic.AddInt64(&totalBytes, n)
				}
			}
		}()
	}

	start := time.Now()
	wg.Wait()
	elapsed := time.Since(start)

	bytes := atomic.LoadInt64(&totalBytes)
	return float64(bytes) / elapsed.Seconds() / (1024 * 1024)
}

// TestPipelinePerformance tests performance of chained operations
func TestPipelinePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping pipeline performance test in short mode")
	}

	data := make([]byte, 10*1024*1024) // 10MB
	_, _ = rand.Read(data)

	pipelines := []struct {
		name  string
		setup func() (io.Reader, []interface{})
	}{
		{
			name: "Simple",
			setup: func() (io.Reader, []interface{}) {
				return bytes.NewReader(data), []interface{}{
					NewSizeCallback(),
				}
			},
		},
		{
			name: "HashChain",
			setup: func() (io.Reader, []interface{}) {
				return bytes.NewReader(data), []interface{}{
					NewHashCallback("md5"),
					NewHashCallback("sha256"),
					NewSizeCallback(),
				}
			},
		},
		{
			name: "ComplexPipeline",
			setup: func() (io.Reader, []interface{}) {
				return bytes.NewReader(data), []interface{}{
					NewMultiHashCallback("md5", "sha1", "sha256"),
					NewSizeCallback(),
					NewHashCallback("sha512"),
				}
			},
		},
		{
			name: "TeeWithCallbacks",
			setup: func() (io.Reader, []interface{}) {
				var buf bytes.Buffer
				r := TeeReader(bytes.NewReader(data), &buf,
					NewSizeCallback(),
					NewHashCallback("sha256"))
				return r, nil
			},
		},
	}

	for _, pipeline := range pipelines {
		t.Run(pipeline.name, func(t *testing.T) {
			iterations := 50
			if os.Getenv("CI") == "true" {
				iterations = 10 // Reduce iterations in CI
			}
			var totalDuration time.Duration

			for i := 0; i < iterations; i++ {
				reader, callbacks := pipeline.setup()

				start := time.Now()
				if callbacks != nil {
					cbs := make([]ReadCallback, len(callbacks))
					for j, cb := range callbacks {
						cbs[j] = cb.(ReadCallback)
					}
					reader = Reader(reader, cbs...)
				}
				_, _ = io.Copy(io.Discard, reader)
				totalDuration += time.Since(start)
			}

			avgDuration := totalDuration / time.Duration(iterations)
			throughput := float64(len(data)) / avgDuration.Seconds() / (1024 * 1024)

			t.Logf("Pipeline %s: avg duration=%v, throughput=%.2f MB/s",
				pipeline.name, avgDuration, throughput)
		})
	}
}

// TestRealWorldScenarios simulates real-world usage patterns
func TestRealWorldScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world scenarios test in short mode")
	}

	scenarios := []struct {
		name       string
		fileSize   int
		chunkSize  int
		operation  string
		concurrent bool
	}{
		{"SmallFileUpload", 100 * 1024, 4096, "hash+size", false},
		{"LargeFileDownload", 10 * 1024 * 1024, 32768, "hash", false},
		{"StreamProcessing", 10 * 1024 * 1024, 8192, "multi-hash", true},
		{"LogAnalysis", 5 * 1024 * 1024, 1024, "size", true},
		{"BackupOperation", 50 * 1024 * 1024, 65536, "hash+compress", false},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			data := make([]byte, scenario.fileSize)
			_, _ = rand.Read(data)

			start := time.Now()
			var totalProcessed int64

			if scenario.concurrent {
				var wg sync.WaitGroup
				chunks := scenario.fileSize / scenario.chunkSize

				for i := 0; i < chunks; i++ {
					wg.Add(1)
					go func(offset int) {
						defer wg.Done()

						chunkStart := offset * scenario.chunkSize
						chunkEnd := chunkStart + scenario.chunkSize
						if chunkEnd > len(data) {
							chunkEnd = len(data)
						}

						chunk := data[chunkStart:chunkEnd]
						processChunk(chunk, scenario.operation)
						atomic.AddInt64(&totalProcessed, int64(len(chunk)))
					}(i)
				}
				wg.Wait()
			} else {
				reader := bytes.NewReader(data)
				buf := make([]byte, scenario.chunkSize)

				for {
					n, err := reader.Read(buf)
					if err == io.EOF {
						break
					}
					if n > 0 {
						processChunk(buf[:n], scenario.operation)
						totalProcessed += int64(n)
					}
				}
			}

			elapsed := time.Since(start)
			throughput := float64(totalProcessed) / elapsed.Seconds() / (1024 * 1024)

			t.Logf("Scenario %s completed:", scenario.name)
			t.Logf("  File size: %d MB", scenario.fileSize/(1024*1024))
			t.Logf("  Chunk size: %d KB", scenario.chunkSize/1024)
			t.Logf("  Duration: %v", elapsed)
			t.Logf("  Throughput: %.2f MB/s", throughput)
		})
	}
}

func processChunk(chunk []byte, operation string) {
	switch operation {
	case "hash":
		cb := NewHashCallback("sha256")
		reader := Reader(bytes.NewReader(chunk), cb)
		_, _ = io.Copy(io.Discard, reader)

	case "hash+size":
		cb1 := NewHashCallback("sha256")
		cb2 := NewSizeCallback()
		reader := Reader(bytes.NewReader(chunk), cb1, cb2)
		_, _ = io.Copy(io.Discard, reader)

	case "multi-hash":
		cb := NewMultiHashCallback("md5", "sha1", "sha256")
		reader := Reader(bytes.NewReader(chunk), cb)
		_, _ = io.Copy(io.Discard, reader)

	case "size":
		cb := NewSizeCallback()
		reader := Reader(bytes.NewReader(chunk), cb)
		_, _ = io.Copy(io.Discard, reader)

	case "hash+compress":
		// Simulate compression by just hashing
		cb := NewHashCallback("sha512")
		reader := Reader(bytes.NewReader(chunk), cb)
		_, _ = io.Copy(io.Discard, reader)
	}
}

// TestResourceContention tests performance under resource contention
func TestResourceContention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource contention test in short mode")
	}

	data := make([]byte, 1024*1024) // 1MB
	_, _ = rand.Read(data)

	// Skip resource contention tests in CI to avoid flaky results
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping resource contention tests in CI")
	}

	// Test CPU contention
	t.Run("CPUContention", func(t *testing.T) {
		var wg sync.WaitGroup
		stopCPU := make(chan struct{})

		// Start CPU-intensive background tasks
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stopCPU:
						return
					default:
						// CPU-intensive operation
						sum := 0
						for j := 0; j < 1000000; j++ {
							sum += j
						}
						_ = sum
					}
				}
			}()
		}

		// Measure performance under CPU contention
		start := time.Now()
		for i := 0; i < 100; i++ {
			cb := NewMultiHashCallback("md5", "sha256", "sha512")
			reader := Reader(bytes.NewReader(data), cb)
			_, _ = io.Copy(io.Discard, reader)
		}
		contentionDuration := time.Since(start)

		close(stopCPU)
		wg.Wait()

		// Measure performance without contention
		start = time.Now()
		for i := 0; i < 100; i++ {
			cb := NewMultiHashCallback("md5", "sha256", "sha512")
			reader := Reader(bytes.NewReader(data), cb)
			_, _ = io.Copy(io.Discard, reader)
		}
		normalDuration := time.Since(start)

		degradation := float64(contentionDuration-normalDuration) / float64(normalDuration) * 100
		t.Logf("CPU contention impact:")
		t.Logf("  Normal duration: %v", normalDuration)
		t.Logf("  Contention duration: %v", contentionDuration)
		t.Logf("  Performance degradation: %.2f%%", degradation)
	})

	// Test memory contention
	t.Run("MemoryContention", func(t *testing.T) {
		var wg sync.WaitGroup
		stopMem := make(chan struct{})

		// Start memory-intensive background tasks
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				allocations := make([][]byte, 0)
				for {
					select {
					case <-stopMem:
						return
					default:
						// Allocate and touch memory
						mem := make([]byte, 1024*1024) // 1MB
						for j := range mem {
							mem[j] = byte(j % 256)
						}
						allocations = append(allocations, mem)
						if len(allocations) > 100 {
							allocations = allocations[1:]
						}
					}
				}
			}()
		}

		time.Sleep(100 * time.Millisecond) // Let allocations start

		// Measure performance under memory pressure
		start := time.Now()
		for i := 0; i < 50; i++ {
			cb := NewSizeCallback()
			reader := Reader(bytes.NewReader(data), cb)
			_, _ = io.Copy(io.Discard, reader)
		}
		contentionDuration := time.Since(start)

		close(stopMem)
		wg.Wait()

		// Measure performance without contention
		start = time.Now()
		for i := 0; i < 50; i++ {
			cb := NewSizeCallback()
			reader := Reader(bytes.NewReader(data), cb)
			_, _ = io.Copy(io.Discard, reader)
		}
		normalDuration := time.Since(start)

		degradation := float64(contentionDuration-normalDuration) / float64(normalDuration) * 100
		t.Logf("Memory contention impact:")
		t.Logf("  Normal duration: %v", normalDuration)
		t.Logf("  Contention duration: %v", contentionDuration)
		t.Logf("  Performance degradation: %.2f%%", degradation)
	})
}
