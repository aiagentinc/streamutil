# CI/CD Optimization Guide

## Overview

To ensure efficient CI/CD workflows and avoid triggering vendor service limitations, we have optimized our performance tests specifically for CI environments.

## Current Status ✅

All CI/CD workflows are configured and passing:
- **Code Quality** (lint.yml): gofmt, go vet, golangci-lint, govulncheck
- **Testing** (test.yml): Unit tests with race detection on Go 1.22-1.24
- **Security** (security.yml): Weekly vulnerability scans
- **Performance** (performance.yml): Optimized stress and throughput tests
- **Benchmarks** (benchmark.yml): Performance tracking and PR comparisons

## Optimization Strategy

### 1. Environment Detection
Tests automatically adjust parameters when running in CI:
```go
if os.Getenv("CI") == "true" {
    // Use CI-optimized parameters
}
```

### 2. Reduced Durations
- **Stress test duration**: 5s → 2s (local) → 0.5s (CI)
- **Throughput test duration**: 10s → 3s (local) → 0.6s (CI)
- **Scalability test duration**: 5s → 2s (local) → 0.4s (CI)
- **Benchmark duration**: Full dataset → Limited to 100KB files in CI

### 3. Reduced Iterations
- **Memory leak test iterations**: 1000 → 500 (local) → 50 (CI)
- **Pipeline performance iterations**: 100 → 50 (local) → 10 (CI)
- **Latency test operations**: 10000 → 5000 (local) → 500 (CI)

### 4. Reduced Data Sizes
- **Benchmark data**: Up to 10MB → Up to 100KB in CI
- **Concurrent tests**: 1MB → 100KB in CI
- **Buffer size tests**: 10MB → 1MB in CI
- **Memory allocation tests**: 1MB & 10MB → 1MB only in CI

### 5. Skipped Unstable Tests
- Resource contention tests are automatically skipped in CI to avoid environment-related flakiness

### 6. Reduced Concurrency Levels
- **Benchmark concurrency**: 1,2,4,8,16 → 1,2,4 in CI
- **Stress test concurrency**: Maintains all levels but with shorter durations

## Workflow Configuration

### Testing Strategy
1. **Unit tests**: Use `-short` flag to skip long-running tests
2. **Performance tests**: Separate workflow, runs only when necessary
3. **Benchmarks**: Scheduled runs with PR comparisons

### Trigger Conditions
- **Every push**: Unit tests and code quality checks
- **Pull requests**: Additional benchmark comparisons
- **Scheduled**: Weekly full performance and security scans

### Timeout Settings
- Global job timeout: 30 minutes
- Individual test timeouts: 2-10 minutes based on test type

## Local vs CI Testing

### CI Environment (Optimized)
```bash
# Quick validation, completes in 2-3 seconds
CI=true go test -run=TestConcurrentStress/LowConcurrency -v

# Throughput: ~6.5 GB/s (0.5-second test)
```

### Local Environment (Full Testing)
```bash
# Complete stress test, 5+ seconds
go test -run=TestConcurrentStress -v

# Throughput: ~6.7 GB/s (5-second test)
```

## Performance Impact

Optimized CI tests still provide effective performance metrics:
- Catch significant performance regressions (tests complete in <2 seconds)
- Verify concurrency safety with race detection
- Detect memory leak trends (50 iterations sufficient)
- Enable benchmark comparisons (focused on smaller data sizes)
- Total CI test time: ~10-15 seconds vs ~2-5 minutes locally

## Pre-commit Checklist

```bash
# Format code
go fmt ./...

# Run static analysis
go vet ./...

# Run tests with race detection
go test -short -race ./...

# Tidy dependencies
go mod tidy
```

## Recommendations

1. **Development phase**: Run complete local tests
2. **PR phase**: Rely on CI's quick feedback
3. **Before release**: Manually trigger full performance test workflow
4. **Regular review**: Check weekly performance trend reports

## Troubleshooting CI Failures

1. **Formatting issues**: Run `go fmt ./...`
2. **Test failures**: Check for race conditions with `-race` flag
3. **Performance regression**: Review benchmark comparison results
4. **Security issues**: Check `go list -m all` for vulnerable dependencies

All workflows support manual dispatch for debugging purposes.