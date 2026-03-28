package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentScriptAuthRepo Agent×script_id 登录凭据（密码 AES-GCM 落库）。
type AgentScriptAuthRepo struct {
	db     *gorm.DB
	cipher *scriptAuthCipher
}

// NewAgentScriptAuthRepo 无 DB 时返回零值；密钥无效或缺失时 CipherConfigured()==false，仅禁止 Upsert，Pull 不注入。
func NewAgentScriptAuthRepo(bc *conf.Bootstrap, d *Data) *AgentScriptAuthRepo {
	if d == nil || d.DB == nil {
		return &AgentScriptAuthRepo{}
	}
	key := ResolveAgentScriptAuthKey(bc)
	if len(key) != 32 {
		return &AgentScriptAuthRepo{db: d.DB, cipher: nil}
	}
	ciph, err := newScriptAuthCipher(key)
	if err != nil {
		return &AgentScriptAuthRepo{db: d.DB, cipher: nil}
	}
	return &AgentScriptAuthRepo{db: d.DB, cipher: ciph}
}

// DBOk ...
func (r *AgentScriptAuthRepo) DBOk() bool {
	return r != nil && r.db != nil
}

// CipherConfigured 为 true 时可 Upsert 与解密下发。
func (r *AgentScriptAuthRepo) CipherConfigured() bool {
	return r != nil && r.cipher != nil
}

// GetPlatformAuth 解密密码；无行或解密失败时 ok=false（err 仅表示 DB 错误）。
func (r *AgentScriptAuthRepo) GetPlatformAuth(ctx context.Context, agentID, scriptID string) (username, password string, ok bool, err error) {
	if !r.DBOk() {
		return "", "", false, ErrDispatchTaskNoDB
	}
	if !r.CipherConfigured() {
		return "", "", false, nil
	}
	agentID = strings.TrimSpace(agentID)
	scriptID = strings.TrimSpace(scriptID)
	if agentID == "" || scriptID == "" {
		return "", "", false, nil
	}
	var row CaichipAgentScriptAuth
	q := r.db.WithContext(ctx).Where("agent_id = ? AND script_id = ?", agentID, scriptID).First(&row)
	if errors.Is(q.Error, gorm.ErrRecordNotFound) {
		return "", "", false, nil
	}
	if q.Error != nil {
		return "", "", false, q.Error
	}
	u := strings.TrimSpace(row.Username)
	cipherText := strings.TrimSpace(row.PasswordCipher)
	if u == "" || cipherText == "" {
		return "", "", false, nil
	}
	pw, decErr := r.cipher.decryptString(cipherText)
	if decErr != nil {
		return "", "", false, nil
	}
	return u, pw, true, nil
}

// ListByAgent 列出该行 Agent 的凭据（不含密码）。
func (r *AgentScriptAuthRepo) ListByAgent(ctx context.Context, agentID string) ([]biz.AgentScriptAuthSummary, error) {
	if !r.DBOk() {
		return nil, ErrDispatchTaskNoDB
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("agent script auth: agent_id required")
	}
	var rows []CaichipAgentScriptAuth
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Order("script_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.AgentScriptAuthSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.AgentScriptAuthSummary{
			ScriptID:  strings.TrimSpace(row.ScriptID),
			Username:  strings.TrimSpace(row.Username),
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// Upsert 写入或更新一行；须 CipherConfigured。
func (r *AgentScriptAuthRepo) Upsert(ctx context.Context, agentID, scriptID, username, password string) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	if !r.CipherConfigured() {
		return errors.New("agent script auth: AES key not configured (set CAICHIP_AGENT_SCRIPT_AUTH_KEY or agent_script_auth.aes_key_base64)")
	}
	agentID = strings.TrimSpace(agentID)
	scriptID = strings.TrimSpace(scriptID)
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if agentID == "" || scriptID == "" || username == "" || password == "" {
		return errors.New("agent script auth: agent_id, script_id, username, password required")
	}
	cipherText, err := r.cipher.encryptString(password)
	if err != nil {
		return err
	}
	now := time.Now()
	row := CaichipAgentScriptAuth{
		AgentID:        agentID,
		ScriptID:       scriptID,
		Username:       username,
		PasswordCipher: cipherText,
		UpdatedAt:      now,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "agent_id"}, {Name: "script_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"username":        username,
			"password_cipher": cipherText,
			"updated_at":      gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}),
	}).Create(&row).Error
}

// Delete 删除一行。
func (r *AgentScriptAuthRepo) Delete(ctx context.Context, agentID, scriptID string) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	agentID = strings.TrimSpace(agentID)
	scriptID = strings.TrimSpace(scriptID)
	if agentID == "" || scriptID == "" {
		return errors.New("agent script auth: agent_id, script_id required")
	}
	return r.db.WithContext(ctx).
		Where("agent_id = ? AND script_id = ?", agentID, scriptID).
		Delete(&CaichipAgentScriptAuth{}).Error
}

var _ biz.AgentScriptAuthRepo = (*AgentScriptAuthRepo)(nil)
