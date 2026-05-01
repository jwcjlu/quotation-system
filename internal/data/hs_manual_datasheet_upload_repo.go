package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// HsManualDatasheetUploadRepo 实现 biz.HsManualDatasheetUploadRepo。
type HsManualDatasheetUploadRepo struct {
	d *Data
}

func NewHsManualDatasheetUploadRepo(d *Data) *HsManualDatasheetUploadRepo {
	return &HsManualDatasheetUploadRepo{d: d}
}

func (r *HsManualDatasheetUploadRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func (r *HsManualDatasheetUploadRepo) Create(ctx context.Context, row *biz.HsManualDatasheetUploadRecord) error {
	if !r.DBOk() || row == nil {
		return gorm.ErrInvalidDB
	}
	id := strings.TrimSpace(row.UploadID)
	if id == "" || strings.TrimSpace(row.LocalPath) == "" || strings.TrimSpace(row.SHA256) == "" {
		return fmt.Errorf("hs_manual_datasheet_upload: upload_id/local_path/sha256 required")
	}
	var owner *string
	if strings.TrimSpace(row.OwnerSubject) != "" {
		s := strings.TrimSpace(row.OwnerSubject)
		owner = &s
	}
	rec := HsManualDatasheetUpload{
		UploadID:     id,
		LocalPath:    strings.TrimSpace(row.LocalPath),
		SHA256:       strings.TrimSpace(row.SHA256),
		ExpiresAt:    row.ExpiresAt,
		OwnerSubject: owner,
	}
	return r.d.DB.WithContext(ctx).Create(&rec).Error
}

func (r *HsManualDatasheetUploadRepo) GetByUploadID(ctx context.Context, uploadID string) (*biz.HsManualDatasheetUploadRecord, error) {
	if !r.DBOk() {
		return nil, nil
	}
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return nil, nil
	}
	var row HsManualDatasheetUpload
	err := r.d.DB.WithContext(ctx).Where("upload_id = ?", uploadID).Limit(1).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &biz.HsManualDatasheetUploadRecord{
		UploadID:   row.UploadID,
		LocalPath:  row.LocalPath,
		SHA256:     row.SHA256,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
	}
	if row.OwnerSubject != nil {
		out.OwnerSubject = strings.TrimSpace(*row.OwnerSubject)
	}
	return out, nil
}

func (r *HsManualDatasheetUploadRepo) MarkConsumed(ctx context.Context, uploadID string) error {
	if !r.DBOk() {
		return gorm.ErrInvalidDB
	}
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return fmt.Errorf("hs_manual_datasheet_upload: upload_id required")
	}
	now := time.Now().UTC()
	res := r.d.DB.WithContext(ctx).Model(&HsManualDatasheetUpload{}).
		Where("upload_id = ?", uploadID).
		Update("consumed_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *HsManualDatasheetUploadRepo) DeleteExpiredBefore(ctx context.Context, t time.Time) (int64, error) {
	if !r.DBOk() {
		return 0, gorm.ErrInvalidDB
	}
	var rows []HsManualDatasheetUpload
	if err := r.d.DB.WithContext(ctx).Where("expires_at < ?", t).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	for i := range rows {
		p := strings.TrimSpace(rows[i].LocalPath)
		if p == "" || !isManualStagingDatasheetPath(p) {
			continue
		}
		_ = os.Remove(p)
	}
	ids := make([]uint64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	res := r.d.DB.WithContext(ctx).Where("id IN ?", ids).Delete(&HsManualDatasheetUpload{})
	return res.RowsAffected, res.Error
}

// isManualStagingDatasheetPath 仅允许删除 manual_staging 目录下的暂存 PDF，防误删正式资产。
func isManualStagingDatasheetPath(p string) bool {
	if p == "" {
		return false
	}
	// 非 Windows 上 filepath.Clean 不会把 `\` 当分隔符，来自 Windows 的路径会先被当成单个「文件名」。
	// 先统一成 `/` 再用 path.Clean（仅处理正斜杠路径），才能在 Linux 上正确分段校验。
	unified := filepath.ToSlash(strings.ReplaceAll(p, `\`, `/`))
	s := strings.ToLower(path.Clean(unified))
	if !strings.HasSuffix(s, ".pdf") {
		return false
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "manual_staging" {
			return true
		}
	}
	return false
}

var _ biz.HsManualDatasheetUploadRepo = (*HsManualDatasheetUploadRepo)(nil)
