package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// parseDateTimeForScan 将 MySQL DATETIME / 驱动返回的字符串解析为本地时区的 time.Time（不依赖 DSN 的 parseTime）。
func parseDateTimeForScan(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	loc := time.Local
	for _, layout := range []string{
		"2006-01-02",
		"2006-01-02 15:04:05.000000",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp: %q", s)
}

// ErrScriptStoreUnavailable 未配置数据库或 DB 为 nil。
var ErrScriptStoreUnavailable = errors.New("script store: database unavailable")

// AgentScriptPackage 一行元数据（GORM 模型，与 t_agent_script_package 表对齐）。
type AgentScriptPackage struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ScriptID       string    `gorm:"column:script_id;size:128;not null"`
	Version        string    `gorm:"column:version;size:64;not null"`
	SHA256         string    `gorm:"column:sha256;size:64;not null"`
	StorageRelPath string    `gorm:"column:storage_rel_path;size:512;not null"`
	Filename       string    `gorm:"column:filename;size:255;not null"`
	Status         string    `gorm:"column:status;size:32;not null;default:uploaded"`
	ReleaseNotes   string    `gorm:"column:release_notes;type:text"`
	CreatedAt      time.Time `gorm:"column:created_at;precision:3"`
	UpdatedAt      time.Time `gorm:"column:updated_at;precision:3"`
}

func (AgentScriptPackage) TableName() string { return TableAgentScriptPackage }

// AgentScriptPackageRepo 脚本包持久化。
type AgentScriptPackageRepo struct {
	db *gorm.DB
}

// NewAgentScriptPackageRepo 无 DB 时仍可构造，方法返回 ErrScriptStoreUnavailable。
func NewAgentScriptPackageRepo(d *Data) *AgentScriptPackageRepo {
	if d == nil {
		return &AgentScriptPackageRepo{}
	}
	return &AgentScriptPackageRepo{db: d.DB}
}

// DBOk 是否已连接数据库。
func (r *AgentScriptPackageRepo) DBOk() bool {
	return r != nil && r.db != nil
}

// Insert 插入 uploaded 行，返回自增 id。
func (r *AgentScriptPackageRepo) Insert(ctx context.Context, p *AgentScriptPackage) (int64, error) {
	if r.db == nil {
		return 0, ErrScriptStoreUnavailable
	}
	p.ScriptID = strings.TrimSpace(p.ScriptID)
	p.Version = strings.TrimSpace(p.Version)
	p.SHA256 = strings.ToLower(strings.TrimSpace(p.SHA256))
	p.StorageRelPath = strings.TrimSpace(p.StorageRelPath)
	p.Filename = strings.TrimSpace(p.Filename)
	p.Status = strings.TrimSpace(p.Status)
	if p.Status == "" {
		p.Status = "uploaded"
	}
	p.ReleaseNotes = strings.TrimSpace(p.ReleaseNotes)
	row := *p
	row.ID = 0
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// SetPublished 将 id 对应行设为 published，同 script_id 的其它 published 行改为 archived。
func (r *AgentScriptPackageRepo) SetPublished(ctx context.Context, id int64) error {
	if r.db == nil {
		return ErrScriptStoreUnavailable
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var row AgentScriptPackage
	if err := tx.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("package id %d not found", id)
		}
		return err
	}
	if strings.EqualFold(strings.TrimSpace(row.Status), "published") {
		return tx.Commit().Error
	}

	if err := tx.Model(&AgentScriptPackage{}).
		Where("script_id = ? AND status = ? AND id <> ?", row.ScriptID, "published", id).
		Update("status", "archived").Error; err != nil {
		return err
	}
	if err := tx.Model(&AgentScriptPackage{}).Where("id = ?", id).Update("status", "published").Error; err != nil {
		return err
	}
	return tx.Commit().Error
}

func (r *AgentScriptPackageRepo) getByIDTx(ctx context.Context, tx *gorm.DB, id int64) (*AgentScriptPackage, error) {
	var p AgentScriptPackage
	err := tx.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetByID ...
func (r *AgentScriptPackageRepo) GetByID(ctx context.Context, id int64) (*AgentScriptPackage, error) {
	if r.db == nil {
		return nil, ErrScriptStoreUnavailable
	}
	return r.getByIDTx(ctx, r.db, id)
}

// GetPublished 当前发布的包（按 script_id）。
func (r *AgentScriptPackageRepo) GetPublished(ctx context.Context, scriptID string) (*AgentScriptPackage, error) {
	if r.db == nil {
		return nil, ErrScriptStoreUnavailable
	}
	scriptID = strings.TrimSpace(scriptID)
	var p AgentScriptPackage
	err := r.db.WithContext(ctx).
		Where("script_id = ? AND status = ?", scriptID, "published").
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListAllPublished 全部 published 包（每个 script_id 至多一行）。
func (r *AgentScriptPackageRepo) ListAllPublished(ctx context.Context) ([]*AgentScriptPackage, error) {
	if r.db == nil {
		return nil, ErrScriptStoreUnavailable
	}
	var list []*AgentScriptPackage
	err := r.db.WithContext(ctx).
		Where("status = ?", "published").
		Order("script_id").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ListPublishedScripts 实现 biz.AgentScriptPublishedLister，供 Agent 同步比对。
func (r *AgentScriptPackageRepo) ListPublishedScripts(ctx context.Context) ([]biz.PublishedScriptMeta, error) {
	ps, err := r.ListAllPublished(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.PublishedScriptMeta, 0, len(ps))
	for _, p := range ps {
		if p == nil {
			continue
		}
		out = append(out, biz.PublishedScriptMeta{
			ScriptID:       p.ScriptID,
			Version:        p.Version,
			SHA256:         p.SHA256,
			StorageRelPath: p.StorageRelPath,
			Status:         p.Status,
		})
	}
	return out, nil
}

// ListPackages 分页列举（按 id 降序）。
func (r *AgentScriptPackageRepo) ListPackages(ctx context.Context, offset, limit int) ([]*AgentScriptPackage, error) {
	if r.db == nil {
		return nil, ErrScriptStoreUnavailable
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var list []*AgentScriptPackage
	err := r.db.WithContext(ctx).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

var _ biz.AgentScriptPublishedLister = (*AgentScriptPackageRepo)(nil)
