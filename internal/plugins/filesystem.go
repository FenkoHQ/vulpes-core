package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

type ResolvedPlugin struct {
	Path   string
	SHA256 string
}

func ResolveFilesystem(path, checksum string) (ResolvedPlugin, error) {
	st, err := os.Stat(path)
	if err != nil {
		return ResolvedPlugin{}, fmt.Errorf("plugin binary %s not found: %w", path, err)
	}
	if st.IsDir() {
		return ResolvedPlugin{}, fmt.Errorf("plugin path %s is a directory; expected executable file", path)
	}
	if st.Mode()&0111 == 0 {
		return ResolvedPlugin{}, fmt.Errorf("plugin binary %s is not executable", path)
	}
	h, err := fileSHA256(path)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	if checksum != "" {
		want := strings.TrimPrefix(checksum, "sha256:")
		if !strings.EqualFold(want, h) {
			return ResolvedPlugin{}, fmt.Errorf("plugin checksum mismatch for %s: got sha256:%s want %s", path, h, checksum)
		}
	}
	return ResolvedPlugin{Path: path, SHA256: h}, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
