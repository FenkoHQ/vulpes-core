package plugins

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Downloader interface {
	Download(ctx context.Context, url string) (io.ReadCloser, error)
}
type HTTPDownloader struct{ Client *http.Client }

func (d HTTPDownloader) Download(ctx context.Context, url string) (io.ReadCloser, error) {
	c := d.Client
	if c == nil {
		c = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: status %s", url, resp.Status)
	}
	return resp.Body, nil
}

func GitHubReleaseURL(repository, release, asset string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repository, release, asset)
}
func GitHubCacheDir(home, repository, release, asset, sha string) string {
	parts := strings.Split(repository, "/")
	owner, repo := "unknown", repository
	if len(parts) == 2 {
		owner, repo = parts[0], parts[1]
	}
	return filepath.Join(home, ".llm-gateway", "plugins", "github", owner, repo, release, asset, sha)
}

type GitHubSource struct{ Repository, Release, Asset, Binary, Checksum string }

func ResolveGitHub(ctx context.Context, src GitHubSource, cacheHome string, d Downloader) (ResolvedPlugin, error) {
	if d == nil {
		d = HTTPDownloader{}
	}
	want := strings.TrimPrefix(src.Checksum, "sha256:")
	if want != "" {
		cached := filepath.Join(GitHubCacheDir(cacheHome, src.Repository, src.Release, src.Asset, want), src.Binary)
		if st, err := os.Stat(cached); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return ResolvedPlugin{Path: cached, SHA256: want}, nil
		}
	}
	r, err := d.Download(ctx, GitHubReleaseURL(src.Repository, src.Release, src.Asset))
	if err != nil {
		return ResolvedPlugin{}, err
	}
	defer r.Close()
	tmp, err := os.CreateTemp("", "llm-gateway-plugin-*")
	if err != nil {
		return ResolvedPlugin{}, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		tmp.Close()
		return ResolvedPlugin{}, err
	}
	if err := tmp.Close(); err != nil {
		return ResolvedPlugin{}, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if want != "" && !strings.EqualFold(want, got) {
		return ResolvedPlugin{}, fmt.Errorf("github asset checksum mismatch: got sha256:%s want %s", got, src.Checksum)
	}
	sha := got
	if want != "" {
		sha = want
	}
	dir := GitHubCacheDir(cacheHome, src.Repository, src.Release, src.Asset, sha)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ResolvedPlugin{}, err
	}
	if err := SafeExtract(tmp.Name(), dir); err != nil {
		return ResolvedPlugin{}, err
	}
	bin := filepath.Join(dir, src.Binary)
	if err := os.Chmod(bin, 0o755); err != nil {
		return ResolvedPlugin{}, fmt.Errorf("mark binary executable: %w", err)
	}
	return ResolveFilesystem(bin, "")
}

func SafeExtract(archivePath, dest string) error {
	lower := strings.ToLower(archivePath)
	if strings.HasSuffix(lower, ".zip") {
		return safeExtractZip(archivePath, dest)
	}
	return safeExtractTarGz(archivePath, dest)
}

func safeJoin(dest, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	p := filepath.Join(dest, clean)
	rel, err := filepath.Rel(dest, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return p, nil
}

func safeExtractZip(path, dest string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %q", f.Name)
		}
		p, err := safeJoin(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			r.Close()
			return err
		}
		_, cpErr := io.Copy(out, r)
		r.Close()
		closeErr := out.Close()
		if cpErr != nil {
			return cpErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func safeExtractTarGz(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeSymlink || h.Typeflag == tar.TypeLink {
			return fmt.Errorf("refusing link %q", h.Name)
		}
		p, err := safeJoin(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, os.FileMode(h.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode))
			if err != nil {
				return err
			}
			_, cpErr := io.Copy(out, tr)
			closeErr := out.Close()
			if cpErr != nil {
				return cpErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported archive entry %q", h.Name)
		}
	}
}
