package store

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// EnsureTiku creates tiku.db from the GitHub-size-split parts when the
// combined file is absent. The expected checksum is the checksum shipped
// with the current repository data.
func EnsureTiku(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	parts, err := filepath.Glob(path + ".part-*")
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return fmt.Errorf("tiku.db not found and no tiku.db.part-* files exist")
	}
	sort.Strings(parts)
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	hash := md5.New()
	for _, part := range parts {
		in, err := os.Open(part)
		if err != nil {
			out.Close()
			os.Remove(tmp)
			return err
		}
		_, copyErr := io.Copy(io.MultiWriter(out, hash), in)
		in.Close()
		if copyErr != nil {
			out.Close()
			os.Remove(tmp)
			return copyErr
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	const expected = "efaa2274fc6aa9c385bc30842556b215"
	got := hex.EncodeToString(hash.Sum(nil))
	if got != expected {
		os.Remove(tmp)
		return fmt.Errorf("tiku.db checksum mismatch: got %s, want %s", got, expected)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
