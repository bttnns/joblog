package state

import (
	"go/format"
	"os"
	"path/filepath"
	"testing"
)

func TestGofmtClean(t *testing.T) {
	// Clean up any stale formatter scratch files from earlier runs.
	stale, _ := filepath.Glob("*.go.fmt")
	for _, s := range stale {
		os.Remove(s)
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out, err := format.Source(src)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if string(out) != string(src) {
			t.Errorf("%s is not gofmt-clean", f)
		}
	}
}
