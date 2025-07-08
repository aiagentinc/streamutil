package streamutil

import (
	"bufio"
	"errors"
	"io"
	"sync/atomic"
)

// BufferedWriter wraps an io.Writer (optionally WriterAt)
// and executes callbacks sequentially for every block.
type BufferedWriter struct {
	dst       io.Writer
	dstAt     io.WriterAt
	buf       *bufio.Writer
	callbacks []WriteCallback
	err       error
	closed    atomic.Bool
}

// NewWriter returns a *BufferedWriter with an internal 32 KiB buffer.
func NewWriter(w io.Writer, cbs []WriteCallback) *BufferedWriter {
	var wa io.WriterAt
	if v, ok := w.(io.WriterAt); ok {
		wa = v
	}
	return &BufferedWriter{
		dst:       w,
		dstAt:     wa,
		buf:       bufio.NewWriterSize(w, 32*1024),
		callbacks: cbs,
	}
}

// Write implements io.Writer.
func (bw *BufferedWriter) Write(p []byte) (int, error) {
	if bw.err != nil {
		return 0, bw.err
	}
	n, err := bw.buf.Write(p)
	if n > 0 && len(bw.callbacks) > 0 {
		if cbErr := bw.dispatch(p[:n]); cbErr != nil {
			bw.err = cbErr
			return n, cbErr
		}
	}
	return n, err
}

// Flush ensures all buffered data reaches the underlying writer.
// (Expose this if your callers need explicit control.)
func (bw *BufferedWriter) Flush() error {
	if bw.err != nil {
		return bw.err
	}
	if err := bw.buf.Flush(); err != nil {
		bw.err = err
	}
	return bw.err
}

// WriteAt passes through when the underlying supports it.
func (bw *BufferedWriter) WriteAt(p []byte, off int64) (int, error) {
	if bw.dstAt == nil {
		return 0, errors.New("WriteAt not supported")
	}
	if bw.err != nil {
		return 0, bw.err
	}
	n, err := bw.dstAt.WriteAt(p, off)
	if n > 0 && len(bw.callbacks) > 0 {
		if cbErr := bw.dispatch(p[:n]); cbErr != nil {
			bw.err = cbErr
			return n, cbErr
		}
	}
	return n, err
}

// Results returns a snapshot of each callback's current state.
func (bw *BufferedWriter) Results() map[string]any {
	out := make(map[string]any, len(bw.callbacks))
	for _, cb := range bw.callbacks {
		out[cb.Name()] = cb.Result()
	}
	return out
}

func (bw *BufferedWriter) dispatch(chunk []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("callback panic: " + formatPanic(r))
		}
	}()

	for _, cb := range bw.callbacks {
		if err := cb.OnData(chunk); err != nil {
			return err
		}
	}
	return nil
}

// Close flushes any buffered data and closes the writer if it implements io.Closer.
func (bw *BufferedWriter) Close() error {
	if !bw.closed.CompareAndSwap(false, true) {
		return nil
	}

	// Flush any remaining buffered data
	if err := bw.Flush(); err != nil {
		return err
	}

	// Close underlying writer if it supports it
	if closer, ok := bw.dst.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}
