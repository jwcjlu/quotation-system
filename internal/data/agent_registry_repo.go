package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	var hp *string
	if hostname != "" {
		hp = &hostname
	}
	now := time.Now()
	ag := CaichipAgent{
		AgentID:             agentID,
		Queue:               q,
		Hostname:            hp,
		LastTaskHeartbeatAt: &now,
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "agent_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"queue":                  q,
			"hostname":               hp,
			"last_task_heartbeat_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
			"updated_at":             gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}),
	}).Create(&ag).Error; err != nil {
		return err
	}

	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Delete(&CaichipAgentTag{}).Error; err != nil {
		return err
	}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		row := CaichipAgentTag{AgentID: agentID, Tag: t}
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	for _, s := range scripts {
		sid := strings.TrimSpace(s.ScriptID)
		if sid == "" {
			continue
		}
		row := CaichipAgentInstalledScript{
			AgentID:   agentID,
			ScriptID:  sid,
			Version:   strings.TrimSpace(s.Version),
			EnvStatus: strings.TrimSpace(s.EnvStatus),
			UpdatedAt: time.Now(),
		}
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "agent_id"}, {Name: "script_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"version":    strings.TrimSpace(s.Version),
				"env_status": strings.TrimSpace(s.EnvStatus),
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
			}),
		}).Create(&row).Error; err != nil {
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
	var ag CaichipAgent
	err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&ag).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && strings.TrimSpace(ag.Queue) != "" {
		out.Queue = strings.TrimSpace(ag.Queue)
	}

	var tagRows []CaichipAgentTag
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&tagRows).Error; err != nil {
		return nil, err
	}
	for _, tr := range tagRows {
		t := strings.TrimSpace(tr.Tag)
		if t != "" {
			out.Tags[t] = struct{}{}
		}
	}

	var scriptRows []CaichipAgentInstalledScript
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&scriptRows).Error; err != nil {
		return nil, err
	}
	for _, sr := range scriptRows {
		s := biz.InstalledScript{
			ScriptID:  strings.TrimSpace(sr.ScriptID),
			Version:   strings.TrimSpace(sr.Version),
			EnvStatus: strings.TrimSpace(sr.EnvStatus),
		}
		if s.ScriptID != "" {
			out.Scripts[s.ScriptID] = s
		}
	}
	return out, nil
}

// ListAgentRegistrySummaries 全部 Agent 行（按 agent_id 排序），供运维列表。
func (r *AgentRegistryRepo) ListAgentRegistrySummaries(ctx context.Context) ([]biz.AgentRegistrySummary, error) {
	if !r.DBOk() {
		return nil, ErrDispatchTaskNoDB
	}
	var rows []CaichipAgent
	if err := r.db.WithContext(ctx).Order("agent_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.AgentRegistrySummary, 0, len(rows))
	for _, ag := range rows {
		s := biz.AgentRegistrySummary{
			AgentID:             strings.TrimSpace(ag.AgentID),
			Queue:               strings.TrimSpace(ag.Queue),
			LastTaskHeartbeatAt: ag.LastTaskHeartbeatAt,
		}
		if ag.Hostname != nil {
			s.Hostname = strings.TrimSpace(*ag.Hostname)
		}
		out = append(out, s)
	}
	return out, nil
}

// ListInstalledScriptsForAgent 某 Agent 已安装脚本快照。
func (r *AgentRegistryRepo) ListInstalledScriptsForAgent(ctx context.Context, agentID string) ([]biz.AgentInstalledScriptRow, error) {
	if !r.DBOk() {
		return nil, ErrDispatchTaskNoDB
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("agent registry: agent_id required")
	}
	var rows []CaichipAgentInstalledScript
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Order("script_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.AgentInstalledScriptRow, 0, len(rows))
	for _, sr := range rows {
		out = append(out, biz.AgentInstalledScriptRow{
			ScriptID:  strings.TrimSpace(sr.ScriptID),
			Version:   strings.TrimSpace(sr.Version),
			EnvStatus: strings.TrimSpace(sr.EnvStatus),
			UpdatedAt: sr.UpdatedAt,
		})
	}
	return out, nil
}

var _ biz.AgentRegistryRepo = (*AgentRegistryRepo)(nil)
