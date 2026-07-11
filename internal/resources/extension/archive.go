package extension

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxExtensionArchiveBytes = 50 * 1024 * 1024

var errExtensionArchiveTooLarge = errors.New("extension archive exceeds 50 MiB limit")

type archiveSnapshot struct {
	data     []byte
	checksum string
}

func loadArchiveSnapshot(path string) (archiveSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return archiveSnapshot{}, fmt.Errorf("open extension archive: %w", err)
	}
	defer file.Close()

	return readArchiveSnapshot(file, maxExtensionArchiveBytes)
}

func readArchiveSnapshot(reader io.Reader, maxBytes int64) (archiveSnapshot, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return archiveSnapshot{}, fmt.Errorf("read extension archive: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return archiveSnapshot{}, errExtensionArchiveTooLarge
	}

	sum := sha256.Sum256(data)
	return archiveSnapshot{
		data:     data,
		checksum: hex.EncodeToString(sum[:]),
	}, nil
}
