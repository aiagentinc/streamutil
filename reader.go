package streamutil

import (
	"bufio"
	"errors"
	"io"
)

// BufferedReader wraps an io.Reader (optionally ReaderAt) and
// executes callbacks sequentially for every block.
type BufferedReader struct {
	src       io.Reader
	srcAt     io.ReaderAt
	buf       *bufio.Reader
	callbacks []ReadCallback
	err       error // first callback error (sticky)
}

// NewReader returns a *BufferedReader with an internal 32 KiB buffer.
// Pass nil or an empty slice to disable callbacks.
func NewReader(r io.Reader, cbs []ReadCallback) *BufferedReader {
	var ra io.ReaderAt
	if v, ok := r.(io.ReaderAt); ok {
		ra = v
	}
	return &BufferedReader{
		src:       r,
		srcAt:     ra,
		buf:       bufio.NewReaderSize(r, 32*1024),
		callbacks: cbs,
	}
}

// Read implements io.Reader.
func (br *BufferedReader) Read(p []byte) (int, error) {
	if br.err != nil {
		return 0, br.err
	}
	n, err := br.buf.Read(p)
	if n > 0 && len(br.callbacks) > 0 {
		if cbErr := br.dispatch(p[:n]); cbErr != nil {
			br.err = cbErr // remember first error
			return n, cbErr
		}
	}
	return n, err
}

// ReadAt passes through when the underlying supports it.
func (br *BufferedReader) ReadAt(p []byte, off int64) (int, error) {
	if br.srcAt == nil {
		return 0, errors.New("ReadAt not supported")
	}
	if br.err != nil {
		return 0, br.err
	}
	n, err := br.srcAt.ReadAt(p, off)
	if n > 0 && len(br.callbacks) > 0 {
		if cbErr := br.dispatch(p[:n]); cbErr != nil {
			br.err = cbErr
			return n, cbErr
		}
	}
	return n, err
}

// Results returns a snapshot of each callback's current state.
func (br *BufferedReader) Results() map[string]any {
	out := make(map[string]any, len(br.callbacks))
	for _, cb := range br.callbacks {
		out[cb.Name()] = cb.Result()
	}
	return out
}

// dispatch iterates callbacks sequentially.
func (br *BufferedReader) dispatch(chunk []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("callback panic: " + formatPanic(r))
		}
	}()

	for _, cb := range br.callbacks {
		if err := cb.OnData(chunk); err != nil {
			return err
		}
	}
	return nil
}

func formatPanic(r interface{}) string {
	switch v := r.(type) {
	case error:
		return v.Error()
	case string:
		return v
	default:
		return "unknown panic"
	}
}
