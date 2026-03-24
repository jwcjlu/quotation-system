package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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
		"2006-01-02", // MySQL DATE、bom_date
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

// AgentScriptPackage 一行元数据（与 agent_script_package 表对齐；分发觉以 script_id 为唯一条目键）。
type AgentScriptPackage struct {
	ID             int64
	ScriptID       string
	Version        string
	SHA256         string
	StorageRelPath string
	Filename       string
	Status         string
	ReleaseNotes   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

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
	notes := strings.TrimSpace(p.ReleaseNotes)
	q := `INSERT INTO agent_script_package
(script_id, version, sha256, storage_rel_path, filename, status, release_notes)
VALUES (?,?,?,?,?,?,?)`
	var notesArg any
	if notes == "" {
		notesArg = nil
	} else {
		notesArg = notes
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return 0, err
	}
	res, err := sqlDB.ExecContext(ctx, q,
		p.ScriptID, p.Version, p.SHA256, p.StorageRelPath, p.Filename, p.Status, notesArg,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
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

	row, err := r.getByIDTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("package id %d not found", id)
	}
	if strings.EqualFold(strings.TrimSpace(row.Status), "published") {
		return tx.Commit().Error
	}

	if err := tx.Exec(`
UPDATE agent_script_package SET status = 'archived'
WHERE script_id = ? AND status = 'published' AND id <> ?`,
		row.ScriptID, id).Error; err != nil {
		return err
	}
	if err := tx.Exec(`UPDATE agent_script_package SET status = 'published' WHERE id = ?`, id).Error; err != nil {
		return err
	}
	return tx.Commit().Error
}

func (r *AgentScriptPackageRepo) getByIDTx(ctx context.Context, tx *gorm.DB, id int64) (*AgentScriptPackage, error) {
	return scanOnePkg(tx.WithContext(ctx).Raw(`
SELECT id, script_id, version, sha256, storage_rel_path, filename, status,
       IFNULL(release_notes,''), created_at, updated_at
FROM agent_script_package WHERE id = ?`, id).Row())
}

// GetByID ...
func (r *AgentScriptPackageRepo) GetByID(ctx context.Context, id int64) (*AgentScriptPackage, error) {
	if r.db == nil {
		return nil, ErrScriptStoreUnavailable
	}
	return scanOnePkg(r.db.WithContext(ctx).Raw(`
SELECT id, script_id, version, sha256, storage_rel_path, filename, status,
       IFNULL(release_notes,''), created_at, updated_at
FROM agent_script_package WHERE id = ?`, id).Row())
}

// GetPublished 当前发布的包（按 script_id）。
func (r *AgentScriptPackageRepo) GetPublished(ctx context.Context, scriptID string) (*AgentScriptPackage, error) {
	if r.db == nil {
		return nil, ErrScriptStoreUnavailable
	}
	scriptID = strings.TrimSpace(scriptID)
	return scanOnePkg(r.db.WithContext(ctx).Raw(`
SELECT id, script_id, version, sha256, storage_rel_path, filename, status,
       IFNULL(release_notes,''), created_at, updated_at
FROM agent_script_package
WHERE script_id = ? AND status = 'published'`, scriptID).Row())
}

// ListAllPublished 全部 published 包（每个 script_id 至多一行）。
func (r *AgentScriptPackageRepo) ListAllPublished(ctx context.Context) ([]*AgentScriptPackage, error) {
	if r.db == nil {
		return nil, ErrScriptStoreUnavailable
	}
	rows, err := r.db.WithContext(ctx).Raw(`
SELECT id, script_id, version, sha256, storage_rel_path, filename, status,
       IFNULL(release_notes,''), created_at, updated_at
FROM agent_script_package
WHERE status = 'published'
ORDER BY script_id`).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*AgentScriptPackage
	for rows.Next() {
		p, err := scanPkgRows(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
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
	rows, err := r.db.WithContext(ctx).Raw(`
SELECT id, script_id, version, sha256, storage_rel_path, filename, status,
       IFNULL(release_notes,''), created_at, updated_at
FROM agent_script_package
ORDER BY id DESC
LIMIT ? OFFSET ?`, limit, offset).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*AgentScriptPackage
	for rows.Next() {
		p, err := scanPkgRows(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func scanOnePkg(row *sql.Row) (*AgentScriptPackage, error) {
	var p AgentScriptPackage
	var cStr, uStr sql.NullString
	err := row.Scan(
		&p.ID, &p.ScriptID, &p.Version, &p.SHA256,
		&p.StorageRelPath, &p.Filename, &p.Status, &p.ReleaseNotes, &cStr, &uStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if cStr.Valid {
		t, err := parseDateTimeForScan(cStr.String)
		if err != nil {
			return nil, fmt.Errorf("created_at: %w", err)
		}
		p.CreatedAt = t
	}
	if uStr.Valid {
		t, err := parseDateTimeForScan(uStr.String)
		if err != nil {
			return nil, fmt.Errorf("updated_at: %w", err)
		}
		p.UpdatedAt = t
	}
	return &p, nil
}

func scanPkgRows(rows *sql.Rows) (*AgentScriptPackage, error) {
	var p AgentScriptPackage
	var cStr, uStr sql.NullString
	err := rows.Scan(
		&p.ID, &p.ScriptID, &p.Version, &p.SHA256,
		&p.StorageRelPath, &p.Filename, &p.Status, &p.ReleaseNotes, &cStr, &uStr,
	)
	if err != nil {
		return nil, err
	}
	if cStr.Valid {
		t, err := parseDateTimeForScan(cStr.String)
		if err != nil {
			return nil, fmt.Errorf("created_at: %w", err)
		}
		p.CreatedAt = t
	}
	if uStr.Valid {
		t, err := parseDateTimeForScan(uStr.String)
		if err != nil {
			return nil, fmt.Errorf("updated_at: %w", err)
		}
		p.UpdatedAt = t
	}
	return &p, nil
}
