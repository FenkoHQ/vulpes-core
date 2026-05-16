package plugins

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFilesystem(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin")
	if err := os.WriteFile(path, []byte("hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveFilesystem(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.SHA256 == "" {
		t.Fatal("missing sha")
	}
	if _, err := ResolveFilesystem(path, "sha256:deadbeef"); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestResolveFilesystemRejectsDirectory(t *testing.T) {
	if _, err := ResolveFilesystem(t.TempDir(), ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestGitHubHelpers(t *testing.T) {
	url := GitHubReleaseURL("fenko/plugins", "v1", "x.tar.gz")
	if !strings.Contains(url, "/fenko/plugins/releases/download/v1/x.tar.gz") {
		t.Fatalf("url %s", url)
	}
	dir := GitHubCacheDir("/home/a", "fenko/plugins", "v1", "x", "sha")
	if !strings.Contains(dir, filepath.Join("fenko", "plugins", "v1", "x", "sha")) {
		t.Fatalf("dir %s", dir)
	}
}

func TestSafeExtractRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bad.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(tw, "x")
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
	if err := SafeExtract(archive, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
