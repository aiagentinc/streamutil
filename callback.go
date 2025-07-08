package streamutil

// ReadCallback processes bytes read from upstream.
type ReadCallback interface {
	Name() string              // e.g. "sha256"
	OnData(chunk []byte) error // called for each block; chunk MUST NOT be modified
	Result() any               // final or interim result
}

// WriteCallback processes bytes written downstream.
type WriteCallback interface {
	Name() string
	OnData(chunk []byte) error // called for each block; chunk MUST NOT be modified
	Result() any
}
