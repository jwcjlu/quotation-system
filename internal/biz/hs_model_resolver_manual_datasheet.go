package biz

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WithManualDatasheet 注入上传 staging 仓储与 datasheet 落盘目录（与下载器目录一致）。
func (r *HsModelResolver) WithManualDatasheet(repo HsManualDatasheetUploadRepo, assetDir string) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.manualUploadRepo = repo
	r.hsAssetDir = strings.TrimSpace(assetDir)
	return r
}

func (r *HsModelResolver) hasManualDatasheetInput(n HsModelResolveRequest) bool {
	return n.ManualComponentDescription != "" || n.ManualUploadID != ""
}

// tryManualDatasheetBypass 在主路径 datasheet 失败后尝试用户上传或纯文本占位资产。
func (r *HsModelResolver) tryManualDatasheetBypass(ctx context.Context, n HsModelResolveRequest) (*HsDatasheetAssetRecord, error) {
	if r == nil {
		return nil, errors.New("hs model resolver: nil receiver")
	}
	if n.ManualUploadID != "" {
		return r.consumeManualUpload(ctx, n)
	}
	if n.ManualComponentDescription != "" {
		return &HsDatasheetAssetRecord{
			Model:          n.Model,
			Manufacturer:   n.Manufacturer,
			DatasheetURL:   ManualDescriptionOnlyURLPrefix,
			LocalPath:      "",
			DownloadStatus: "ok",
			ErrorMsg:       "",
			UpdatedAt:      time.Now(),
		}, nil
	}
	return nil, nil
}

func (r *HsModelResolver) consumeManualUpload(ctx context.Context, n HsModelResolveRequest) (*HsDatasheetAssetRecord, error) {
	if r.manualUploadRepo == nil || !r.manualUploadRepo.DBOk() {
		return nil, fmt.Errorf("manual datasheet: upload repo not configured")
	}
	if strings.TrimSpace(r.hsAssetDir) == "" {
		return nil, fmt.Errorf("manual datasheet: asset dir not configured")
	}
	rec, err := r.manualUploadRepo.GetByUploadID(ctx, n.ManualUploadID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, fmt.Errorf("manual_upload_id not found")
	}
	if rec.ConsumedAt != nil {
		return nil, fmt.Errorf("manual_upload_id already consumed")
	}
	if time.Now().After(rec.ExpiresAt) {
		return nil, fmt.Errorf("manual_upload_id expired")
	}
	if strings.TrimSpace(rec.OwnerSubject) != "" && strings.TrimSpace(rec.OwnerSubject) != n.ManualUploadOwnerSubject {
		return nil, fmt.Errorf("manual_upload_id owner mismatch")
	}
	body, err := os.ReadFile(rec.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("manual datasheet: read staging: %w", err)
	}
	if len(body) < 5 || string(body[:5]) != "%PDF-" {
		return nil, fmt.Errorf("manual datasheet: not a pdf file")
	}
	if err := os.MkdirAll(r.hsAssetDir, 0o755); err != nil {
		return nil, err
	}
	dst := filepath.Join(r.hsAssetDir, rec.SHA256+".pdf")
	if _, stErr := os.Stat(dst); stErr != nil {
		if !os.IsNotExist(stErr) {
			return nil, stErr
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return nil, err
		}
	}
	asset := &HsDatasheetAssetRecord{
		Model:          n.Model,
		Manufacturer:   n.Manufacturer,
		DatasheetURL:   UserUploadDatasheetURLPrefix + n.ManualUploadID,
		LocalPath:      dst,
		SHA256:         rec.SHA256,
		DownloadStatus: "ok",
		UpdatedAt:      time.Now(),
	}
	if r.assetRepo == nil || !r.assetRepo.DBOk() {
		return nil, fmt.Errorf("manual datasheet: asset repo not configured")
	}
	if err := r.assetRepo.Save(ctx, asset); err != nil {
		return nil, err
	}
	if err := r.manualUploadRepo.MarkConsumed(ctx, n.ManualUploadID); err != nil {
		return nil, err
	}
	return asset, nil
}
