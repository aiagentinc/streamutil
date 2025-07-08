package streamutil

import (
	"io"
	"sync/atomic"
)

// Reader wraps any io.Reader with callbacks.
// It behaves exactly like the underlying reader, but executes callbacks for each chunk.
func Reader(r io.Reader, callbacks ...ReadCallback) io.Reader {
	if len(callbacks) == 0 {
		return r // no callbacks, return original reader
	}
	return NewReader(r, callbacks)
}

// Writer wraps any io.Writer with callbacks.
// It behaves exactly like the underlying writer, but executes callbacks for each chunk.
func Writer(w io.Writer, callbacks ...WriteCallback) io.Writer {
	if len(callbacks) == 0 {
		return w // no callbacks, return original writer
	}
	return NewWriter(w, callbacks)
}

// TeeReader returns a Reader that writes to w what it reads from r.
// All reads from r performed through it are matched with
// corresponding writes to w. Similar to io.TeeReader but with callback support.
func TeeReader(r io.Reader, w io.Writer, callbacks ...ReadCallback) io.Reader {
	// Create a write callback that tees to the writer
	teeCallback := &teeWriterCallback{w: w}

	// Combine with other callbacks
	allCallbacks := append([]ReadCallback{teeCallback}, callbacks...)

	return Reader(r, allCallbacks...)
}

// teeWriterCallback implements ReadCallback to tee data to a writer
type teeWriterCallback struct {
	w      io.Writer
	errPtr atomic.Pointer[error]
}

func (t *teeWriterCallback) Name() string { return "_tee_writer" }

func (t *teeWriterCallback) OnData(chunk []byte) error {
	if err := t.errPtr.Load(); err != nil {
		return *err
	}
	_, err := t.w.Write(chunk)
	if err != nil {
		t.errPtr.CompareAndSwap(nil, &err)
		return err
	}
	return nil
}

func (t *teeWriterCallback) Result() any { return nil }

// Ensure our types implement the standard interfaces
var (
	_ io.Reader   = (*BufferedReader)(nil)
	_ io.ReaderAt = (*BufferedReader)(nil)
	_ io.Writer   = (*BufferedWriter)(nil)
	_ io.WriterAt = (*BufferedWriter)(nil)
	_ io.Closer   = (*BufferedWriter)(nil)
)
