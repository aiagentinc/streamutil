package streamutil

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"runtime"
	"testing"
)

var testDataSizes = []int{
	1024,        // 1KB
	1024 * 10,   // 10KB
	1024 * 100,  // 100KB
	1024 * 1024, // 1MB
}

func getTestDataSizes() []int {
	if os.Getenv("CI") == "true" {
		return testDataSizes[:3] // Only up to 100KB in CI
	}
	return testDataSizes
}

func generateTestData(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

func BenchmarkReader(b *testing.B) {
	for _, size := range getTestDataSizes() {
		b.Run(fmt.Sprintf("size=%dKB", size/1024), func(b *testing.B) {
			data := generateTestData(size)
			cb := NewSizeCallback()

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				reader := Reader(bytes.NewReader(data), cb)
				_, _ = io.Copy(io.Discard, reader)
			}
		})
	}
}

func BenchmarkWriter(b *testing.B) {
	for _, size := range getTestDataSizes() {
		b.Run(fmt.Sprintf("size=%dKB", size/1024), func(b *testing.B) {
			data := generateTestData(size)
			cb := NewSizeCallback()

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				writer := Writer(io.Discard, cb)
				_, _ = writer.Write(data)
			}
		})
	}
}

func BenchmarkHashCallback(b *testing.B) {
	algorithms := []string{"md5", "sha1", "sha256", "sha512"}

	for _, algo := range algorithms {
		for _, size := range getTestDataSizes() {
			b.Run(fmt.Sprintf("algo=%s/size=%dKB", algo, size/1024), func(b *testing.B) {
				data := generateTestData(size)

				b.ResetTimer()
				b.SetBytes(int64(size))

				for i := 0; i < b.N; i++ {
					cb := NewHashCallback(algo)
					reader := Reader(bytes.NewReader(data), cb)
					_, _ = io.Copy(io.Discard, reader)
				}
			})
		}
	}
}

func BenchmarkMultiHashCallback(b *testing.B) {
	hashCounts := []int{1, 2, 3, 4}
	allAlgos := []string{"md5", "sha1", "sha256", "sha512"}

	for _, count := range hashCounts {
		algos := allAlgos[:count]
		b.Run(fmt.Sprintf("hashes=%d", count), func(b *testing.B) {
			size := 1024 * 1024 // 1MB
			if os.Getenv("CI") == "true" {
				size = 1024 * 100 // 100KB in CI
			}
			data := generateTestData(size)

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				cb := NewMultiHashCallback(algos...)
				reader := Reader(bytes.NewReader(data), cb)
				_, _ = io.Copy(io.Discard, reader)
			}
		})
	}
}

func BenchmarkMultipleCallbacks(b *testing.B) {
	callbackCounts := []int{1, 2, 5, 10}

	for _, count := range callbackCounts {
		b.Run(fmt.Sprintf("callbacks=%d", count), func(b *testing.B) {
			size := 1024 * 1024 // 1MB
			if os.Getenv("CI") == "true" {
				size = 1024 * 100 // 100KB in CI
			}
			data := generateTestData(size)

			callbacks := make([]ReadCallback, count)
			for i := 0; i < count; i++ {
				callbacks[i] = NewSizeCallback()
			}

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				reader := Reader(bytes.NewReader(data), callbacks...)
				_, _ = io.Copy(io.Discard, reader)
			}
		})
	}
}

func BenchmarkTeeReader(b *testing.B) {
	for _, size := range getTestDataSizes() {
		b.Run(fmt.Sprintf("size=%dKB", size/1024), func(b *testing.B) {
			data := generateTestData(size)
			cb := NewSizeCallback()

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				reader := TeeReader(bytes.NewReader(data), io.Discard, cb)
				_, _ = io.Copy(io.Discard, reader)
			}
		})
	}
}

func BenchmarkConcurrentReaders(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8, 16}
	if os.Getenv("CI") == "true" {
		concurrencyLevels = []int{1, 2, 4} // Reduce in CI
	}
	size := 1024 * 1024 // 1MB
	if os.Getenv("CI") == "true" {
		size = 1024 * 100 // 100KB in CI
	}
	data := generateTestData(size)

	for _, level := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrency=%d", level), func(b *testing.B) {
			b.SetParallelism(level)
			b.ResetTimer()
			b.SetBytes(int64(size))

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					cb := NewSizeCallback()
					reader := Reader(bytes.NewReader(data), cb)
					_, _ = io.Copy(io.Discard, reader)
				}
			})
		})
	}
}

func BenchmarkConcurrentWriters(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8, 16}
	if os.Getenv("CI") == "true" {
		concurrencyLevels = []int{1, 2, 4} // Reduce in CI
	}
	size := 1024 * 1024 // 1MB
	if os.Getenv("CI") == "true" {
		size = 1024 * 100 // 100KB in CI
	}
	data := generateTestData(size)

	for _, level := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrency=%d", level), func(b *testing.B) {
			b.SetParallelism(level)
			b.ResetTimer()
			b.SetBytes(int64(size))

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					cb := NewSizeCallback()
					writer := Writer(io.Discard, cb)
					_, _ = writer.Write(data)
				}
			})
		})
	}
}

func BenchmarkSizeCallbackAtomic(b *testing.B) {
	cb := NewSizeCallback()
	data := generateTestData(1024) // 1KB chunks

	b.ResetTimer()
	b.SetBytes(int64(1024))

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.OnData(data)
		}
	})
}

func BenchmarkBufferSizes(b *testing.B) {
	bufferSizes := []int{
		4 * 1024,   // 4KB
		16 * 1024,  // 16KB
		32 * 1024,  // 32KB (current default)
		64 * 1024,  // 64KB
		128 * 1024, // 128KB
	}
	if os.Getenv("CI") == "true" {
		bufferSizes = bufferSizes[:3] // Only test up to 32KB in CI
	}

	size := 10 * 1024 * 1024 // 10MB
	if os.Getenv("CI") == "true" {
		size = 1024 * 1024 // 1MB in CI
	}
	data := generateTestData(size)

	for _, bufSize := range bufferSizes {
		b.Run(fmt.Sprintf("buffer=%dKB", bufSize/1024), func(b *testing.B) {
			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(data)
				// Create a reader with the specified buffer size
				// Note: This is a simulation of buffer size impact
				buf := make([]byte, bufSize)
				for {
					n, err := reader.Read(buf)
					if err == io.EOF {
						break
					}
					if n > 0 {
						// Simulate processing
						_ = buf[:n]
					}
				}
			}
		})
	}
}

func BenchmarkMemoryAllocation(b *testing.B) {
	sizes := []int{1024 * 1024} // 1MB
	if os.Getenv("CI") != "true" {
		sizes = append(sizes, 10*1024*1024) // Add 10MB only for local tests
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%dMB", size/(1024*1024)), func(b *testing.B) {
			data := generateTestData(size)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var totalAlloc uint64
				memStats := &runtime.MemStats{}

				runtime.GC()
				runtime.ReadMemStats(memStats)
				allocBefore := memStats.Alloc

				cb := NewMultiHashCallback("md5", "sha256")
				reader := Reader(bytes.NewReader(data), cb)
				_, _ = io.Copy(io.Discard, reader)

				runtime.ReadMemStats(memStats)
				allocAfter := memStats.Alloc

				if allocAfter > allocBefore {
					totalAlloc = allocAfter - allocBefore
				}

				b.ReportMetric(float64(totalAlloc), "bytes/op")
			}
		})
	}
}
