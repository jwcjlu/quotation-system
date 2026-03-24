package data

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// AgentRegistryRepo Agent 元数据表（caichip_agent / tag / installed_script），供 DB 调度 match 与离线判断。
type AgentRegistryRepo struct {
	db *gorm.DB
}

// NewAgentRegistryRepo ...
func NewAgentRegistryRepo(d *Data) *AgentRegistryRepo {
	if d == nil || d.DB == nil {
		return &AgentRegistryRepo{}
	}
	return &AgentRegistryRepo{db: d.DB}
}

// DBOk ...
func (r *AgentRegistryRepo) DBOk() bool {
	return r != nil && r.db != nil
}

// UpsertTaskHeartbeat 任务心跳时刷新 Agent 快照（与 agent_mysql.sql 一致）。
func (r *AgentRegistryRepo) UpsertTaskHeartbeat(ctx context.Context, agentID, queue, hostname string, scripts []biz.InstalledScript, tags []string) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return errors.New("agent registry: agent_id required")
	}
	q := strings.TrimSpace(queue)
	if q == "" {
		q = "default"
	}
	hostname = strings.TrimSpace(hostname)
	err := r.db.WithContext(ctx).Exec(`
INSERT INTO caichip_agent (agent_id, queue, hostname, last_task_heartbeat_at, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  queue = VALUES(queue),
  hostname = VALUES(hostname),
  last_task_heartbeat_at = CURRENT_TIMESTAMP(3),
  updated_at = CURRENT_TIMESTAMP(3)`,
		agentID, q, nullIfEmpty(hostname)).Error
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Exec(`DELETE FROM caichip_agent_tag WHERE agent_id = ?`, agentID).Error; err != nil {
		return err
	}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if err := r.db.WithContext(ctx).Exec(`INSERT INTO caichip_agent_tag (agent_id, tag) VALUES (?, ?)`, agentID, t).Error; err != nil {
			return err
		}
	}
	for _, s := range scripts {
		sid := strings.TrimSpace(s.ScriptID)
		if sid == "" {
			continue
		}
		err = r.db.WithContext(ctx).Exec(`
INSERT INTO caichip_agent_installed_script (agent_id, script_id, version, env_status, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  version = VALUES(version),
  env_status = VALUES(env_status),
  updated_at = CURRENT_TIMESTAMP(3)`,
			agentID, sid, strings.TrimSpace(s.Version), strings.TrimSpace(s.EnvStatus)).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// LoadSchedulingMeta 读入用于 MatchTaskForAgent 的快照；无行则返回默认队列与空能力图。
func (r *AgentRegistryRepo) LoadSchedulingMeta(ctx context.Context, agentID string) (*biz.AgentSchedulingMeta, error) {
	if !r.DBOk() {
		return nil, ErrDispatchTaskNoDB
	}
	agentID = strings.TrimSpace(agentID)
	out := &biz.AgentSchedulingMeta{
		Queue:   "default",
		Tags:    make(map[string]struct{}),
		Scripts: make(map[string]biz.InstalledScript),
	}
	var queue sql.NullString
	err := r.db.WithContext(ctx).Raw(`SELECT queue FROM caichip_agent WHERE agent_id = ?`, agentID).Row().Scan(&queue)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if queue.Valid && strings.TrimSpace(queue.String) != "" {
		out.Queue = strings.TrimSpace(queue.String)
	}
	tagRows, err := r.db.WithContext(ctx).Raw(`SELECT tag FROM caichip_agent_tag WHERE agent_id = ?`, agentID).Rows()
	if err != nil {
		return nil, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var tag string
		if err = tagRows.Scan(&tag); err != nil {
			return nil, err
		}
		tag = strings.TrimSpace(tag)
		if tag != "" {
			out.Tags[tag] = struct{}{}
		}
	}
	if err = tagRows.Err(); err != nil {
		return nil, err
	}
	scriptRows, err := r.db.WithContext(ctx).Raw(`
SELECT script_id, version, env_status FROM caichip_agent_installed_script WHERE agent_id = ?`, agentID).Rows()
	if err != nil {
		return nil, err
	}
	defer scriptRows.Close()
	for scriptRows.Next() {
		var s biz.InstalledScript
		if err = scriptRows.Scan(&s.ScriptID, &s.Version, &s.EnvStatus); err != nil {
			return nil, err
		}
		s.ScriptID = strings.TrimSpace(s.ScriptID)
		if s.ScriptID != "" {
			out.Scripts[s.ScriptID] = s
		}
	}
	return out, scriptRows.Err()
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
