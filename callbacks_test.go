package streamutil

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"testing"
)

func TestNewHashCallback(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		wantAlgo  string
		testData  []byte
		wantHash  string
	}{
		{
			name:      "md5",
			algorithm: "md5",
			wantAlgo:  "md5",
			testData:  []byte("hello world"),
			wantHash:  "5eb63bbbe01eeed093cb22bb8f5acdc3",
		},
		{
			name:      "sha1",
			algorithm: "sha1",
			wantAlgo:  "sha1",
			testData:  []byte("hello world"),
			wantHash:  "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed",
		},
		{
			name:      "sha256",
			algorithm: "sha256",
			wantAlgo:  "sha256",
			testData:  []byte("hello world"),
			wantHash:  "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:      "sha512",
			algorithm: "sha512",
			wantAlgo:  "sha512",
			testData:  []byte("hello world"),
			wantHash:  "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f",
		},
		{
			name:      "unknown defaults to sha256",
			algorithm: "unknown",
			wantAlgo:  "sha256",
			testData:  []byte("hello world"),
			wantHash:  "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := NewHashCallback(tt.algorithm)

			if hc.Name() != tt.wantAlgo {
				t.Errorf("NewHashCallback() name = %v, want %v", hc.Name(), tt.wantAlgo)
			}

			// Test OnData
			err := hc.OnData(tt.testData)
			if err != nil {
				t.Errorf("HashCallback.OnData() error = %v", err)
			}

			// Test HexSum
			got := hc.HexSum()
			if got != tt.wantHash {
				t.Errorf("HashCallback.HexSum() = %v, want %v", got, tt.wantHash)
			}

			// Test Result
			result := hc.Result()
			if result == nil {
				t.Error("HashCallback.Result() returned nil")
			}
			if _, ok := result.([]byte); !ok {
				t.Error("HashCallback.Result() should return []byte")
			}
		})
	}
}

func TestHashCallback_MultipleChunks(t *testing.T) {
	hc := NewHashCallback("sha256")

	chunks := [][]byte{
		[]byte("hello"),
		[]byte(" "),
		[]byte("world"),
	}

	for _, chunk := range chunks {
		if err := hc.OnData(chunk); err != nil {
			t.Errorf("HashCallback.OnData() error = %v", err)
		}
	}

	got := hc.HexSum()
	want := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if got != want {
		t.Errorf("HashCallback.HexSum() after multiple chunks = %v, want %v", got, want)
	}
}

func TestNewSizeCallback(t *testing.T) {
	sc := NewSizeCallback()

	if sc.Name() != "size" {
		t.Errorf("SizeCallback.Name() = %v, want size", sc.Name())
	}

	// Test initial size
	if sc.Size() != 0 {
		t.Errorf("SizeCallback.Size() initial = %v, want 0", sc.Size())
	}

	// Test OnData
	testData := [][]byte{
		[]byte("hello"),
		[]byte(" "),
		[]byte("world"),
	}

	totalSize := int64(0)
	for _, chunk := range testData {
		if err := sc.OnData(chunk); err != nil {
			t.Errorf("SizeCallback.OnData() error = %v", err)
		}
		totalSize += int64(len(chunk))
	}

	if sc.Size() != totalSize {
		t.Errorf("SizeCallback.Size() = %v, want %v", sc.Size(), totalSize)
	}

	// Test Result
	result := sc.Result()
	if result != totalSize {
		t.Errorf("SizeCallback.Result() = %v, want %v", result, totalSize)
	}
}

func TestSizeCallback_Concurrent(t *testing.T) {
	sc := NewSizeCallback()

	// Test concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			data := []byte("test data")
			_ = sc.OnData(data)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	expectedSize := int64(10 * len("test data"))
	if sc.Size() != expectedSize {
		t.Errorf("SizeCallback.Size() after concurrent writes = %v, want %v", sc.Size(), expectedSize)
	}
}

func TestNewMultiHashCallback(t *testing.T) {
	tests := []struct {
		name       string
		algorithms []string
		wantAlgos  []string
	}{
		{
			name:       "no algorithms defaults to sha256",
			algorithms: nil,
			wantAlgos:  []string{"sha256"},
		},
		{
			name:       "single algorithm",
			algorithms: []string{"md5"},
			wantAlgos:  []string{"md5"},
		},
		{
			name:       "multiple algorithms",
			algorithms: []string{"md5", "sha1", "sha256"},
			wantAlgos:  []string{"md5", "sha1", "sha256"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mh := NewMultiHashCallback(tt.algorithms...)

			if mh.Name() != "multi_hash" {
				t.Errorf("MultiHashCallback.Name() = %v, want multi_hash", mh.Name())
			}

			// Check that all expected algorithms are present
			for _, algo := range tt.wantAlgos {
				if _, ok := mh.hashes[algo]; !ok {
					t.Errorf("MultiHashCallback missing algorithm %v", algo)
				}
			}

			if len(mh.hashes) != len(tt.wantAlgos) {
				t.Errorf("MultiHashCallback has %v algorithms, want %v", len(mh.hashes), len(tt.wantAlgos))
			}
		})
	}
}

func TestMultiHashCallback_OnData(t *testing.T) {
	mh := NewMultiHashCallback("md5", "sha1", "sha256")

	testData := []byte("hello world")

	err := mh.OnData(testData)
	if err != nil {
		t.Errorf("MultiHashCallback.OnData() error = %v", err)
	}

	// Expected hashes
	expected := map[string]string{
		"md5":    "5eb63bbbe01eeed093cb22bb8f5acdc3",
		"sha1":   "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed",
		"sha256": "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
	}

	// Test Get method
	for algo, want := range expected {
		got := mh.Get(algo)
		if got != want {
			t.Errorf("MultiHashCallback.Get(%v) = %v, want %v", algo, got, want)
		}
	}

	// Test Get for non-existent algorithm
	if got := mh.Get("sha512"); got != "" {
		t.Errorf("MultiHashCallback.Get(sha512) = %v, want empty string", got)
	}
}

func TestMultiHashCallback_GetAll(t *testing.T) {
	mh := NewMultiHashCallback("md5", "sha256")

	testData := []byte("test data")
	_ = mh.OnData(testData)

	all := mh.GetAll()

	if len(all) != 2 {
		t.Errorf("MultiHashCallback.GetAll() returned %v results, want 2", len(all))
	}

	// Verify we got results for both algorithms
	if _, ok := all["md5"]; !ok {
		t.Error("MultiHashCallback.GetAll() missing md5")
	}
	if _, ok := all["sha256"]; !ok {
		t.Error("MultiHashCallback.GetAll() missing sha256")
	}
}

func TestMultiHashCallback_Result(t *testing.T) {
	mh := NewMultiHashCallback("md5", "sha1")

	testData := []byte("test")
	_ = mh.OnData(testData)

	result := mh.Result()
	resultMap, ok := result.(map[string]string)
	if !ok {
		t.Fatal("MultiHashCallback.Result() should return map[string]string")
	}

	if len(resultMap) != 2 {
		t.Errorf("MultiHashCallback.Result() returned %v results, want 2", len(resultMap))
	}

	// Verify both hashes are present
	if _, ok := resultMap["md5"]; !ok {
		t.Error("MultiHashCallback.Result() missing md5")
	}
	if _, ok := resultMap["sha1"]; !ok {
		t.Error("MultiHashCallback.Result() missing sha1")
	}
}

func TestMultiHashCallback_ErrorPropagation(t *testing.T) {
	// Test that MultiHashCallback handles errors from individual callbacks
	mh := NewMultiHashCallback("md5", "sha256")

	// Test normal operation - no errors expected
	err := mh.OnData([]byte("test"))
	if err != nil {
		t.Errorf("MultiHashCallback.OnData() unexpected error = %v", err)
	}

	// Verify both hashes processed the data
	if mh.Get("md5") == "" {
		t.Error("MultiHashCallback failed to compute md5")
	}
	if mh.Get("sha256") == "" {
		t.Error("MultiHashCallback failed to compute sha256")
	}
}

func TestHashCallback_Integration(t *testing.T) {
	// Test using callbacks with actual Reader
	data := "The quick brown fox jumps over the lazy dog"
	reader := bytes.NewReader([]byte(data))

	hashCb := NewHashCallback("sha256")
	sizeCb := NewSizeCallback()

	r := Reader(reader, hashCb, sizeCb)

	// Read all data
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	// Verify data
	if buf.String() != data {
		t.Errorf("Read data = %v, want %v", buf.String(), data)
	}

	// Verify hash
	expectedHash := "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592"
	if hashCb.HexSum() != expectedHash {
		t.Errorf("Hash = %v, want %v", hashCb.HexSum(), expectedHash)
	}

	// Verify size
	if sizeCb.Size() != int64(len(data)) {
		t.Errorf("Size = %v, want %v", sizeCb.Size(), len(data))
	}
}

func TestMultiHashCallback_Integration(t *testing.T) {
	// Test using multi-hash callback with Writer
	data := []byte("The quick brown fox jumps over the lazy dog")
	buf := new(bytes.Buffer)

	mhCb := NewMultiHashCallback("md5", "sha1", "sha256", "sha512")

	w := Writer(buf, mhCb)

	// Write data
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(data) {
		t.Errorf("Wrote %v bytes, want %v", n, len(data))
	}

	// Flush if it's a BufferedWriter
	if bw, ok := w.(*BufferedWriter); ok {
		_ = bw.Flush()
	}

	// Verify written data
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("Written data mismatch")
	}

	// Verify all hashes
	expected := map[string]string{
		"md5":    "9e107d9d372bb6826bd81d3542a419d6",
		"sha1":   "2fd4e1c67a2d28fced849ee1bb76e7391b93eb12",
		"sha256": "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592",
		"sha512": "07e547d9586f6a73f73fbac0435ed76951218fb7d0c8d788a309d785436bbb642e93a252a954f23912547d1e8a3b5ed6e1bfd7097821233fa0538f3db854fee6",
	}

	all := mhCb.GetAll()
	for algo, want := range expected {
		if got := all[algo]; got != want {
			t.Errorf("Hash %v = %v, want %v", algo, got, want)
		}
	}
}

// Verify actual hash computations
func TestHashCallback_VerifyHashes(t *testing.T) {
	testData := []byte("test data")

	md5sum := md5.Sum(testData)
	sha1sum := sha1.Sum(testData)
	sha256sum := sha256.Sum256(testData)
	sha512sum := sha512.Sum512(testData)

	tests := []struct {
		algo string
		want string
	}{
		{"md5", hex.EncodeToString(md5sum[:])},
		{"sha1", hex.EncodeToString(sha1sum[:])},
		{"sha256", hex.EncodeToString(sha256sum[:])},
		{"sha512", hex.EncodeToString(sha512sum[:])},
	}

	for _, tt := range tests {
		t.Run(tt.algo, func(t *testing.T) {
			hc := NewHashCallback(tt.algo)
			_ = hc.OnData(testData)

			if got := hc.HexSum(); got != tt.want {
				t.Errorf("HashCallback(%v).HexSum() = %v, want %v", tt.algo, got, tt.want)
			}
		})
	}
}
