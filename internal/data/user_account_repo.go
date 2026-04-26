package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

type UserAccountRepo struct {
	db *gorm.DB
}

func NewUserAccountRepo(d *Data) *UserAccountRepo {
	if d == nil {
		return &UserAccountRepo{}
	}
	return &UserAccountRepo{db: d.DB}
}

func (r *UserAccountRepo) Create(ctx context.Context, user *biz.UserAccount) error {
	if r == nil || r.db == nil {
		return ErrDispatchTaskNoDB
	}
	row := UserAccount{
		Username: strings.TrimSpace(user.Username), DisplayName: strings.TrimSpace(user.DisplayName),
		PasswordHash: user.PasswordHash, Role: string(user.Role), Status: user.Status,
	}
	if row.Status == "" {
		row.Status = "active"
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return biz.ErrAuthUserExists
		}
		return err
	}
	user.ID = row.ID
	user.Status = row.Status
	return nil
}

func (r *UserAccountRepo) GetByUsername(ctx context.Context, username string) (*biz.UserAccount, error) {
	if r == nil || r.db == nil {
		return nil, ErrDispatchTaskNoDB
	}
	var row UserAccount
	err := r.db.WithContext(ctx).Where("username = ?", strings.TrimSpace(username)).First(&row).Error
	return userAccountFromRow(row, err)
}

func (r *UserAccountRepo) GetByID(ctx context.Context, id uint64) (*biz.UserAccount, error) {
	if r == nil || r.db == nil {
		return nil, ErrDispatchTaskNoDB
	}
	var row UserAccount
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	return userAccountFromRow(row, err)
}

func (r *UserAccountRepo) UpdateLastLoginAt(ctx context.Context, userID uint64, at time.Time) error {
	if r == nil || r.db == nil {
		return ErrDispatchTaskNoDB
	}
	return r.db.WithContext(ctx).Model(&UserAccount{}).Where("id = ?", userID).Update("last_login_at", at).Error
}

func userAccountFromRow(row UserAccount, err error) (*biz.UserAccount, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrAuthUserNotFound
	}
	if err != nil {
		return nil, err
	}
	role, err := biz.NormalizeRole(row.Role)
	if err != nil {
		return nil, err
	}
	return &biz.UserAccount{
		ID: row.ID, Username: row.Username, DisplayName: row.DisplayName,
		PasswordHash: row.PasswordHash, Role: role, Status: row.Status, LastLoginAt: row.LastLoginAt,
	}, nil
}

func isDuplicateKey(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate")
}

var _ biz.AuthUserRepo = (*UserAccountRepo)(nil)
