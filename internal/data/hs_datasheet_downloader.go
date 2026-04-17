package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"caichip/internal/biz"
)

// HsDatasheetDownloader 负责下载 datasheet，并按 sha256 复用本地资产文件。
type HsDatasheetDownloader struct {
	assetDir string
	client   *http.Client
}

func NewHsDatasheetDownloader(assetDir string, client *http.Client) *HsDatasheetDownloader {
	c := client
	if c == nil {
		c = http.DefaultClient
	}
	return &HsDatasheetDownloader{
		assetDir: strings.TrimSpace(assetDir),
		client:   c,
	}
}

func (d *HsDatasheetDownloader) CanDownload(ctx context.Context, rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

func (d *HsDatasheetDownloader) Download(ctx context.Context, model, manufacturer, datasheetURL string) (*biz.HsDatasheetAssetRecord, error) {
	record := &biz.HsDatasheetAssetRecord{
		Model:          strings.TrimSpace(model),
		Manufacturer:   strings.TrimSpace(manufacturer),
		DatasheetURL:   strings.TrimSpace(datasheetURL),
		DownloadStatus: "failed",
	}
	if record.DatasheetURL == "" {
		record.ErrorMsg = "datasheet_url is empty"
		return record, errors.New(record.ErrorMsg)
	}
	if strings.TrimSpace(d.assetDir) == "" {
		record.ErrorMsg = "asset dir is empty"
		return record, errors.New(record.ErrorMsg)
	}
	if err := os.MkdirAll(d.assetDir, 0o755); err != nil {
		record.ErrorMsg = err.Error()
		return record, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, record.DatasheetURL, nil)
	if err != nil {
		record.ErrorMsg = err.Error()
		return record, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		record.ErrorMsg = err.Error()
		return record, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		record.ErrorMsg = fmt.Sprintf("download failed: status %d", resp.StatusCode)
		return record, errors.New(record.ErrorMsg)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		record.ErrorMsg = err.Error()
		return record, err
	}
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	ext := fileExtFromURL(record.DatasheetURL)
	dstPath := filepath.Join(d.assetDir, sha+ext)

	if _, statErr := os.Stat(dstPath); statErr != nil {
		if !os.IsNotExist(statErr) {
			record.ErrorMsg = statErr.Error()
			return record, statErr
		}
		if err := os.WriteFile(dstPath, body, 0o644); err != nil {
			record.ErrorMsg = err.Error()
			return record, err
		}
	}

	record.SHA256 = sha
	record.LocalPath = dstPath
	record.DownloadStatus = "ok"
	record.ErrorMsg = ""
	return record, nil
}

func fileExtFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ".bin"
	}
	ext := strings.TrimSpace(path.Ext(u.Path))
	if ext == "" || len(ext) > 8 {
		return ".bin"
	}
	return ext
}
