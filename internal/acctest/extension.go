package acctest

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// ExtensionArchive writes a minimal Chrome extension ZIP and returns its path and SHA-256 checksum.
func ExtensionArchive(t testing.TB, marker string) (string, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "extension.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create extension archive: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	writeExtensionArchiveFile(t, writer, "manifest.json", `{"manifest_version":3,"name":"Kernel Terraform acceptance","version":"1.0.0"}`)
	writeExtensionArchiveFile(t, writer, "marker.txt", marker)
	if err := writer.Close(); err != nil {
		t.Fatalf("close extension ZIP: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close extension archive: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extension archive: %v", err)
	}
	checksum := sha256.Sum256(contents)
	return path, hex.EncodeToString(checksum[:])
}

func writeExtensionArchiveFile(t testing.TB, writer *zip.Writer, name, contents string) {
	t.Helper()

	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create %s in extension ZIP: %v", name, err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatalf("write %s in extension ZIP: %v", name, err)
	}
}
