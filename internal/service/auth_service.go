package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"caichip/internal/biz"

	"golang.org/x/crypto/bcrypt"
)

const authSessionTTL = 24 * time.Hour

var (
	ErrAuthInvalidCredentials = errors.New("auth: invalid username or password")
	ErrAuthUnauthorized       = errors.New("auth: unauthorized")
)

type RegisterUserInput struct {
	Username    string
	DisplayName string
	Password    string
}

type PublicAuthUser struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Role        biz.Role `json:"role"`
	Status      string   `json:"status"`
}

type LoginResult struct {
	SessionToken string         `json:"session_token"`
	User         PublicAuthUser `json:"user"`
}

type AuthService struct {
	users    biz.AuthUserRepo
	sessions biz.AuthSessionRepo
	now      func() time.Time
}

func NewAuthService(users biz.AuthUserRepo, sessions biz.AuthSessionRepo) *AuthService {
	return &AuthService{users: users, sessions: sessions, now: time.Now}
}

func (s *AuthService) Register(ctx context.Context, in RegisterUserInput) (PublicAuthUser, error) {
	username, displayName, password, err := normalizeRegisterInput(in)
	if err != nil {
		return PublicAuthUser{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return PublicAuthUser{}, err
	}
	user := &biz.UserAccount{
		Username: username, DisplayName: displayName,
		PasswordHash: string(hash), Role: biz.RoleUser, Status: "active",
	}
	if err := s.users.Create(ctx, user); err != nil {
		return PublicAuthUser{}, err
	}
	return publicUser(user), nil
}

func (s *AuthService) Login(ctx context.Context, username, password string) (LoginResult, error) {
	user, err := s.users.GetByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, biz.ErrAuthUserNotFound) {
			return LoginResult{}, ErrAuthInvalidCredentials
		}
		return LoginResult{}, err
	}
	if user.Status != "active" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return LoginResult{}, ErrAuthInvalidCredentials
	}
	token, err := newSessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	now := s.now()
	row := &biz.UserSession{UserID: user.ID, TokenHash: TokenHash(token), ExpiresAt: now.Add(authSessionTTL), LastSeenAt: now}
	if err := s.sessions.Create(ctx, row); err != nil {
		return LoginResult{}, err
	}
	_ = s.users.UpdateLastLoginAt(ctx, user.ID, now)
	return LoginResult{SessionToken: token, User: publicUser(user)}, nil
}

func (s *AuthService) Me(ctx context.Context, authorization string) (PublicAuthUser, error) {
	user, _, err := s.currentUser(ctx, authorization)
	return user, err
}

func (s *AuthService) Logout(ctx context.Context, authorization string) error {
	token := bearerToken(authorization)
	if token == "" {
		return nil
	}
	return s.sessions.DeleteByTokenHash(ctx, TokenHash(token))
}

func (s *AuthService) currentUser(ctx context.Context, authorization string) (PublicAuthUser, *biz.UserSession, error) {
	token := bearerToken(authorization)
	if token == "" {
		return PublicAuthUser{}, nil, ErrAuthUnauthorized
	}
	now := s.now()
	sess, err := s.sessions.GetActiveByTokenHash(ctx, TokenHash(token), now)
	if err != nil {
		return PublicAuthUser{}, nil, ErrAuthUnauthorized
	}
	user, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil || user.Status != "active" {
		return PublicAuthUser{}, nil, ErrAuthUnauthorized
	}
	_ = s.sessions.Touch(ctx, sess.ID, now)
	return publicUser(user), sess, nil
}

func normalizeRegisterInput(in RegisterUserInput) (string, string, string, error) {
	username := strings.TrimSpace(in.Username)
	displayName := strings.TrimSpace(in.DisplayName)
	if username == "" || displayName == "" || in.Password == "" {
		return "", "", "", &BadRequestError{Message: "username, display_name and password required"}
	}
	if len(in.Password) < 8 {
		return "", "", "", &BadRequestError{Message: "password must be at least 8 characters"}
	}
	return username, displayName, in.Password, nil
}

func publicUser(u *biz.UserAccount) PublicAuthUser {
	if u == nil {
		return PublicAuthUser{}
	}
	return PublicAuthUser{ID: uint64String(u.ID), Username: u.Username, DisplayName: u.DisplayName, Role: u.Role, Status: u.Status}
}

func uint64String(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func bearerToken(auth string) string {
	auth = strings.TrimSpace(auth)
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return ""
}
