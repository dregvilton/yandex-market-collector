package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RotatingFile keeps the current log plus two bounded backups.
type RotatingFile struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	size     int64
	maxBytes int64
}

func Open(path string, maxBytes int64) (*RotatingFile, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("maxBytes must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &RotatingFile{path: path, file: f, size: info.Size(), maxBytes: maxBytes}, nil
}
func (l *RotatingFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size+int64(len(p)) > l.maxBytes {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.file.Write(p)
	l.size += int64(n)
	return n, err
}
func (l *RotatingFile) Close() error { l.mu.Lock(); defer l.mu.Unlock(); return l.file.Close() }
func (l *RotatingFile) rotate() error {
	if err := l.file.Close(); err != nil {
		return err
	}
	_ = os.Remove(l.path + ".2")
	if err := os.Rename(l.path+".1", l.path+".2"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.file = f
	l.size = 0
	return nil
}
