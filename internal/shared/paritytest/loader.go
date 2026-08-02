// Package paritytest loads behavioral parity fixtures.
//
// Fixtures in testdata/parity are captured by executing the legacy TypeScript
// backend, not by reading it. They are the specification the Go port must
// reproduce. See docs/PARITY_CONTRACT.md.
package paritytest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// dirName is the fixture directory relative to the repository root.
const dirName = "testdata/parity"

// Load decodes a fixture file into dst.
//
// The path is resolved by walking up from the test's working directory until
// testdata/parity is found, so callers do not hard-code a relative depth that
// breaks when a package moves.
func Load(t *testing.T, filename string, dst interface{}) {
	t.Helper()

	path, err := resolve(filename)
	if err != nil {
		t.Fatalf("parity fixture %s not found: %v", filename, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read parity fixture %s: %v", path, err)
	}

	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("failed to decode parity fixture %s: %v", path, err)
	}
}

// RequireCases fails the test when a fixture decoded to zero cases. Without this
// guard a fixture that failed to decode would silently pass every assertion.
func RequireCases(t *testing.T, filename string, count int) {
	t.Helper()
	if count == 0 {
		t.Fatalf("parity fixture %s decoded to 0 cases; the fixture or its struct tags are wrong", filename)
	}
}

func resolve(filename string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, dirName, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
