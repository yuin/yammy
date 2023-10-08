package yammy_test

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

type mockFileInfo struct {
	name    string
	data    []byte
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

var defaultModTime = time.Date(2022, 1, 1, 1, 0, 0, 0, time.UTC)

func newMockFileInfo(name string, data []byte) fs.FileInfo {
	return &mockFileInfo{
		name:    name,
		data:    data,
		mode:    fs.FileMode(0777),
		modTime: defaultModTime,
		isDir:   false,
	}
}

func (fi *mockFileInfo) Name() string {
	return fi.name
}

func (fi *mockFileInfo) Size() int64 {
	return int64(len(fi.data))
}

func (fi *mockFileInfo) Mode() fs.FileMode {
	return fi.mode
}

func (fi *mockFileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi *mockFileInfo) IsDir() bool {
	return fi.isDir
}

func (fi *mockFileInfo) Sys() any {
	return nil
}

type mockFile struct {
	name      string
	data      []byte
	readIndex int64
}

func newMockFile(name string, data []byte) fs.File {
	return &mockFile{
		name:      name,
		data:      data,
		readIndex: 0,
	}
}

func (m *mockFile) Stat() (fs.FileInfo, error) {
	return newMockFileInfo(m.name, m.data), nil
}

func (m *mockFile) Read(p []byte) (int, error) {
	if m.readIndex >= int64(len(m.data)) {
		return 0, io.EOF
	}

	n := copy(p, m.data[m.readIndex:])
	m.readIndex += int64(n)
	return n, nil
}

func (m *mockFile) Close() error {
	return nil
}

type readErrorMockFile struct {
	fs.File
}

func (m *readErrorMockFile) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

type mapFS struct {
	m map[string][]byte
}

func newMockFS(m map[string][]byte) fs.FS {
	f := &mapFS{
		m: map[string][]byte{},
	}
	for key, value := range m {
		f.m[key] = value
	}
	return f
}

func (f *mapFS) Open(path string) (fs.File, error) {
	readError := false
	if strings.HasSuffix(path, ";readError") {
		readError = true
		path = path[0 : len(path)-len(";readError")]
	}
	if v, ok := f.m[path]; ok {
		if readError {
			return &readErrorMockFile{newMockFile(filepath.Base(path), v)}, nil
		}
		return newMockFile(filepath.Base(path), v), nil
	}
	return nil, fmt.Errorf("open %s: no such file or directory", path)
}
