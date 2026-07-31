package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOASVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(path, []byte(`{"info":{"version":"1.2.3"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	version, err := LoadOASVersion(path)
	if err != nil {
		t.Fatalf("LoadOASVersion() error = %v", err)
	}
	if version != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", version)
	}
}

func TestLoadOASVersionErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := LoadOASVersion(filepath.Join(t.TempDir(), "missing.json"))
		if err == nil || !strings.Contains(err.Error(), "could not open OAS file") {
			t.Fatalf("error = %v, want open error", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "openapi.json")
		if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadOASVersion(path)
		if err == nil || !strings.Contains(err.Error(), "could not parse OAS") {
			t.Fatalf("error = %v, want parse error", err)
		}
	})

	t.Run("missing version", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "openapi.json")
		if err := os.WriteFile(path, []byte(`{"info":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadOASVersion(path)
		if err == nil || !strings.Contains(err.Error(), "version missing from OAS") {
			t.Fatalf("error = %v, want missing version error", err)
		}
	})
}
