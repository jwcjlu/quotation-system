package data

import "time"

type UserAccount struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string     `gorm:"column:username;size:128;not null;uniqueIndex:uk_user_username"`
	DisplayName  string     `gorm:"column:display_name;size:128;not null"`
	PasswordHash string     `gorm:"column:password_hash;size:255;not null"`
	Role         string     `gorm:"column:role;size:16;not null;index:idx_user_role"`
	Status       string     `gorm:"column:status;size:16;not null;default:active"`
	LastLoginAt  *time.Time `gorm:"column:last_login_at;precision:3"`
	CreatedAt    time.Time  `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (UserAccount) TableName() string { return TableUser }

type UserSession struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID     uint64    `gorm:"column:user_id;not null;index:idx_user_session_user"`
	TokenHash  string    `gorm:"column:token_hash;size:64;not null;uniqueIndex:uk_user_session_token_hash"`
	ExpiresAt  time.Time `gorm:"column:expires_at;precision:3;not null;index:idx_user_session_expires"`
	LastSeenAt time.Time `gorm:"column:last_seen_at;precision:3;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
}

func (UserSession) TableName() string { return TableUserSession }
