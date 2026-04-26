# Auth Access Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add user registration/login, role-based access control, and write-operation audit logging across the BOM web system without breaking existing BOM/HS/Agent flows.

**Architecture:** Add a small auth domain in `internal/biz` and `internal/data` for users, sessions, and audit logs; expose login/register/current-user/admin-create-user APIs through a new `api/auth/v1` service; enforce access with a single HTTP middleware that maps request paths to public, user, admin, and agent-only scopes; then switch the React app to a shared bearer-token auth client and hide/protect nav entries by role.

**Tech Stack:** Kratos HTTP + protobuf APIs, GORM/MySQL, `golang.org/x/crypto/bcrypt`, React 19 + Vitest.

---

### Task 1: Add Auth/Audit Domain Contracts And Persistence Models

**Files:**
- Create: `docs/schema/migrations/20260423_auth_access_audit.sql`
- Create: `internal/biz/auth_identity.go`
- Create: `internal/biz/auth_repo.go`
- Create: `internal/biz/audit_log.go`
- Create: `internal/data/auth_models.go`
- Create: `internal/data/user_account_repo.go`
- Create: `internal/data/user_session_repo.go`
- Create: `internal/data/audit_log_repo.go`
- Create: `internal/biz/auth_identity_test.go`
- Modify: `internal/data/migrate.go`
- Modify: `internal/data/provider.go`
- Modify: `cmd/server/wire.go`

- [ ] **Step 1: Write the failing domain test**

```go
package biz

import "testing"

func TestAccessScopeAllows(t *testing.T) {
	tests := []struct {
		scope Role
		wantUser bool
		wantAdmin bool
	}{
		{scope: RoleUser, wantUser: true, wantAdmin: true},
		{scope: RoleAdmin, wantUser: false, wantAdmin: true},
	}
	for _, tt := range tests {
		if got := tt.scope.Allows(RoleUser); got != tt.wantUser {
			t.Fatalf("scope %q user allow = %v, want %v", tt.scope, got, tt.wantUser)
		}
		if got := tt.scope.Allows(RoleAdmin); got != tt.wantAdmin {
			t.Fatalf("scope %q admin allow = %v, want %v", tt.scope, got, tt.wantAdmin)
		}
	}
}

func TestNormalizeRole(t *testing.T) {
	if got, err := NormalizeRole("admin"); err != nil || got != RoleAdmin {
		t.Fatalf("NormalizeRole(admin) = %v, %v", got, err)
	}
	if _, err := NormalizeRole("owner"); err == nil {
		t.Fatal("expected invalid role error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/biz -run 'TestAccessScopeAllows|TestNormalizeRole' -count=1`

Expected: FAIL with undefined `Role`, `Allows`, or `NormalizeRole`.

- [ ] **Step 3: Write the minimal domain + persistence implementation**

```go
package biz

import (
	"context"
	"errors"
	"time"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

func NormalizeRole(raw string) (Role, error) {
	switch Role(raw) {
	case RoleUser:
		return RoleUser, nil
	case RoleAdmin:
		return RoleAdmin, nil
	default:
		return "", errors.New("auth: invalid role")
	}
}

func (r Role) Allows(actual Role) bool {
	if r == RoleUser {
		return actual == RoleUser || actual == RoleAdmin
	}
	return actual == RoleAdmin
}

type UserAccount struct {
	ID          uint64
	Username    string
	DisplayName string
	PasswordHash string
	Role        Role
	Status      string
	LastLoginAt *time.Time
}

type AuthUserRepo interface {
	Create(ctx context.Context, user *UserAccount) error
	GetByUsername(ctx context.Context, username string) (*UserAccount, error)
	GetByID(ctx context.Context, id uint64) (*UserAccount, error)
	UpdateLastLoginAt(ctx context.Context, userID uint64, at time.Time) error
}

type UserSession struct {
	ID        uint64
	UserID    uint64
	TokenHash string
	ExpiresAt time.Time
	LastSeenAt time.Time
}

type AuthSessionRepo interface {
	Create(ctx context.Context, row *UserSession) error
	GetActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (*UserSession, error)
	Touch(ctx context.Context, id uint64, at time.Time) error
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
}

type AuditLog struct {
	ID           uint64
	ActorUserID  uint64
	ActorUsername string
	Action       string
	ResourceType string
	ResourceID   string
	Summary      string
	DetailJSON   []byte
	CreatedAt    time.Time
}

type AuditLogRepo interface {
	Create(ctx context.Context, row *AuditLog) error
}
```

```go
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

type UserSession struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID     uint64    `gorm:"column:user_id;not null;index:idx_user_session_user"`
	TokenHash  string    `gorm:"column:token_hash;size:64;not null;uniqueIndex:uk_user_session_token_hash"`
	ExpiresAt  time.Time `gorm:"column:expires_at;precision:3;not null;index:idx_user_session_expires"`
	LastSeenAt time.Time `gorm:"column:last_seen_at;precision:3;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
}

type AuditLog struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	ActorUserID   uint64    `gorm:"column:actor_user_id;not null;index:idx_audit_actor"`
	ActorUsername string    `gorm:"column:actor_username;size:128;not null"`
	Action        string    `gorm:"column:action;size:32;not null;index:idx_audit_action"`
	ResourceType  string    `gorm:"column:resource_type;size:64;not null;index:idx_audit_resource"`
	ResourceID    string    `gorm:"column:resource_id;size:128;not null"`
	Summary       string    `gorm:"column:summary;size:255;not null"`
	DetailJSON    []byte    `gorm:"column:detail_json;type:json"`
	CreatedAt     time.Time `gorm:"column:created_at;precision:3;autoCreateTime;index:idx_audit_created"`
}
```

```sql
CREATE TABLE IF NOT EXISTS t_user (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(128) NOT NULL,
  display_name VARCHAR(128) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  last_login_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_user_username (username)
);

CREATE TABLE IF NOT EXISTS t_user_session (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  token_hash CHAR(64) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  last_seen_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_user_session_token_hash (token_hash),
  KEY idx_user_session_user (user_id),
  KEY idx_user_session_expires (expires_at)
);

CREATE TABLE IF NOT EXISTS t_audit_log (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  actor_user_id BIGINT UNSIGNED NOT NULL,
  actor_username VARCHAR(128) NOT NULL,
  action VARCHAR(32) NOT NULL,
  resource_type VARCHAR(64) NOT NULL,
  resource_id VARCHAR(128) NOT NULL,
  summary VARCHAR(255) NOT NULL,
  detail_json JSON NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_audit_actor (actor_user_id),
  KEY idx_audit_action (action),
  KEY idx_audit_resource (resource_type, resource_id),
  KEY idx_audit_created (created_at)
);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/biz -run 'TestAccessScopeAllows|TestNormalizeRole' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add docs/schema/migrations/20260423_auth_access_audit.sql internal/biz/auth_identity.go internal/biz/auth_repo.go internal/biz/audit_log.go internal/biz/auth_identity_test.go internal/data/auth_models.go internal/data/user_account_repo.go internal/data/user_session_repo.go internal/data/audit_log_repo.go internal/data/migrate.go internal/data/provider.go cmd/server/wire.go
git commit -m "feat(auth): add auth and audit persistence"
```

### Task 2: Add Auth API And Service Logic

**Files:**
- Create: `api/auth/v1/auth.proto`
- Create: `internal/service/auth_service.go`
- Create: `internal/service/auth_service_test.go`
- Modify: `internal/service/provider.go`
- Modify: `internal/server/http.go`
- Modify: `cmd/server/wire.go`

- [ ] **Step 1: Write the failing service test**

```go
package service

import (
	"context"
	"testing"
	"time"

	"caichip/internal/biz"
)

func TestAuthService_RegisterCreatesUserRole(t *testing.T) {
	users := &stubAuthUserRepo{}
	svc := NewAuthService(users, &stubAuthSessionRepo{}, &stubAuditLogRepo{}, nil)

	out, err := svc.Register(context.Background(), &authv1.RegisterRequest{
		Username: "alice",
		DisplayName: "Alice",
		Password: "Passw0rd!",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.GetRole() != "user" {
		t.Fatalf("role = %q, want user", out.GetRole())
	}
	if users.created == nil || users.created.Role != biz.RoleUser {
		t.Fatalf("created user role = %+v, want user", users.created)
	}
}

func TestAuthService_AdminCreateUserRequiresAdmin(t *testing.T) {
	svc := NewAuthService(&stubAuthUserRepo{}, &stubAuthSessionRepo{}, &stubAuditLogRepo{}, nil)
	_, err := svc.AdminCreateUser(context.Background(), &authv1.AdminCreateUserRequest{
		Username: "root2",
		DisplayName: "Root Two",
		Password: "Passw0rd!",
		Role: "admin",
	})
	if err == nil {
		t.Fatal("expected permission error")
	}
}

func TestAuthService_LoginCreatesSession(t *testing.T) {
	hash, _ := hashPassword("Passw0rd!")
	users := &stubAuthUserRepo{
		byUsername: &biz.UserAccount{ID: 9, Username: "alice", DisplayName: "Alice", PasswordHash: hash, Role: biz.RoleUser, Status: "active"},
	}
	sessions := &stubAuthSessionRepo{}
	svc := NewAuthService(users, sessions, &stubAuditLogRepo{}, func() time.Time { return time.Unix(1700000000, 0) })

	out, err := svc.Login(context.Background(), &authv1.LoginRequest{Username: "alice", Password: "Passw0rd!"})
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if out.GetToken() == "" || sessions.created == nil {
		t.Fatalf("expected session token and stored session, got %#v / %#v", out, sessions.created)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service -run 'TestAuthService_(RegisterCreatesUserRole|AdminCreateUserRequiresAdmin|LoginCreatesSession)' -count=1`

Expected: FAIL with missing `NewAuthService`, auth protobuf types, or password helper functions.

- [ ] **Step 3: Write the minimal API and service implementation**

```proto
syntax = "proto3";

package api.auth.v1;

import "google/api/annotations.proto";

option go_package = "caichip/api/auth/v1;authv1";

service AuthService {
  rpc Register(RegisterRequest) returns (AuthUserReply) {
    option (google.api.http) = { post: "/api/v1/auth/register" body: "*" };
  }
  rpc Login(LoginRequest) returns (LoginReply) {
    option (google.api.http) = { post: "/api/v1/auth/login" body: "*" };
  }
  rpc Me(MeRequest) returns (AuthUserReply) {
    option (google.api.http) = { get: "/api/v1/auth/me" };
  }
  rpc Logout(LogoutRequest) returns (LogoutReply) {
    option (google.api.http) = { post: "/api/v1/auth/logout" body: "*" };
  }
  rpc AdminCreateUser(AdminCreateUserRequest) returns (AuthUserReply) {
    option (google.api.http) = { post: "/api/v1/auth/admin/users" body: "*" };
  }
}
```

```go
func (s *AuthService) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.AuthUserReply, error) {
	hash, err := hashPassword(req.GetPassword())
	if err != nil {
		return nil, kerrors.BadRequest("AUTH_PASSWORD", err.Error())
	}
	row := &biz.UserAccount{
		Username: req.GetUsername(),
		DisplayName: req.GetDisplayName(),
		PasswordHash: hash,
		Role: biz.RoleUser,
		Status: "active",
	}
	if err := s.users.Create(ctx, row); err != nil {
		return nil, err
	}
	return &authv1.AuthUserReply{
		UserId: row.ID,
		Username: row.Username,
		DisplayName: row.DisplayName,
		Role: string(row.Role),
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginReply, error) {
	user, err := s.users.GetByUsername(ctx, req.GetUsername())
	if err != nil {
		return nil, err
	}
	if err := verifyPassword(user.PasswordHash, req.GetPassword()); err != nil {
		return nil, kerrors.Unauthorized("AUTH_LOGIN_FAILED", "invalid username or password")
	}
	token := newSessionToken()
	now := s.now()
	if err := s.sessions.Create(ctx, &biz.UserSession{
		UserID: user.ID,
		TokenHash: sha256Hex(token),
		ExpiresAt: now.Add(24 * time.Hour),
		LastSeenAt: now,
	}); err != nil {
		return nil, err
	}
	_ = s.users.UpdateLastLoginAt(ctx, user.ID, now)
	return &authv1.LoginReply{
		Token: token,
		User: &authv1.AuthUserReply{
			UserId: user.ID,
			Username: user.Username,
			DisplayName: user.DisplayName,
			Role: string(user.Role),
		},
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service -run 'TestAuthService_(RegisterCreatesUserRole|AdminCreateUserRequiresAdmin|LoginCreatesSession)' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/auth/v1/auth.proto api/auth/v1/auth.pb.go api/auth/v1/auth_http.pb.go api/auth/v1/auth_grpc.pb.go internal/service/auth_service.go internal/service/auth_service_test.go internal/service/provider.go internal/server/http.go cmd/server/wire.go
git commit -m "feat(auth): add auth service endpoints"
```

### Task 3: Add HTTP Auth Middleware And Protect Existing APIs

**Files:**
- Create: `internal/server/auth_middleware.go`
- Create: `internal/server/auth_context.go`
- Create: `internal/server/auth_middleware_test.go`
- Modify: `internal/server/http.go`
- Modify: `internal/server/script_admin_http.go`
- Modify: `internal/service/agent_admin.go`

- [ ] **Step 1: Write the failing middleware test**

```go
package server

import (
	"context"
	"testing"

	"caichip/internal/biz"
)

func TestRequireAuthMiddleware_RejectsAnonymous(t *testing.T) {
	mw := requireRoleMiddleware("/api/v1/bom-sessions", biz.RoleUser, &stubAuthResolver{})
	_, err := mw(func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Fatal("handler should not run")
		return nil, nil
	})(context.Background(), nil)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
}

func TestRequireAuthMiddleware_RejectsInsufficientRole(t *testing.T) {
	mw := requireRoleMiddleware("/api/hs/meta/list", biz.RoleAdmin, &stubAuthResolver{
		user: &biz.AuthenticatedUser{UserID: 1, Username: "alice", Role: biz.RoleUser},
	})
	_, err := mw(func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Fatal("handler should not run")
		return nil, nil
	})(context.Background(), nil)
	if err == nil {
		t.Fatal("expected forbidden error")
	}
}

func TestRequireAuthMiddleware_AllowsAdmin(t *testing.T) {
	mw := requireRoleMiddleware("/api/hs/meta/list", biz.RoleAdmin, &stubAuthResolver{
		user: &biz.AuthenticatedUser{UserID: 2, Username: "root", Role: biz.RoleAdmin},
	})
	called := false
	_, err := mw(func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return nil, nil
	})(context.Background(), nil)
	if err != nil || !called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server -run 'TestRequireAuthMiddleware_(RejectsAnonymous|RejectsInsufficientRole|AllowsAdmin)' -count=1`

Expected: FAIL with missing middleware helpers or auth resolver types.

- [ ] **Step 3: Write the minimal middleware and route protection**

```go
func requireRoleMiddleware(path string, required biz.Role, resolver AuthResolver) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if strings.HasPrefix(path, "/api/v1/agent") {
				return next(ctx, req)
			}
			if strings.HasPrefix(path, "/api/v1/auth/") {
				return next(ctx, req)
			}
			user, err := resolver.Resolve(ctx)
			if err != nil || user == nil {
				return nil, kerrors.Unauthorized("UNAUTHORIZED", "login required")
			}
			if !required.Allows(user.Role) {
				return nil, kerrors.Forbidden("FORBIDDEN", "permission denied")
			}
			ctx = context.WithValue(ctx, authUserContextKey{}, user)
			return next(ctx, req)
		}
	}
}

func CurrentAuthUser(ctx context.Context) *biz.AuthenticatedUser {
	user, _ := ctx.Value(authUserContextKey{}).(*biz.AuthenticatedUser)
	return user
}
```

```go
func authScriptAdmin(ctx khttp.Context, resolver server.AuthResolver) (*biz.AuthenticatedUser, bool) {
	user, err := resolver.Resolve(ctx.Request().Context())
	if err != nil || user == nil || user.Role != biz.RoleAdmin {
		return nil, false
	}
	return user, true
}
```

```go
func (s *AgentAdminService) ensureAdmin(ctx context.Context) (*biz.AuthenticatedUser, error) {
	user := server.CurrentAuthUser(ctx)
	if user == nil {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "login required")
	}
	if user.Role != biz.RoleAdmin {
		return nil, kerrors.Forbidden("FORBIDDEN", "admin required")
	}
	return user, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server -run 'TestRequireAuthMiddleware_(RejectsAnonymous|RejectsInsufficientRole|AllowsAdmin)' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/auth_middleware.go internal/server/auth_context.go internal/server/auth_middleware_test.go internal/server/http.go internal/server/script_admin_http.go internal/service/agent_admin.go
git commit -m "feat(auth): protect routes by role"
```

### Task 4: Attach Audit Logging To Existing Write Paths

**Files:**
- Create: `internal/service/audit_helper.go`
- Create: `internal/service/auth_audit_test.go`
- Modify: `internal/service/bom_service.go`
- Modify: `internal/service/hs_meta_service.go`
- Modify: `internal/service/agent_admin.go`
- Modify: `internal/service/script_package_admin.go`
- Modify: `internal/service/hs_resolve_manual_upload.go`

- [ ] **Step 1: Write the failing audit tests**

```go
package service

import (
	"context"
	"testing"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
)

func TestHsMetaService_CreateWritesAuditLog(t *testing.T) {
	audit := &stubAuditLogRepo{}
	svc := NewHsMetaServiceWithAudit(&stubHsMetaRepo{}, audit)
	ctx := withAuthUser(context.Background(), &biz.AuthenticatedUser{
		UserID: 7,
		Username: "root",
		Role: biz.RoleAdmin,
	})

	_, err := svc.CreateHsMeta(ctx, &v1.HsMetaCreateRequest{
		Category: "ic",
		ComponentName: "MCU",
		CoreHs6: "854231",
		Description: "microcontroller",
		Enabled: true,
		SortOrder: 10,
	})
	if err != nil {
		t.Fatalf("CreateHsMeta error: %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != "create" || audit.rows[0].ResourceType != "hs_meta" {
		t.Fatalf("audit rows = %+v", audit.rows)
	}
}

func TestBomService_CreateSessionLineWritesAuditLog(t *testing.T) {
	audit := &stubAuditLogRepo{}
	svc := NewBomServiceWithAudit(&bomSessionRepoStub{}, &bomSearchTaskRepoStub{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, audit, nil)
	ctx := withAuthUser(context.Background(), &biz.AuthenticatedUser{
		UserID: 9,
		Username: "alice",
		Role: biz.RoleUser,
	})

	_, _ = svc.CreateSessionLine(ctx, &v1.CreateSessionLineRequest{
		SessionId: "11111111-1111-1111-1111-111111111111",
		Mpn: "LM358",
	})
	if len(audit.rows) != 1 || audit.rows[0].ResourceType != "bom_session_line" {
		t.Fatalf("audit rows = %+v", audit.rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service -run 'Test(HsMetaService_CreateWritesAuditLog|BomService_CreateSessionLineWritesAuditLog)' -count=1`

Expected: FAIL with missing audit injection or missing audit writes.

- [ ] **Step 3: Write the minimal audit helper and service hooks**

```go
func writeAudit(ctx context.Context, repo biz.AuditLogRepo, action, resourceType, resourceID, summary string, detail any) {
	if repo == nil {
		return
	}
	user := server.CurrentAuthUser(ctx)
	if user == nil {
		return
	}
	raw, _ := json.Marshal(detail)
	_ = repo.Create(ctx, &biz.AuditLog{
		ActorUserID: user.UserID,
		ActorUsername: user.Username,
		Action: action,
		ResourceType: resourceType,
		ResourceID: resourceID,
		Summary: summary,
		DetailJSON: raw,
	})
}
```

```go
if err := s.repo.Create(ctx, row); err != nil {
	return nil, kerrors.InternalServer("HS_META_CREATE", err.Error())
}
writeAudit(ctx, s.audit, "create", "hs_meta", strconv.FormatUint(row.ID, 10), "create hs meta", map[string]any{
	"category": row.Category,
	"component_name": row.ComponentName,
	"core_hs6": row.CoreHS6,
})
```

```go
id, lineNo, rev, err := s.session.CreateSessionLine(ctx, req.GetSessionId(), req.GetMpn(), req.GetMfr(), req.GetPackage(), qty, raw, extra)
if err != nil {
	return nil, err
}
writeAudit(ctx, s.audit, "create", "bom_session_line", strconv.FormatInt(id, 10), "create bom session line", map[string]any{
	"session_id": req.GetSessionId(),
	"line_no": lineNo,
	"mpn": req.GetMpn(),
	"selection_revision": rev,
})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service -run 'Test(HsMetaService_CreateWritesAuditLog|BomService_CreateSessionLineWritesAuditLog)' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/audit_helper.go internal/service/auth_audit_test.go internal/service/bom_service.go internal/service/hs_meta_service.go internal/service/agent_admin.go internal/service/script_package_admin.go internal/service/hs_resolve_manual_upload.go
git commit -m "feat(audit): log write operations"
```

### Task 5: Add Frontend Auth Client, Session State, And Navigation Gating

**Files:**
- Create: `web/src/api/auth.ts`
- Create: `web/src/auth/session.ts`
- Create: `web/src/components/AuthPanel.tsx`
- Modify: `web/src/api/http.ts`
- Modify: `web/src/api/index.ts`
- Modify: `web/src/api/agentAdmin.ts`
- Modify: `web/src/api/agentScripts.ts`
- Modify: `web/src/api/hsMeta.ts`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`

- [ ] **Step 1: Write the failing frontend gating test**

```tsx
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import App from './App'

const { getMe } = vi.hoisted(() => ({ getMe: vi.fn() }))

vi.mock('./api/auth', () => ({
  getMe,
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
}))

describe('App auth gating', () => {
  it('shows only guide when anonymous', async () => {
    getMe.mockRejectedValue(new Error('401'))
    render(<App />)
    expect(await screen.findByRole('button', { name: '使用指南' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Agent运维' })).not.toBeInTheDocument()
  })

  it('shows only user scopes for normal user', async () => {
    getMe.mockResolvedValue({ user_id: 1, username: 'alice', display_name: 'Alice', role: 'user' })
    render(<App />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'BOM会话' })).toBeInTheDocument())
    expect(screen.getByRole('button', { name: '匹配单' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS型号解析' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Agent运维' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'HS元数据' })).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- src/App.test.tsx`

Expected: FAIL because `App` still renders unrestricted nav and has no auth bootstrapping.

- [ ] **Step 3: Write the minimal auth client and app gating**

```ts
const AUTH_TOKEN_KEY = 'caichip_auth_token'

export function getAuthToken(): string {
  return localStorage.getItem(AUTH_TOKEN_KEY) ?? ''
}

export function setAuthToken(token: string): void {
  if (token.trim()) localStorage.setItem(AUTH_TOKEN_KEY, token.trim())
  else localStorage.removeItem(AUTH_TOKEN_KEY)
}
```

```ts
export async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  const token = getAuthToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const res = await fetch(url, { ...init, headers })
  if (res.status === 401) {
    setAuthToken('')
    window.dispatchEvent(new CustomEvent('auth:unauthorized'))
  }
  if (res.status === 403) {
    window.dispatchEvent(new CustomEvent('auth:forbidden'))
  }
  ...
}
```

```tsx
const allowedPagesByRole: Record<string, Page[]> = {
  anonymous: ['guide'],
  user: ['bom-list', 'result', 'hs-resolve', 'guide'],
  admin: ['bom-list', 'result', 'agent-scripts', 'agent-admin', 'hs-resolve', 'hs-meta', 'guide'],
}

useEffect(() => {
  void getMe()
    .then(setCurrentUser)
    .catch(() => setCurrentUser(null))
}, [])

const visiblePages = allowedPagesByRole[currentUser?.role ?? 'anonymous']
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- src/App.test.tsx`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/api/auth.ts web/src/auth/session.ts web/src/components/AuthPanel.tsx web/src/api/http.ts web/src/api/index.ts web/src/api/agentAdmin.ts web/src/api/agentScripts.ts web/src/api/hsMeta.ts web/src/App.tsx web/src/App.test.tsx
git commit -m "feat(web): add auth gating and shared session client"
```

### Task 6: Add Admin User Management UI And Final Verification

**Files:**
- Create: `web/src/pages/UserAdminSection.tsx`
- Create: `web/src/pages/UserAdminSection.test.tsx`
- Modify: `web/src/pages/AgentAdminPage.tsx`
- Modify: `web/src/api/types.ts`

- [ ] **Step 1: Write the failing admin UI test**

```tsx
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentAdminPage } from './AgentAdminPage'

const { adminCreateUser } = vi.hoisted(() => ({
  adminCreateUser: vi.fn(),
}))

vi.mock('../api/auth', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api/auth')
  return { ...actual, adminCreateUser }
})

describe('UserAdminSection', () => {
  it('creates an admin user from the admin page', async () => {
    adminCreateUser.mockResolvedValue({
      user_id: 12,
      username: 'root2',
      display_name: 'Root Two',
      role: 'admin',
    })
    render(<AgentAdminPage />)
    fireEvent.click(screen.getByRole('tab', { name: '账号管理' }))
    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'root2' } })
    fireEvent.change(screen.getByLabelText('显示名'), { target: { value: 'Root Two' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'Passw0rd!' } })
    fireEvent.click(screen.getByRole('radio', { name: '管理员' }))
    fireEvent.click(screen.getByRole('button', { name: '创建账号' }))
    await waitFor(() => expect(adminCreateUser).toHaveBeenCalled())
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- src/pages/UserAdminSection.test.tsx`

Expected: FAIL because the admin page has no account-management tab and the auth client has no admin-create function.

- [ ] **Step 3: Write the minimal admin account UI**

```tsx
export function UserAdminSection() {
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<'user' | 'admin'>('user')

  const submit = async () => {
    await adminCreateUser({ username, display_name: displayName, password, role })
  }

  return (
    <form onSubmit={(e) => { e.preventDefault(); void submit() }} className="space-y-4">
      <input aria-label="用户名" value={username} onChange={(e) => setUsername(e.target.value)} />
      <input aria-label="显示名" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
      <input aria-label="密码" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      <label><input type="radio" name="role" checked={role === 'user'} onChange={() => setRole('user')} />普通用户</label>
      <label><input type="radio" name="role" checked={role === 'admin'} onChange={() => setRole('admin')} />管理员</label>
      <button type="submit">创建账号</button>
    </form>
  )
}
```

```tsx
const [adminTab, setAdminTab] = useState<'bom-platforms' | 'agents' | 'users'>('bom-platforms')
...
<button role="tab" aria-selected={adminTab === 'users'} onClick={() => setAdminTab('users')}>
  账号管理
</button>
...
{adminTab === 'users' && <UserAdminSection />}
```

- [ ] **Step 4: Run full verification**

Run:

```bash
go test ./internal/biz ./internal/service ./internal/server -count=1
npm test -- src/App.test.tsx src/pages/UserAdminSection.test.tsx
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/UserAdminSection.tsx web/src/pages/UserAdminSection.test.tsx web/src/pages/AgentAdminPage.tsx web/src/api/types.ts
git commit -m "feat(admin): add account management UI"
```
