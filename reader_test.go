package streamutil

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type mockReader struct {
	data []byte
	err  error
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	if len(m.data) == 0 {
		return 0, io.EOF
	}
	n = copy(p, m.data)
	m.data = m.data[n:]
	return n, nil
}

type mockReaderAt struct {
	data []byte
	err  error
}

func (m *mockReaderAt) Read(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	if len(m.data) == 0 {
		return 0, io.EOF
	}
	n = copy(p, m.data)
	m.data = m.data[n:]
	return n, nil
}

func (m *mockReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	if off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n = copy(p, m.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return n, err
}

type testCallback struct {
	name     string
	chunks   [][]byte
	err      error
	result   interface{}
	panicMsg string
}

func (tc *testCallback) Name() string {
	return tc.name
}

func (tc *testCallback) OnData(chunk []byte) error {
	if tc.panicMsg != "" {
		panic(tc.panicMsg)
	}
	if tc.err != nil {
		return tc.err
	}
	chunkCopy := make([]byte, len(chunk))
	copy(chunkCopy, chunk)
	tc.chunks = append(tc.chunks, chunkCopy)
	return nil
}

func (tc *testCallback) Result() any {
	if tc.result != nil {
		return tc.result
	}
	return len(tc.chunks)
}

func TestReader(t *testing.T) {
	tests := []struct {
		name      string
		reader    io.Reader
		callbacks []ReadCallback
		wantData  string
		wantErr   bool
	}{
		{
			name:      "no callbacks returns original reader",
			reader:    strings.NewReader("test data"),
			callbacks: nil,
			wantData:  "test data",
			wantErr:   false,
		},
		{
			name:      "empty callbacks returns original reader",
			reader:    strings.NewReader("test data"),
			callbacks: []ReadCallback{},
			wantData:  "test data",
			wantErr:   false,
		},
		{
			name:      "with callbacks returns BufferedReader",
			reader:    strings.NewReader("test data"),
			callbacks: []ReadCallback{&testCallback{name: "test"}},
			wantData:  "test data",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Reader(tt.reader, tt.callbacks...)
			data, err := io.ReadAll(r)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(data) != tt.wantData {
				t.Errorf("Reader() data = %v, want %v", string(data), tt.wantData)
			}
		})
	}
}

func TestNewReader(t *testing.T) {
	tests := []struct {
		name   string
		reader io.Reader
		cbs    []ReadCallback
	}{
		{
			name:   "creates BufferedReader with callbacks",
			reader: strings.NewReader("test"),
			cbs:    []ReadCallback{&testCallback{name: "test"}},
		},
		{
			name:   "creates BufferedReader without callbacks",
			reader: strings.NewReader("test"),
			cbs:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := NewReader(tt.reader, tt.cbs)
			if br == nil {
				t.Fatal("NewReader() returned nil")
			}
			if br.src != tt.reader {
				t.Error("NewReader() src not set correctly")
			}
			if len(br.callbacks) != len(tt.cbs) {
				t.Errorf("NewReader() callbacks = %v, want %v", len(br.callbacks), len(tt.cbs))
			}
		})
	}
}

func TestBufferedReader_Read(t *testing.T) {
	tests := []struct {
		name        string
		reader      io.Reader
		callbacks   []ReadCallback
		readSize    int
		wantData    string
		wantErr     bool
		wantErrType error
	}{
		{
			name:      "reads data without callbacks",
			reader:    strings.NewReader("hello world"),
			callbacks: nil,
			readSize:  5,
			wantData:  "hello",
			wantErr:   false,
		},
		{
			name:      "reads data with successful callback",
			reader:    strings.NewReader("hello world"),
			callbacks: []ReadCallback{&testCallback{name: "test"}},
			readSize:  5,
			wantData:  "hello",
			wantErr:   false,
		},
		{
			name:      "callback error stops reading",
			reader:    strings.NewReader("hello world"),
			callbacks: []ReadCallback{&testCallback{name: "test", err: errors.New("callback error")}},
			readSize:  5,
			wantData:  "hello",
			wantErr:   true,
		},
		{
			name:      "sticky error prevents further reads",
			reader:    strings.NewReader("hello world"),
			callbacks: []ReadCallback{&testCallback{name: "test", err: errors.New("sticky error")}},
			readSize:  5,
			wantData:  "hello",
			wantErr:   true,
		},
		{
			name:      "handles reader error",
			reader:    &mockReader{err: errors.New("read error")},
			callbacks: nil,
			readSize:  5,
			wantErr:   true,
		},
		{
			name:      "handles EOF correctly",
			reader:    strings.NewReader("hi"),
			callbacks: []ReadCallback{&testCallback{name: "test"}},
			readSize:  5,
			wantData:  "hi",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := NewReader(tt.reader, tt.callbacks)
			buf := make([]byte, tt.readSize)
			n, err := br.Read(buf)

			if (err != nil && err != io.EOF) != tt.wantErr {
				t.Errorf("BufferedReader.Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if n > 0 && string(buf[:n]) != tt.wantData {
				t.Errorf("BufferedReader.Read() data = %v, want %v", string(buf[:n]), tt.wantData)
			}

			// Test sticky error
			if tt.wantErr && br.err != nil {
				n2, err2 := br.Read(buf)
				if n2 != 0 || err2 != br.err {
					t.Errorf("BufferedReader.Read() sticky error not working: n=%v, err=%v", n2, err2)
				}
			}
		})
	}
}

func TestBufferedReader_ReadAt(t *testing.T) {
	tests := []struct {
		name      string
		reader    io.Reader
		callbacks []ReadCallback
		offset    int64
		readSize  int
		wantData  string
		wantErr   bool
	}{
		{
			name:      "ReadAt not supported for regular reader",
			reader:    &mockReader{data: []byte("hello world")},
			callbacks: nil,
			offset:    0,
			readSize:  5,
			wantErr:   true,
		},
		{
			name:      "ReadAt works with ReaderAt",
			reader:    &mockReaderAt{data: []byte("hello world")},
			callbacks: nil,
			offset:    6,
			readSize:  5,
			wantData:  "world",
			wantErr:   false,
		},
		{
			name:      "ReadAt with callbacks",
			reader:    &mockReaderAt{data: []byte("hello world")},
			callbacks: []ReadCallback{&testCallback{name: "test"}},
			offset:    0,
			readSize:  5,
			wantData:  "hello",
			wantErr:   false,
		},
		{
			name:      "ReadAt with callback error",
			reader:    &mockReaderAt{data: []byte("hello world")},
			callbacks: []ReadCallback{&testCallback{name: "test", err: errors.New("callback error")}},
			offset:    0,
			readSize:  5,
			wantData:  "hello",
			wantErr:   true,
		},
		{
			name:      "ReadAt with sticky error",
			reader:    &mockReaderAt{data: []byte("hello world")},
			callbacks: []ReadCallback{&testCallback{name: "test"}},
			offset:    0,
			readSize:  5,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := NewReader(tt.reader, tt.callbacks)

			// Set sticky error for specific test
			if tt.name == "ReadAt with sticky error" {
				br.err = errors.New("sticky error")
			}

			buf := make([]byte, tt.readSize)
			n, err := br.ReadAt(buf, tt.offset)

			if (err != nil && err != io.EOF) != tt.wantErr {
				t.Errorf("BufferedReader.ReadAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if n > 0 && string(buf[:n]) != tt.wantData {
				t.Errorf("BufferedReader.ReadAt() data = %v, want %v", string(buf[:n]), tt.wantData)
			}
		})
	}
}

func TestBufferedReader_Results(t *testing.T) {
	cb1 := &testCallback{name: "test1", result: "result1"}
	cb2 := &testCallback{name: "test2", result: 42}

	br := NewReader(strings.NewReader("test"), []ReadCallback{cb1, cb2})

	results := br.Results()

	if len(results) != 2 {
		t.Errorf("BufferedReader.Results() returned %d results, want 2", len(results))
	}

	if results["test1"] != "result1" {
		t.Errorf("BufferedReader.Results()[test1] = %v, want result1", results["test1"])
	}

	if results["test2"] != 42 {
		t.Errorf("BufferedReader.Results()[test2] = %v, want 42", results["test2"])
	}
}

func TestBufferedReader_dispatch(t *testing.T) {
	tests := []struct {
		name      string
		callbacks []ReadCallback
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
			callbacks: []ReadCallback{
				&testCallback{name: "cb1"},
				&testCallback{name: "cb2"},
			},
			chunk:   []byte("test"),
			wantErr: false,
		},
		{
			name: "dispatch stops on first error",
			callbacks: []ReadCallback{
				&testCallback{name: "cb1"},
				&testCallback{name: "cb2", err: errors.New("cb2 error")},
				&testCallback{name: "cb3"},
			},
			chunk:   []byte("test"),
			wantErr: true,
			errMsg:  "cb2 error",
		},
		{
			name: "dispatch handles panic with error",
			callbacks: []ReadCallback{
				&testCallback{name: "cb1", panicMsg: "panic error"},
			},
			chunk:   []byte("test"),
			wantErr: true,
			errMsg:  "callback panic: panic error",
		},
		{
			name: "dispatch handles panic with non-error",
			callbacks: []ReadCallback{
				&testCallback{name: "cb1", panicMsg: "string panic"},
			},
			chunk:   []byte("test"),
			wantErr: true,
			errMsg:  "callback panic: string panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := NewReader(strings.NewReader(""), tt.callbacks)
			err := br.dispatch(tt.chunk)

			if (err != nil) != tt.wantErr {
				t.Errorf("BufferedReader.dispatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("BufferedReader.dispatch() error = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestFormatPanic(t *testing.T) {
	tests := []struct {
		name  string
		panic interface{}
		want  string
	}{
		{
			name:  "error type",
			panic: errors.New("test error"),
			want:  "test error",
		},
		{
			name:  "string type",
			panic: "string panic",
			want:  "string panic",
		},
		{
			name:  "other type",
			panic: 123,
			want:  "unknown panic",
		},
		{
			name:  "nil",
			panic: nil,
			want:  "unknown panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPanic(tt.panic)
			if got != tt.want {
				t.Errorf("formatPanic() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTeeReader(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		callbacks []ReadCallback
		wantRead  string
		wantWrite string
		wantErr   bool
	}{
		{
			name:      "basic tee functionality",
			input:     "hello world",
			callbacks: nil,
			wantRead:  "hello world",
			wantWrite: "hello world",
			wantErr:   false,
		},
		{
			name:      "tee with additional callbacks",
			input:     "test data",
			callbacks: []ReadCallback{&testCallback{name: "extra"}},
			wantRead:  "test data",
			wantWrite: "test data",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := strings.NewReader(tt.input)
			tr := TeeReader(r, &buf, tt.callbacks...)

			data, err := io.ReadAll(tr)
			if (err != nil) != tt.wantErr {
				t.Errorf("TeeReader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if string(data) != tt.wantRead {
				t.Errorf("TeeReader() read = %v, want %v", string(data), tt.wantRead)
			}

			if buf.String() != tt.wantWrite {
				t.Errorf("TeeReader() write = %v, want %v", buf.String(), tt.wantWrite)
			}
		})
	}
}

func TestTeeWriterCallback(t *testing.T) {
	tests := []struct {
		name        string
		chunk       []byte
		writeErr    error
		wantErr     bool
		wantWritten string
	}{
		{
			name:        "successful write",
			chunk:       []byte("test"),
			writeErr:    nil,
			wantErr:     false,
			wantWritten: "test",
		},
		{
			name:        "write error",
			chunk:       []byte("test"),
			writeErr:    errors.New("write error"),
			wantErr:     true,
			wantWritten: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if tt.writeErr != nil {
				// Use a writer that returns an error
				tw := &teeWriterCallback{w: &errorWriter{err: tt.writeErr}}
				err := tw.OnData(tt.chunk)
				if (err != nil) != tt.wantErr {
					t.Errorf("teeWriterCallback.OnData() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else {
				tw := &teeWriterCallback{w: &buf}
				err := tw.OnData(tt.chunk)
				if (err != nil) != tt.wantErr {
					t.Errorf("teeWriterCallback.OnData() error = %v, wantErr %v", err, tt.wantErr)
				}
				if buf.String() != tt.wantWritten {
					t.Errorf("teeWriterCallback.OnData() written = %v, want %v", buf.String(), tt.wantWritten)
				}
			}
		})
	}

	// Test Name and Result methods
	tw := &teeWriterCallback{w: &bytes.Buffer{}}
	if tw.Name() != "_tee_writer" {
		t.Errorf("teeWriterCallback.Name() = %v, want _tee_writer", tw.Name())
	}
	if tw.Result() != nil {
		t.Errorf("teeWriterCallback.Result() = %v, want nil", tw.Result())
	}

	// Test error persistence
	ew := &errorWriter{err: errors.New("first error")}
	tw = &teeWriterCallback{w: ew}

	// First call sets the error
	err1 := tw.OnData([]byte("test"))
	if err1 == nil || err1.Error() != "first error" {
		t.Errorf("teeWriterCallback first error = %v, want 'first error'", err1)
	}

	// Second call should return the same error without writing
	ew.err = errors.New("second error")
	err2 := tw.OnData([]byte("test2"))
	if err2 == nil || err2.Error() != "first error" {
		t.Errorf("teeWriterCallback sticky error = %v, want 'first error'", err2)
	}
}

type errorWriter struct {
	err error
}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, e.err
}
