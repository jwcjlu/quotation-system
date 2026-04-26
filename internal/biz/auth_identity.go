package biz

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

var (
	ErrAuthUserExists       = errors.New("auth: user exists")
	ErrAuthUserNotFound     = errors.New("auth: user not found")
	ErrAuthSessionNotFound  = errors.New("auth: session not found")
	ErrAuthInvalidRole      = errors.New("auth: invalid role")
	ErrAuthInvalidUserState = errors.New("auth: invalid user state")
)

func NormalizeRole(raw string) (Role, error) {
	switch Role(strings.TrimSpace(raw)) {
	case RoleUser:
		return RoleUser, nil
	case RoleAdmin:
		return RoleAdmin, nil
	default:
		return "", ErrAuthInvalidRole
	}
}

func (r Role) Allows(actual Role) bool {
	if r == RoleUser {
		return actual == RoleUser || actual == RoleAdmin
	}
	return actual == RoleAdmin
}

type UserAccount struct {
	ID           uint64
	Username     string
	DisplayName  string
	PasswordHash string
	Role         Role
	Status       string
	LastLoginAt  *time.Time
}

type UserSession struct {
	ID         uint64
	UserID     uint64
	TokenHash  string
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

type AuthUserRepo interface {
	Create(ctx context.Context, user *UserAccount) error
	GetByUsername(ctx context.Context, username string) (*UserAccount, error)
	GetByID(ctx context.Context, id uint64) (*UserAccount, error)
	UpdateLastLoginAt(ctx context.Context, userID uint64, at time.Time) error
}

type AuthSessionRepo interface {
	Create(ctx context.Context, row *UserSession) error
	GetActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (*UserSession, error)
	Touch(ctx context.Context, id uint64, at time.Time) error
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
}
