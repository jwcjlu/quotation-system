package data

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// BomPlatformScriptRepo 实现 biz.BomPlatformScriptRepo。
type BomPlatformScriptRepo struct {
	d *Data
}

// NewBomPlatformScriptRepo ...
func NewBomPlatformScriptRepo(d *Data) *BomPlatformScriptRepo {
	return &BomPlatformScriptRepo{d: d}
}

func (r *BomPlatformScriptRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func toBizRow(m *BomPlatformScript) *biz.BomPlatformScript {
	if m == nil {
		return nil
	}
	dn := ""
	if m.DisplayName != nil {
		dn = *m.DisplayName
	}
	return &biz.BomPlatformScript{
		PlatformID:    m.PlatformID,
		ScriptID:      m.ScriptID,
		DisplayName:   dn,
		Enabled:       m.Enabled,
		RunParamsJSON: append([]byte(nil), m.RunParamsJSON...),
		UpdatedAt:     m.UpdatedAt,
	}
}

func (r *BomPlatformScriptRepo) List(ctx context.Context) ([]biz.BomPlatformScript, error) {
	if !r.DBOk() {
		return nil, nil
	}
	var rows []BomPlatformScript
	if err := r.d.DB.WithContext(ctx).Model(&BomPlatformScript{}).Order("platform_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.BomPlatformScript, 0, len(rows))
	for i := range rows {
		out = append(out, *toBizRow(&rows[i]))
	}
	return out, nil
}

func (r *BomPlatformScriptRepo) Get(ctx context.Context, platformID string) (*biz.BomPlatformScript, error) {
	if !r.DBOk() {
		return nil, nil
	}
	pid := strings.TrimSpace(platformID)
	if pid == "" {
		return nil, nil
	}
	var m BomPlatformScript
	err := r.d.DB.WithContext(ctx).Where("platform_id = ?", pid).Limit(1).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toBizRow(&m), nil
}

func (r *BomPlatformScriptRepo) Upsert(ctx context.Context, p *biz.BomPlatformScript) error {
	if !r.DBOk() || p == nil {
		return gorm.ErrInvalidDB
	}
	pid := strings.TrimSpace(p.PlatformID)
	sid := strings.TrimSpace(p.ScriptID)
	if pid == "" || sid == "" {
		return fmt.Errorf("bom_platform_script: empty platform_id or script_id")
	}
	var dn *string
	if strings.TrimSpace(p.DisplayName) != "" {
		s := strings.TrimSpace(p.DisplayName)
		dn = &s
	}
	row := BomPlatformScript{
		PlatformID:    pid,
		ScriptID:      sid,
		DisplayName:   dn,
		Enabled:       p.Enabled,
		RunParamsJSON: append([]byte(nil), p.RunParamsJSON...),
	}
	return r.d.DB.WithContext(ctx).Save(&row).Error
}

func (r *BomPlatformScriptRepo) Delete(ctx context.Context, platformID string) error {
	if !r.DBOk() {
		return gorm.ErrInvalidDB
	}
	pid := strings.TrimSpace(platformID)
	if pid == "" {
		return fmt.Errorf("bom_platform_script: empty platform_id")
	}
	return r.d.DB.WithContext(ctx).Where("platform_id = ?", pid).Delete(&BomPlatformScript{}).Error
}
