package data

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHsDatasheetDownloader_Download_ReuseBySHA256(t *testing.T) {
	t.Parallel()

	content := []byte("%PDF-1.4\nsame-content\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok-a.pdf", "/ok-b.pdf":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	assetDir := t.TempDir()
	downloader := NewHsDatasheetDownloader(assetDir, srv.Client())
	ctx := context.Background()

	first, err := downloader.Download(ctx, "M1", "NXP", srv.URL+"/ok-a.pdf")
	if err != nil {
		t.Fatalf("first download: %v", err)
	}
	if first.DownloadStatus != "ok" {
		t.Fatalf("first status = %q, want ok", first.DownloadStatus)
	}
	if first.SHA256 == "" {
		t.Fatal("first sha256 should not be empty")
	}
	if first.LocalPath == "" {
		t.Fatal("first local path should not be empty")
	}

	second, err := downloader.Download(ctx, "M1", "NXP", srv.URL+"/ok-b.pdf")
	if err != nil {
		t.Fatalf("second download: %v", err)
	}
	if second.DownloadStatus != "ok" {
		t.Fatalf("second status = %q, want ok", second.DownloadStatus)
	}
	if second.SHA256 != first.SHA256 {
		t.Fatalf("sha mismatch: first=%s second=%s", first.SHA256, second.SHA256)
	}
	if second.LocalPath != first.LocalPath {
		t.Fatalf("expected reused local path, first=%q second=%q", first.LocalPath, second.LocalPath)
	}

	data, err := os.ReadFile(first.LocalPath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("downloaded content mismatch: got %q", string(data))
	}
}

func TestHsDatasheetDownloader_CanDownload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.pdf":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	downloader := NewHsDatasheetDownloader(filepath.Join(t.TempDir(), "assets"), srv.Client())
	ctx := context.Background()

	if !downloader.CanDownload(ctx, srv.URL+"/ok.pdf") {
		t.Fatal("expected /ok.pdf downloadable")
	}
	if downloader.CanDownload(ctx, srv.URL+"/404.pdf") {
		t.Fatal("expected /404.pdf non-downloadable")
	}
	if downloader.CanDownload(ctx, "user-upload://abc") {
		t.Fatal("expected user-upload scheme blocked")
	}
	if downloader.CanDownload(ctx, "manual-description-only://") {
		t.Fatal("expected manual-description-only scheme blocked")
	}
}

func TestHsDatasheetDownloader_Download_BlocksUserUploadURL(t *testing.T) {
	t.Parallel()
	d := NewHsDatasheetDownloader(t.TempDir(), http.DefaultClient)
	_, err := d.Download(context.Background(), "M", "ST", "user-upload://x")
	if err == nil {
		t.Fatal("expected error")
	}
}
