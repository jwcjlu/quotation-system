package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"caichip/internal/biz"
)

type fakeAuthUserRepo struct {
	byID       map[uint64]*biz.UserAccount
	byUsername map[string]*biz.UserAccount
	nextID     uint64
}

func newFakeAuthUserRepo() *fakeAuthUserRepo {
	return &fakeAuthUserRepo{byID: map[uint64]*biz.UserAccount{}, byUsername: map[string]*biz.UserAccount{}, nextID: 1}
}

func (r *fakeAuthUserRepo) Create(_ context.Context, user *biz.UserAccount) error {
	if _, ok := r.byUsername[user.Username]; ok {
		return biz.ErrAuthUserExists
	}
	cp := *user
	cp.ID = r.nextID
	r.nextID++
	r.byID[cp.ID] = &cp
	r.byUsername[cp.Username] = &cp
	user.ID = cp.ID
	return nil
}

func (r *fakeAuthUserRepo) GetByUsername(_ context.Context, username string) (*biz.UserAccount, error) {
	u, ok := r.byUsername[username]
	if !ok {
		return nil, biz.ErrAuthUserNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *fakeAuthUserRepo) GetByID(_ context.Context, id uint64) (*biz.UserAccount, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, biz.ErrAuthUserNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *fakeAuthUserRepo) UpdateLastLoginAt(_ context.Context, userID uint64, at time.Time) error {
	u, ok := r.byID[userID]
	if !ok {
		return biz.ErrAuthUserNotFound
	}
	u.LastLoginAt = &at
	return nil
}

type fakeAuthSessionRepo struct {
	byHash map[string]*biz.UserSession
	nextID uint64
}

func newFakeAuthSessionRepo() *fakeAuthSessionRepo {
	return &fakeAuthSessionRepo{byHash: map[string]*biz.UserSession{}, nextID: 1}
}

func (r *fakeAuthSessionRepo) Create(_ context.Context, row *biz.UserSession) error {
	cp := *row
	cp.ID = r.nextID
	r.nextID++
	r.byHash[cp.TokenHash] = &cp
	row.ID = cp.ID
	return nil
}

func (r *fakeAuthSessionRepo) GetActiveByTokenHash(_ context.Context, tokenHash string, now time.Time) (*biz.UserSession, error) {
	row, ok := r.byHash[tokenHash]
	if !ok || !row.ExpiresAt.After(now) {
		return nil, biz.ErrAuthSessionNotFound
	}
	cp := *row
	return &cp, nil
}

func (r *fakeAuthSessionRepo) Touch(_ context.Context, id uint64, at time.Time) error {
	for _, row := range r.byHash {
		if row.ID == id {
			row.LastSeenAt = at
			return nil
		}
	}
	return biz.ErrAuthSessionNotFound
}

func (r *fakeAuthSessionRepo) DeleteByTokenHash(_ context.Context, tokenHash string) error {
	delete(r.byHash, tokenHash)
	return nil
}

func TestAuthServiceRegisterLoginAndMe(t *testing.T) {
	users := newFakeAuthUserRepo()
	sessions := newFakeAuthSessionRepo()
	svc := NewAuthService(users, sessions)

	registered, err := svc.Register(context.Background(), RegisterUserInput{
		Username:    "alice",
		DisplayName: "Alice",
		Password:    "Secret123!",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registered.Role != biz.RoleUser {
		t.Fatalf("registered role = %q, want user", registered.Role)
	}
	stored := users.byUsername["alice"]
	if stored.PasswordHash == "" || stored.PasswordHash == "Secret123!" {
		t.Fatal("Register() did not store a password hash")
	}

	login, err := svc.Login(context.Background(), "alice", "Secret123!")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.SessionToken == "" {
		t.Fatal("Login() returned empty session token")
	}

	me, err := svc.Me(context.Background(), "Bearer "+login.SessionToken)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if me.Username != "alice" || me.Role != biz.RoleUser {
		t.Fatalf("Me() = %#v, want alice user", me)
	}
}

func TestAuthServiceRejectsBadPassword(t *testing.T) {
	svc := NewAuthService(newFakeAuthUserRepo(), newFakeAuthSessionRepo())
	_, err := svc.Register(context.Background(), RegisterUserInput{
		Username:    "bob",
		DisplayName: "Bob",
		Password:    "Secret123!",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := svc.Login(context.Background(), "bob", "wrong"); !errors.Is(err, ErrAuthInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrAuthInvalidCredentials", err)
	}
}
