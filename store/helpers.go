package store

import (
	"crypto/sha512"
	"io"
	"os"

	"github.com/Avalanche-io/c4"
)

// defaultHas checks existence by attempting to Open. Stores with a more
// efficient check (HEAD request, stat, etc.) should override this.
func defaultHas(s Source, id c4.ID) bool {
	rc, err := s.Open(id)
	if err != nil {
		return false
	}
	rc.Close()
	return true
}

// defaultPut reads content, computes its C4 ID, and stores it via Create.
// Stores with a more efficient mechanism (temp file + rename, multipart
// upload, etc.) should override this.
func defaultPut(s Store, r io.Reader) (c4.ID, error) {
	// Write to temp file while computing hash.
	tmp, err := os.CreateTemp("", "c4put.*")
	if err != nil {
		return c4.ID{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	h := sha512.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		return c4.ID{}, err
	}

	var digest c4.ID
	copy(digest[:], h.Sum(nil))

	// Check if already stored.
	if defaultHas(s, digest) {
		return digest, nil
	}

	// Rewind and write to store.
	if _, err := tmp.Seek(0, 0); err != nil {
		return c4.ID{}, err
	}

	w, err := s.Create(digest)
	if err != nil {
		// Already exists (race) is fine.
		if os.IsExist(err) {
			return digest, nil
		}
		return c4.ID{}, err
	}
	if _, err := io.Copy(w, tmp); err != nil {
		return c4.ID{}, err
	}
	if err := w.Close(); err != nil {
		return c4.ID{}, err
	}

	return digest, nil
}
