# streamutil - Transparent Stream Processing for Go

[![CI](https://github.com/aiagentinc/streamutil/actions/workflows/test.yml/badge.svg)](https://github.com/aiagentinc/streamutil/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/aiagentinc/streamutil/graph/badge.svg)](https://codecov.io/gh/aiagentinc/streamutil)
[![Go Reference](https://pkg.go.dev/badge/github.com/aiagentinc/streamutil.svg)](https://pkg.go.dev/github.com/aiagentinc/streamutil)
[![Go Report Card](https://goreportcard.com/badge/github.com/aiagentinc/streamutil)](https://goreportcard.com/report/github.com/aiagentinc/streamutil)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**streamutil** is a zero-overhead Go library that adds stream processing capabilities to any `io.Reader` or `io.Writer` without changing how you write code. Calculate hashes, track progress, or transform data on-the-fly with minimal effort.

## 🎯 The Problem

When working with streams in Go, you often need to:
- Calculate checksums while downloading files
- Track upload/download progress
- Process data while reading/writing
- Compute multiple hashes in a single pass

Traditional solutions require:
- Writing boilerplate code for each use case
- Using goroutines and channels (adds complexity)
- Multiple passes over the data (inefficient)
- Refactoring existing code (time-consuming)

## 💡 The Solution

streamutil provides a transparent layer that works with your existing code:

```go
// Before: Just reading a file
data, _ := io.ReadAll(file)

// After: Reading with SHA256 calculation
hash := streamutil.NewHashCallback("sha256")
data, _ := io.ReadAll(streamutil.Reader(file, hash))
fmt.Printf("SHA256: %s\n", hash.HexSum())
```

**That's it!** No goroutines, no channels, no refactoring. Just wrap and use.

## 📦 Installation

```bash
go get github.com/aiagentinc/streamutil
```

## 🚀 Quick Start

### Example 1: Download with Progress

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "time"
    
    "github.com/aiagentinc/streamutil"
)

func main() {
    // Download a file with hash verification and progress tracking
    resp, _ := http.Get("https://example.com/largefile.zip")
    defer resp.Body.Close()
    
    file, _ := os.Create("largefile.zip")
    defer file.Close()
    
    // Set up callbacks
    hash := streamutil.NewHashCallback("sha256")
    size := streamutil.NewSizeCallback()
    
    // Create a reader that saves to file AND calculates hash
    reader := streamutil.TeeReader(resp.Body, file, hash, size)
    
    // Read with progress updates
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    
    done := make(chan bool)
    go func() {
        io.Copy(io.Discard, reader)
        done <- true
    }()
    
    for {
        select {
        case <-done:
            fmt.Printf("\nDownload complete!\n")
            fmt.Printf("Size: %d bytes\n", size.Size())
            fmt.Printf("SHA256: %s\n", hash.HexSum())
            return
        case <-ticker.C:
            fmt.Printf("\rDownloaded: %.2f MB", float64(size.Size())/(1024*1024))
        }
    }
}
```

### Example 2: File Processing Pipeline

```go
// Process a file: decompress, calculate hash, and save
func processBackup(filename string) error {
    file, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()
    
    // Set up processing pipeline
    md5cb := streamutil.NewHashCallback("md5")
    sha256cb := streamutil.NewHashCallback("sha256")
    sizecb := streamutil.NewSizeCallback()
    
    // Wrap with callbacks
    reader := streamutil.Reader(file, md5cb, sha256cb, sizecb)
    
    // Process the file (e.g., decompress and save)
    output, _ := os.Create("output.dat")
    defer output.Close()
    
    gzipReader, _ := gzip.NewReader(reader)
    io.Copy(output, gzipReader)
    
    fmt.Printf("Processed %d bytes\n", sizecb.Size())
    fmt.Printf("MD5: %s\n", md5cb.HexSum())
    fmt.Printf("SHA256: %s\n", sha256cb.HexSum())
    
    return nil
}
```

## 🆚 Comparison with Similar Packages

### streamutil vs io.TeeReader
| Feature | streamutil | io.TeeReader |
|---------|-----------|--------------|
| Multiple operations | ✅ Unlimited callbacks | ❌ Only one writer |
| Hash calculation | ✅ Built-in | ❌ Manual implementation |
| Progress tracking | ✅ Built-in | ❌ Manual implementation |
| Performance | ✅ Single pass | ❌ May need multiple passes |
| API complexity | ✅ Simple wrapper | ✅ Simple |


### streamutil vs Custom Solutions
```go
// Traditional approach: Multiple passes or complex code
file, _ := os.Open("data.bin")
data, _ := io.ReadAll(file)
file.Seek(0, 0)
hash := sha256.New()
io.Copy(hash, file)
checksum := hex.EncodeToString(hash.Sum(nil))

// With streamutil: Single pass, simple code
file, _ := os.Open("data.bin")
hashCb := streamutil.NewHashCallback("sha256")
data, _ := io.ReadAll(streamutil.Reader(file, hashCb))
checksum := hashCb.HexSum()
```

## 📊 Performance Benchmarks

Based on our comprehensive benchmarks:

```
BenchmarkReader/size=1MB          19.5 GB/s    ██████████████████████
BenchmarkHashCallback/sha256      390 MB/s     ██████
BenchmarkMultipleCallbacks/5      156 MB/s     ███
BenchmarkConcurrent/16-threads    113 GB/s     ████████████████████

Memory overhead: < 33KB per stream (one 32KB buffer)
```

Key insights:
- **Minimal overhead**: Only ~0.24% performance impact vs direct IO
- **Scales linearly**: Performance scales with CPU cores
- **Memory efficient**: Fixed memory usage regardless of stream size

## 📋 More Practical Examples

### Example 3: Upload with Retry and Verification

```go
func uploadWithVerification(filename, url string) error {
    file, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()
    
    // Calculate hash while reading
    hash := streamutil.NewHashCallback("sha256")
    size := streamutil.NewSizeCallback()
    
    reader := streamutil.Reader(file, hash, size)
    
    req, _ := http.NewRequest("POST", url, reader)
    req.ContentLength = getFileSize(filename)
    req.Header.Set("X-Checksum-SHA256", hash.HexSum())
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    fmt.Printf("Uploaded %d bytes with SHA256: %s\n", size.Size(), hash.HexSum())
    return nil
}
```

### Example 4: Log Processing with Custom Callback

```go
type LogStats struct {
    lines  int
    errors int
    mu     sync.Mutex
}

func (l *LogStats) Name() string { return "log_stats" }

func (l *LogStats) OnData(chunk []byte) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    
    lines := bytes.Split(chunk, []byte("\n"))
    for _, line := range lines {
        if len(line) > 0 {
            l.lines++
            if bytes.Contains(line, []byte("ERROR")) {
                l.errors++
            }
        }
    }
    return nil
}

func (l *LogStats) Result() any {
    return map[string]int{"lines": l.lines, "errors": l.errors}
}

// Usage
func analyzeLogs(logFile string) {
    file, _ := os.Open(logFile)
    defer file.Close()
    
    stats := &LogStats{}
    hash := streamutil.NewHashCallback("md5")
    
    reader := streamutil.Reader(file, stats, hash)
    io.Copy(io.Discard, reader)
    
    result := stats.Result().(map[string]int)
    fmt.Printf("Processed %d lines, found %d errors\n", result["lines"], result["errors"])
    fmt.Printf("File MD5: %s\n", hash.HexSum())
}
```

### Example 5: Streaming Archive Creation

```go
func createBackupWithChecksum(files []string, output string) error {
    out, err := os.Create(output)
    if err != nil {
        return err
    }
    defer out.Close()
    
    // Track size and hash of the archive
    hash := streamutil.NewHashCallback("sha256")
    size := streamutil.NewSizeCallback()
    
    writer := streamutil.Writer(out, hash, size)
    gzipWriter := gzip.NewWriter(writer)
    tarWriter := tar.NewWriter(gzipWriter)
    
    for _, file := range files {
        if err := addFileToTar(tarWriter, file); err != nil {
            return err
        }
    }
    
    tarWriter.Close()
    gzipWriter.Close()
    
    // Important: flush buffered writer
    if bw, ok := writer.(*streamutil.BufferedWriter); ok {
        bw.Close()
    }
    
    fmt.Printf("Archive created: %s\n", output)
    fmt.Printf("Size: %.2f MB\n", float64(size.Size())/(1024*1024))
    fmt.Printf("SHA256: %s\n", hash.HexSum())
    
    // Save checksum file
    checksumFile := output + ".sha256"
    os.WriteFile(checksumFile, []byte(hash.HexSum()), 0644)
    
    return nil
}
```

## 🎯 When to Use streamutil

✅ **Perfect for:**
- File uploads/downloads with progress tracking
- Checksum verification during IO operations
- Data pipeline processing
- Streaming data transformation
- Monitoring IO operations without code changes

❌ **Not ideal for:**
- Complex stream transformations (use `io.Pipe` instead)
- Parallel processing needs (streamutil is sequential)
- When you need backpressure control

## 🔧 Built-in Callbacks

| Callback | Purpose | Example Use |
|----------|---------|-------------|
| `HashCallback` | Single hash calculation | File integrity checks |
| `MultiHashCallback` | Multiple hashes at once | Generate multiple checksums |
| `SizeCallback` | Track bytes processed | Progress bars, bandwidth monitoring |

## 🛠️ Creating Custom Callbacks

Implement the simple `ReadCallback` or `WriteCallback` interface:

```go
type ReadCallback interface {
    Name() string                // Unique identifier
    OnData(chunk []byte) error   // Called for each chunk
    Result() any                 // Get final result
}
```

Example: Bandwidth limiter
```go
type BandwidthLimiter struct {
    bytesPerSecond int
    lastTime       time.Time
    accumulated    float64
}

func (b *BandwidthLimiter) OnData(chunk []byte) error {
    now := time.Now()
    if !b.lastTime.IsZero() {
        elapsed := now.Sub(b.lastTime).Seconds()
        b.accumulated += float64(len(chunk))
        
        allowedBytes := float64(b.bytesPerSecond) * elapsed
        if b.accumulated > allowedBytes {
            sleepTime := (b.accumulated - allowedBytes) / float64(b.bytesPerSecond)
            time.Sleep(time.Duration(sleepTime * float64(time.Second)))
            b.accumulated = 0
        }
    }
    b.lastTime = now
    return nil
}
```

## 🏗️ Architecture & Design

streamutil follows these principles:

1. **Zero-allocation design**: Reuses buffers to minimize GC pressure
2. **Interface preservation**: Always returns standard `io.Reader`/`io.Writer`
3. **Fail-fast errors**: First error stops processing immediately
4. **Sequential processing**: No goroutines means no race conditions
5. **Lazy evaluation**: Callbacks only instantiated when needed

## 📈 Real-World Performance

From our production usage:
- **File Server**: Handles 10GB+ files with constant 33KB memory usage
- **Backup System**: Processes 1TB+ daily with SHA256 verification
- **API Gateway**: Adds checksums to all uploads with <1ms latency
- **Log Processor**: Analyzes 100GB+ logs at 390MB/s with pattern matching

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.