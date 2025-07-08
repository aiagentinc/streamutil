package streamutil

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type mockWriter struct {
	buf         bytes.Buffer
	err         error
	atErr       error
	writeAtData map[int64][]byte
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.buf.Write(p)
}

func (m *mockWriter) WriteAt(p []byte, off int64) (n int, err error) {
	if m.atErr != nil {
		return 0, m.atErr
	}
	if m.writeAtData == nil {
		m.writeAtData = make(map[int64][]byte)
	}
	m.writeAtData[off] = append([]byte(nil), p...)
	return len(p), nil
}

type mockCloser struct {
	mockWriter
	closed   bool
	closeErr error
}

func (m *mockCloser) Close() error {
	m.closed = true
	return m.closeErr
}

type mockWriteCallback struct {
	name     string
	chunks   [][]byte
	err      error
	result   interface{}
	panicMsg string
}

func (mc *mockWriteCallback) Name() string {
	return mc.name
}

func (mc *mockWriteCallback) OnData(chunk []byte) error {
	if mc.panicMsg != "" {
		panic(mc.panicMsg)
	}
	if mc.err != nil {
		return mc.err
	}
	chunkCopy := make([]byte, len(chunk))
	copy(chunkCopy, chunk)
	mc.chunks = append(mc.chunks, chunkCopy)
	return nil
}

func (mc *mockWriteCallback) Result() any {
	if mc.result != nil {
		return mc.result
	}
	return len(mc.chunks)
}

func TestWriter(t *testing.T) {
	tests := []struct {
		name      string
		writer    io.Writer
		callbacks []WriteCallback
		wantData  string
		wantErr   bool
	}{
		{
			name:      "no callbacks returns original writer",
			writer:    &bytes.Buffer{},
			callbacks: nil,
			wantData:  "test data",
			wantErr:   false,
		},
		{
			name:      "empty callbacks returns original writer",
			writer:    &bytes.Buffer{},
			callbacks: []WriteCallback{},
			wantData:  "test data",
			wantErr:   false,
		},
		{
			name:      "with callbacks returns BufferedWriter",
			writer:    &bytes.Buffer{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test"}},
			wantData:  "test data",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Writer(tt.writer, tt.callbacks...)
			n, err := w.Write([]byte(tt.wantData))
			if (err != nil) != tt.wantErr {
				t.Errorf("Writer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if n != len(tt.wantData) {
				t.Errorf("Writer() n = %v, want %v", n, len(tt.wantData))
			}
		})
	}
}

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name   string
		writer io.Writer
		cbs    []WriteCallback
	}{
		{
			name:   "creates BufferedWriter with callbacks",
			writer: &bytes.Buffer{},
			cbs:    []WriteCallback{&mockWriteCallback{name: "test"}},
		},
		{
			name:   "creates BufferedWriter without callbacks",
			writer: &bytes.Buffer{},
			cbs:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bw := NewWriter(tt.writer, tt.cbs)
			if bw == nil {
				t.Fatal("NewWriter() returned nil")
			}
			if bw.dst != tt.writer {
				t.Error("NewWriter() dst not set correctly")
			}
			if len(bw.callbacks) != len(tt.cbs) {
				t.Errorf("NewWriter() callbacks = %v, want %v", len(bw.callbacks), len(tt.cbs))
			}
		})
	}
}

func TestBufferedWriter_Write(t *testing.T) {
	tests := []struct {
		name        string
		writer      io.Writer
		callbacks   []WriteCallback
		writeData   string
		wantData    string
		wantErr     bool
		wantErrType error
	}{
		{
			name:      "writes data without callbacks",
			writer:    &bytes.Buffer{},
			callbacks: nil,
			writeData: "hello world",
			wantData:  "hello world",
			wantErr:   false,
		},
		{
			name:      "writes data with successful callback",
			writer:    &bytes.Buffer{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test"}},
			writeData: "hello world",
			wantData:  "hello world",
			wantErr:   false,
		},
		{
			name:      "callback error stops writing",
			writer:    &bytes.Buffer{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test", err: errors.New("callback error")}},
			writeData: "hello world",
			wantData:  "hello world",
			wantErr:   true,
		},
		{
			name:      "sticky error prevents further writes",
			writer:    &bytes.Buffer{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test", err: errors.New("sticky error")}},
			writeData: "hello world",
			wantErr:   true,
		},
		{
			name:      "empty write does nothing",
			writer:    &bytes.Buffer{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test"}},
			writeData: "",
			wantData:  "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bw := NewWriter(tt.writer, tt.callbacks)
			n, err := bw.Write([]byte(tt.writeData))

			if (err != nil) != tt.wantErr {
				t.Errorf("BufferedWriter.Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Flush to check data
			if err == nil {
				_ = bw.Flush()
			}

			if tt.wantData != "" && !tt.wantErr {
				var gotData string
				switch w := tt.writer.(type) {
				case *bytes.Buffer:
					gotData = w.String()
				case *mockWriter:
					gotData = w.buf.String()
				}
				if gotData != tt.wantData {
					t.Errorf("BufferedWriter.Write() data = %v, want %v", gotData, tt.wantData)
				}
			}

			// Test sticky error
			if tt.wantErr && bw.err != nil {
				n2, err2 := bw.Write([]byte("more"))
				if n2 != 0 || err2 != bw.err {
					t.Errorf("BufferedWriter.Write() sticky error not working: n=%v, err=%v", n2, err2)
				}
			}

			if err == nil && n != len(tt.writeData) {
				t.Errorf("BufferedWriter.Write() n = %v, want %v", n, len(tt.writeData))
			}
		})
	}
}

func TestBufferedWriter_Flush(t *testing.T) {
	tests := []struct {
		name      string
		writeData string
		stickyErr error
		wantErr   bool
	}{
		{
			name:      "successful flush",
			writeData: "test data",
			stickyErr: nil,
			wantErr:   false,
		},
		{
			name:      "flush with sticky error",
			writeData: "test data",
			stickyErr: errors.New("sticky error"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			bw := NewWriter(buf, nil)

			// Write some data
			_, _ = bw.Write([]byte(tt.writeData))

			// Set sticky error if needed
			if tt.stickyErr != nil {
				bw.err = tt.stickyErr
			}

			err := bw.Flush()
			if (err != nil) != tt.wantErr {
				t.Errorf("BufferedWriter.Flush() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify data was flushed (if no error)
			if !tt.wantErr && buf.String() != tt.writeData {
				t.Errorf("BufferedWriter.Flush() data = %v, want %v", buf.String(), tt.writeData)
			}
		})
	}
}

func TestBufferedWriter_WriteAt(t *testing.T) {
	tests := []struct {
		name      string
		writer    io.Writer
		callbacks []WriteCallback
		data      []byte
		offset    int64
		wantErr   bool
	}{
		{
			name:      "WriteAt not supported for regular writer",
			writer:    &bytes.Buffer{},
			callbacks: nil,
			data:      []byte("test"),
			offset:    0,
			wantErr:   true,
		},
		{
			name:      "WriteAt works with WriterAt",
			writer:    &mockWriter{},
			callbacks: nil,
			data:      []byte("hello"),
			offset:    10,
			wantErr:   false,
		},
		{
			name:      "WriteAt with callbacks",
			writer:    &mockWriter{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test"}},
			data:      []byte("world"),
			offset:    20,
			wantErr:   false,
		},
		{
			name:      "WriteAt with callback error",
			writer:    &mockWriter{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test", err: errors.New("callback error")}},
			data:      []byte("test"),
			offset:    0,
			wantErr:   true,
		},
		{
			name:      "WriteAt with sticky error",
			writer:    &mockWriter{},
			callbacks: []WriteCallback{&mockWriteCallback{name: "test"}},
			data:      []byte("test"),
			offset:    0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bw := NewWriter(tt.writer, tt.callbacks)

			// Set sticky error for specific test
			if tt.name == "WriteAt with sticky error" {
				bw.err = errors.New("sticky error")
			}

			n, err := bw.WriteAt(tt.data, tt.offset)

			if (err != nil) != tt.wantErr {
				t.Errorf("BufferedWriter.WriteAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if n != len(tt.data) {
					t.Errorf("BufferedWriter.WriteAt() n = %v, want %v", n, len(tt.data))
				}

				// Verify data was written at correct offset
				if mw, ok := tt.writer.(*mockWriter); ok {
					if written, exists := mw.writeAtData[tt.offset]; exists {
						if !bytes.Equal(written, tt.data) {
							t.Errorf("BufferedWriter.WriteAt() data mismatch: got %q, want %q", written, tt.data)
						}
					} else {
						t.Error("BufferedWriter.WriteAt() did not write data at expected offset")
					}
				}
			}
		})
	}
}

func TestBufferedWriter_Results(t *testing.T) {
	cb1 := &mockWriteCallback{name: "test1", result: "result1"}
	cb2 := &mockWriteCallback{name: "test2", result: 42}

	bw := NewWriter(&bytes.Buffer{}, []WriteCallback{cb1, cb2})

	results := bw.Results()

	if len(results) != 2 {
		t.Errorf("BufferedWriter.Results() returned %d results, want 2", len(results))
	}

	if results["test1"] != "result1" {
		t.Errorf("BufferedWriter.Results()[test1] = %v, want result1", results["test1"])
	}

	if results["test2"] != 42 {
		t.Errorf("BufferedWriter.Results()[test2] = %v, want 42", results["test2"])
	}
}

func TestBufferedWriter_dispatch(t *testing.T) {
	tests := []struct {
		name      string
		callbacks []WriteCallback
		chunk     []byte
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "dispatch with no callbacks",
			callbacks: nil,
			chunk:     []byte("test"),
			wantErr:   false,
		},
		{
			name: "dispatch with successful callbacks",
			callbacks: []WriteCallback{
				&mockWriteCallback{name: "cb1"},
				&mockWriteCallback{name: "cb2"},
			},
			chunk:   []byte("test"),
			wantErr: false,
		},
		{
			name: "dispatch stops on first error",
			callbacks: []WriteCallback{
				&mockWriteCallback{name: "cb1"},
				&mockWriteCallback{name: "cb2", err: errors.New("cb2 error")},
				&mockWriteCallback{name: "cb3"},
			},
			chunk:   []byte("test"),
			wantErr: true,
			errMsg:  "cb2 error",
		},
		{
			name: "dispatch handles panic with error",
			callbacks: []WriteCallback{
				&mockWriteCallback{name: "cb1", panicMsg: "panic error"},
			},
			chunk:   []byte("test"),
			wantErr: true,
			errMsg:  "callback panic: panic error",
		},
		{
			name: "dispatch handles panic with non-error",
			callbacks: []WriteCallback{
				&mockWriteCallback{name: "cb1", panicMsg: "string panic"},
			},
			chunk:   []byte("test"),
			wantErr: true,
			errMsg:  "callback panic: string panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bw := NewWriter(&bytes.Buffer{}, tt.callbacks)
			err := bw.dispatch(tt.chunk)

			if (err != nil) != tt.wantErr {
				t.Errorf("BufferedWriter.dispatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("BufferedWriter.dispatch() error = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestBufferedWriter_Close(t *testing.T) {
	tests := []struct {
		name       string
		writer     io.Writer
		writeData  string
		flushErr   error
		wantErr    bool
		wantClosed bool
	}{
		{
			name:       "close non-closer writer",
			writer:     &bytes.Buffer{},
			writeData:  "test data",
			wantErr:    false,
			wantClosed: false,
		},
		{
			name:       "close closer writer",
			writer:     &mockCloser{},
			writeData:  "test data",
			wantErr:    false,
			wantClosed: true,
		},
		{
			name:       "close with close error",
			writer:     &mockCloser{closeErr: errors.New("close error")},
			writeData:  "test data",
			wantErr:    true,
			wantClosed: true,
		},
		{
			name:       "close with flush error",
			writer:     &bytes.Buffer{},
			writeData:  "test data",
			flushErr:   errors.New("flush error"),
			wantErr:    true,
			wantClosed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bw := NewWriter(tt.writer, nil)

			// Write some data
			if tt.writeData != "" {
				_, _ = bw.Write([]byte(tt.writeData))
			}

			// Set flush error if needed
			if tt.flushErr != nil {
				bw.err = tt.flushErr
			}

			// Close
			err := bw.Close()

			if (err != nil) != tt.wantErr {
				t.Errorf("BufferedWriter.Close() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if closed
			if mc, ok := tt.writer.(*mockCloser); ok {
				if mc.closed != tt.wantClosed {
					t.Errorf("BufferedWriter.Close() closed = %v, want %v", mc.closed, tt.wantClosed)
				}
			}

			// Verify data was flushed (if no flush error)
			if tt.flushErr == nil && tt.writeData != "" {
				var gotData string
				switch w := tt.writer.(type) {
				case *bytes.Buffer:
					gotData = w.String()
				case *mockCloser:
					gotData = w.buf.String()
				}
				if gotData != tt.writeData {
					t.Errorf("BufferedWriter.Close() flushed data = %v, want %v", gotData, tt.writeData)
				}
			}
		})
	}
}

func TestBufferedWriter_Close_Idempotent(t *testing.T) {
	mc := &mockCloser{}
	bw := NewWriter(mc, nil)

	// First close
	err := bw.Close()
	if err != nil {
		t.Errorf("BufferedWriter.Close() first call error = %v", err)
	}
	if !mc.closed {
		t.Error("BufferedWriter.Close() should close writer on first call")
	}

	// Reset closed flag to check if Close is called again
	mc.closed = false

	// Second close should be no-op
	err = bw.Close()
	if err != nil {
		t.Errorf("BufferedWriter.Close() second call error = %v", err)
	}
	if mc.closed {
		t.Error("BufferedWriter.Close() should not call Close twice")
	}
}
