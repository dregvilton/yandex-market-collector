package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "collector.log")
	l, e := Open(path, 10)
	if e != nil {
		t.Fatal(e)
	}
	for _, line := range []string{"123456789\n", "abcdefghi\n", "987654321\n"} {
		if _, e = l.Write([]byte(line)); e != nil {
			t.Fatal(e)
		}
	}
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	for _, suffix := range []string{"", ".1", ".2"} {
		b, e := os.ReadFile(path + suffix)
		if e != nil || len(b) != 10 {
			t.Fatalf("%s: %q %v", suffix, b, e)
		}
	}
}
