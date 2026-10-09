package synapse

import (
	"os"
	"testing"
)

// removeAndMakeDir replaces path with a directory, so that appending to it as a file fails.
func removeAndMakeDir(t *testing.T, path string) {
	t.Helper()
	os.Remove(path)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}
