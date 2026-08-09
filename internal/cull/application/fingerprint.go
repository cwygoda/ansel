package application

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// fingerprintWindow is how much is read from each end of the file. RAW files
// run to tens of megabytes and a shoot holds thousands of them, so hashing
// every byte would cost more than the analysis it is meant to save.
const fingerprintWindow = 64 * 1024

// Fingerprint identifies file content cheaply, combining size with the head
// and tail bytes. It exists to answer one question: has this photograph
// changed since it was last analyzed?
//
// A full cryptographic hash is deliberately not required here — two distinct
// photographs sharing size, head and tail is not a case that arises from a
// camera.
func Fingerprint(path string) (fingerprint string, size int64, mtimeNs int64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to stat %s: %w", path, err)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	if err := binary.Write(hash, binary.LittleEndian, info.Size()); err != nil {
		return "", 0, 0, err
	}
	if err := hashWindows(hash, file, info.Size()); err != nil {
		return "", 0, 0, fmt.Errorf("failed to read %s: %w", path, err)
	}

	return hex.EncodeToString(hash.Sum(nil)[:16]), info.Size(), info.ModTime().UnixNano(), nil
}

func hashWindows(hash io.Writer, file *os.File, size int64) error {
	if _, err := io.CopyN(hash, file, fingerprintWindow); err != nil && err != io.EOF {
		return err
	}
	if size <= fingerprintWindow {
		return nil
	}

	offset := size - fingerprintWindow
	if offset < fingerprintWindow {
		offset = fingerprintWindow
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	return nil
}
