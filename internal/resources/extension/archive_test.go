package extension

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestLoadArchiveSnapshotReadsAndHashesSameBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extension.zip")
	data := []byte("extension archive")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	snapshot, err := loadArchiveSnapshot(path)
	if err != nil {
		t.Fatalf("loadArchiveSnapshot returned error: %v", err)
	}
	if !bytes.Equal(snapshot.data, data) {
		t.Fatalf("snapshot data = %q, want %q", snapshot.data, data)
	}
	if got, want := snapshot.checksum, "9602532cc2b8bc6ca42336f7d93a1f49b5272bf308cd4a6a132e0074efd76fbc"; got != want {
		t.Fatalf("snapshot checksum = %q, want %q", got, want)
	}
}

func TestLoadArchiveSnapshotReturnsOpenError(t *testing.T) {
	_, err := loadArchiveSnapshot(filepath.Join(t.TempDir(), "missing.zip"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want wrapped os.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "open extension archive") {
		t.Fatalf("error = %q, want open context", err)
	}
}

func TestReadArchiveSnapshotEnforcesSizeLimit(t *testing.T) {
	tests := map[string]struct {
		data    string
		wantErr bool
	}{
		"below limit": {data: "ab"},
		"at limit":    {data: "abc"},
		"over limit":  {data: "abcd", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot, err := readArchiveSnapshot(strings.NewReader(test.data), 3)
			if test.wantErr {
				if !errors.Is(err, errExtensionArchiveTooLarge) {
					t.Fatalf("error = %v, want errExtensionArchiveTooLarge", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("readArchiveSnapshot returned error: %v", err)
			}
			if got, want := string(snapshot.data), test.data; got != want {
				t.Fatalf("snapshot data = %q, want %q", got, want)
			}
		})
	}
}

func TestReadArchiveSnapshotReturnsReadError(t *testing.T) {
	want := errors.New("read failed")
	_, err := readArchiveSnapshot(iotest.ErrReader(want), 3)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped read failure", err)
	}
}
