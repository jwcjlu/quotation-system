package data

import (
	"context"
	"errors"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

type UserSessionRepo struct {
	db *gorm.DB
}

func NewUserSessionRepo(d *Data) *UserSessionRepo {
	if d == nil {
		return &UserSessionRepo{}
	}
	return &UserSessionRepo{db: d.DB}
}

func (r *UserSessionRepo) Create(ctx context.Context, row *biz.UserSession) error {
	if r == nil || r.db == nil {
		return ErrDispatchTaskNoDB
	}
	dbRow := UserSession{
		UserID: row.UserID, TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt, LastSeenAt: row.LastSeenAt,
	}
	if err := r.db.WithContext(ctx).Create(&dbRow).Error; err != nil {
		return err
	}
	row.ID = dbRow.ID
	return nil
}

func (r *UserSessionRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (*biz.UserSession, error) {
	if r == nil || r.db == nil {
		return nil, ErrDispatchTaskNoDB
	}
	var row UserSession
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND expires_at > ?", tokenHash, now).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrAuthSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &biz.UserSession{
		ID: row.ID, UserID: row.UserID, TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt, LastSeenAt: row.LastSeenAt,
	}, nil
}

func (r *UserSessionRepo) Touch(ctx context.Context, id uint64, at time.Time) error {
	if r == nil || r.db == nil {
		return ErrDispatchTaskNoDB
	}
	return r.db.WithContext(ctx).Model(&UserSession{}).Where("id = ?", id).Update("last_seen_at", at).Error
}

func (r *UserSessionRepo) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	if r == nil || r.db == nil {
		return ErrDispatchTaskNoDB
	}
	return r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Delete(&UserSession{}).Error
}

var _ biz.AuthSessionRepo = (*UserSessionRepo)(nil)
