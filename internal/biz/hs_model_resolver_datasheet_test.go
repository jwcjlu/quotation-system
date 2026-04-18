package biz

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubDatasheetChecker struct {
	allow map[string]bool
}

func (s stubDatasheetChecker) CanDownload(_ context.Context, url string) bool {
	return s.allow[url]
}

func TestHsModelResolver_SelectBestDatasheetSource(t *testing.T) {
	t.Parallel()

	now := time.Now()
	candidates := []HsDatasheetCandidate{
		{
			ID:           1,
			DatasheetURL: "",
			UpdatedAt:    now.Add(2 * time.Hour),
		},
		{
			ID:           2,
			DatasheetURL: "https://a.example/a.pdf",
			UpdatedAt:    now.Add(-1 * time.Hour),
		},
		{
			ID:           3,
			DatasheetURL: "https://a.example/b.pdf",
			UpdatedAt:    now.Add(-1 * time.Hour),
		},
		{
			ID:           4,
			DatasheetURL: "https://a.example/c.pdf",
			UpdatedAt:    now.Add(1 * time.Hour),
		},
	}

	resolver := NewHsModelResolver(stubDatasheetChecker{
		allow: map[string]bool{
			"https://a.example/a.pdf": true,
			"https://a.example/b.pdf": true,
			"https://a.example/c.pdf": false,
		},
	})

	got := resolver.SelectBestDatasheetSource(context.Background(), candidates)
	if got == nil {
		t.Fatal("expected candidate, got nil")
	}
	// c.pdf 最新但不可下载；a/b 时间并列，取 id 较大的 b。
	if got.DatasheetURL != "https://a.example/b.pdf" || got.ID != 3 {
		t.Fatalf("unexpected candidate: %+v", *got)
	}
}

type stubDatasheetDownloader struct {
	canDownloadFn func(ctx context.Context, url string) bool
	downloadFn    func(ctx context.Context, model, manufacturer, datasheetURL string) (*HsDatasheetAssetRecord, error)
}

func (s stubDatasheetDownloader) CanDownload(ctx context.Context, url string) bool {
	if s.canDownloadFn != nil {
		return s.canDownloadFn(ctx, url)
	}
	return true
}

func (s stubDatasheetDownloader) Download(ctx context.Context, model, manufacturer, datasheetURL string) (*HsDatasheetAssetRecord, error) {
	if s.downloadFn == nil {
		return nil, nil
	}
	return s.downloadFn(ctx, model, manufacturer, datasheetURL)
}

type stubDatasheetAssetRepo struct {
	saved  []*HsDatasheetAssetRecord
	err    error
	nextID uint64
}

func (s *stubDatasheetAssetRepo) DBOk() bool { return true }

func (s *stubDatasheetAssetRepo) GetLatestByModelManufacturer(_ context.Context, _, _ string) (*HsDatasheetAssetRecord, error) {
	return nil, nil
}

func (s *stubDatasheetAssetRepo) Save(_ context.Context, row *HsDatasheetAssetRecord) error {
	if s.err != nil {
		return s.err
	}
	if row != nil {
		if row.ID == 0 {
			s.nextID++
			row.ID = s.nextID
		}
		copied := *row
		s.saved = append(s.saved, &copied)
	}
	return s.err
}

func TestHsModelResolver_ResolveAndPersistDatasheet_SaveAsset(t *testing.T) {
	t.Parallel()

	now := time.Now()
	candidates := []HsDatasheetCandidate{
		{ID: 1, DatasheetURL: "https://a.example/old.pdf", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: 2, DatasheetURL: "https://a.example/newer.pdf", UpdatedAt: now.Add(-1 * time.Hour)},
	}
	repo := &stubDatasheetAssetRepo{}
	downloader := stubDatasheetDownloader{
		downloadFn: func(_ context.Context, model, manufacturer, datasheetURL string) (*HsDatasheetAssetRecord, error) {
			return &HsDatasheetAssetRecord{
				Model:          model,
				Manufacturer:   manufacturer,
				DatasheetURL:   datasheetURL,
				LocalPath:      "/tmp/hash.pdf",
				SHA256:         "abc123",
				DownloadStatus: "ok",
			}, nil
		},
	}
	resolver := NewHsModelResolver(stubDatasheetChecker{
		allow: map[string]bool{
			"https://a.example/old.pdf":   true,
			"https://a.example/newer.pdf": true,
		},
	}).WithAssetPersistence(downloader, repo)

	got, err := resolver.ResolveAndPersistDatasheet(context.Background(), "M-100", "NXP", candidates)
	if err != nil {
		t.Fatalf("resolve and persist: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted asset record, got nil")
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected one saved asset, got %d", len(repo.saved))
	}
	if repo.saved[0].DatasheetURL != "https://a.example/newer.pdf" {
		t.Fatalf("unexpected saved url: %+v", repo.saved[0])
	}
}

func TestHsModelResolver_ResolveAndPersistDatasheet_PropagateSaveError(t *testing.T) {
	t.Parallel()

	saveErr := errors.New("save failed")
	repo := &stubDatasheetAssetRepo{err: saveErr}
	resolver := NewHsModelResolver(stubDatasheetChecker{
		allow: map[string]bool{
			"https://a.example/a.pdf": true,
		},
	}).WithAssetPersistence(stubDatasheetDownloader{
		downloadFn: func(_ context.Context, model, manufacturer, datasheetURL string) (*HsDatasheetAssetRecord, error) {
			return &HsDatasheetAssetRecord{
				Model:          model,
				Manufacturer:   manufacturer,
				DatasheetURL:   datasheetURL,
				DownloadStatus: "ok",
			}, nil
		},
	}, repo)

	_, err := resolver.ResolveAndPersistDatasheet(context.Background(), "M-200", "TI", []HsDatasheetCandidate{
		{ID: 9, DatasheetURL: "https://a.example/a.pdf", UpdatedAt: time.Now()},
	})
	if err == nil || !errors.Is(err, saveErr) {
		t.Fatalf("expected save error, got: %v", err)
	}
}

func TestHsModelResolver_ResolveAndPersistDatasheet_DownloadFailedStillSaveFailedAsset(t *testing.T) {
	t.Parallel()

	repo := &stubDatasheetAssetRepo{}
	resolver := NewHsModelResolver(stubDatasheetChecker{
		allow: map[string]bool{
			"https://a.example/fail.pdf": true,
		},
	}).WithAssetPersistence(stubDatasheetDownloader{
		canDownloadFn: func(_ context.Context, url string) bool {
			// 故意与 checker 不一致，验证选源判定以 downloader 为准。
			return url == "https://a.example/fail.pdf"
		},
		downloadFn: func(_ context.Context, _ string, _ string, datasheetURL string) (*HsDatasheetAssetRecord, error) {
			return &HsDatasheetAssetRecord{
				DatasheetURL: datasheetURL,
			}, errors.New("network timeout")
		},
	}, repo)

	got, err := resolver.ResolveAndPersistDatasheet(context.Background(), "M-300", "INFINEON", []HsDatasheetCandidate{
		{ID: 1, DatasheetURL: "https://a.example/fail.pdf", UpdatedAt: time.Now()},
	})
	if err == nil {
		t.Fatal("expected download error, got nil")
	}
	if got == nil {
		t.Fatal("expected failed record, got nil")
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected failed record persisted once, got %d", len(repo.saved))
	}
	saved := repo.saved[0]
	if saved.DownloadStatus != "failed" {
		t.Fatalf("expected failed status, got %+v", saved)
	}
	if saved.DatasheetURL != "https://a.example/fail.pdf" {
		t.Fatalf("expected datasheet url kept, got %+v", saved)
	}
	if saved.ErrorMsg == "" {
		t.Fatalf("expected error message persisted, got %+v", saved)
	}
}
