package streamutil

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"sync/atomic"
)

// HashCallback computes hash while data passes through.
type HashCallback struct {
	name string
	h    hash.Hash
}

// NewHashCallback creates a callback for the specified algorithm.
// Supported algorithms: "md5", "sha1", "sha256", "sha512"
func NewHashCallback(algorithm string) *HashCallback {
	var h hash.Hash
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		// Default to sha256 if unknown
		h = sha256.New()
		algorithm = "sha256"
	}
	return &HashCallback{name: algorithm, h: h}
}

func (hc *HashCallback) Name() string { return hc.name }

func (hc *HashCallback) OnData(chunk []byte) error {
	_, _ = hc.h.Write(chunk)
	return nil
}

func (hc *HashCallback) Result() any { return hc.h.Sum(nil) }

// HexSum returns the hash as a hex string
func (hc *HashCallback) HexSum() string {
	return hex.EncodeToString(hc.h.Sum(nil))
}

// SizeCallback tracks the number of bytes processed.
type SizeCallback struct {
	size int64
}

func NewSizeCallback() *SizeCallback { return &SizeCallback{} }

func (sc *SizeCallback) Name() string { return "size" }

func (sc *SizeCallback) OnData(chunk []byte) error {
	atomic.AddInt64(&sc.size, int64(len(chunk)))
	return nil
}

func (sc *SizeCallback) Result() any { return atomic.LoadInt64(&sc.size) }

// Size returns the total bytes processed
func (sc *SizeCallback) Size() int64 { return atomic.LoadInt64(&sc.size) }

// MultiHashCallback computes multiple hashes in one pass.
type MultiHashCallback struct {
	hashes map[string]*HashCallback
}

// NewMultiHashCallback creates a callback that computes multiple hashes.
func NewMultiHashCallback(algorithms ...string) *MultiHashCallback {
	if len(algorithms) == 0 {
		algorithms = []string{"sha256"} // default
	}

	mh := &MultiHashCallback{
		hashes: make(map[string]*HashCallback),
	}

	for _, algo := range algorithms {
		mh.hashes[algo] = NewHashCallback(algo)
	}

	return mh
}

func (mh *MultiHashCallback) Name() string { return "multi_hash" }

func (mh *MultiHashCallback) OnData(chunk []byte) error {
	for _, h := range mh.hashes {
		if err := h.OnData(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (mh *MultiHashCallback) Result() any {
	results := make(map[string]string)
	for algo, h := range mh.hashes {
		results[algo] = h.HexSum()
	}
	return results
}

// Get returns the hex hash for a specific algorithm
func (mh *MultiHashCallback) Get(algorithm string) string {
	if h, ok := mh.hashes[algorithm]; ok {
		return h.HexSum()
	}
	return ""
}

// GetAll returns all hashes as hex strings
func (mh *MultiHashCallback) GetAll() map[string]string {
	results := make(map[string]string)
	for algo, h := range mh.hashes {
		results[algo] = h.HexSum()
	}
	return results
}
