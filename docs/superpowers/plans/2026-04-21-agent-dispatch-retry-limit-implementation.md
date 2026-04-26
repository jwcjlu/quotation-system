# Agent Dispatch Retry Limit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add capped automatic retry for generic Agent dispatch tasks so agent-reported failures and stale-lease reclaims both back off and stop at `failed_terminal`, with persisted failure reasons.

**Architecture:** Introduce a small retry-policy helper in `internal/biz`, persist per-task retry overrides on `t_caichip_dispatch_task`, and move DB task result handling from “always finish” to “success / retry / terminal-fail” branching in the dispatch repo. Keep the scheduler and service layers thin: they only parse config, accept `error_message`, and pass result input through to the repo.

**Tech Stack:** Go, Kratos, GORM, MySQL, protobuf (`make api` / `protoc`), existing Agent dispatch repo and service tests.

---

## File Map

| Path | Change | Responsibility |
| --- | --- | --- |
| `docs/schema/migrations/20260421_dispatch_task_retry_limit.sql` | Create | Add `retry_max` / `retry_backoff_json` columns for `t_caichip_dispatch_task`. |
| `internal/conf/conf.proto` | Modify | Add global retry config under `message Agent`. |
| `internal/conf/conf.pb.go` | Modify (generated) | Regenerated Go config bindings. |
| `api/agent/v1/agent.proto` | Modify | Add `error_message` to `TaskResultRequest`. |
| `api/agent/v1/agent.pb.go` | Modify (generated) | Regenerated protobuf model. |
| `api/agent/v1/agent_http.pb.go` | Modify (generated) | Regenerated Kratos HTTP bindings. |
| `api/agent/v1/agent_grpc.pb.go` | Modify (generated) | Regenerated gRPC bindings. |
| `internal/biz/agent_hub.go` | Modify | Extend `QueuedTask` / `TaskResultIn` inputs. |
| `internal/biz/dispatch_retry_policy.go` | Create | Normalize global + per-task retry settings and compute retry delays. |
| `internal/biz/dispatch_retry_policy_test.go` | Create | Unit-test retry-policy math and fallback rules. |
| `internal/biz/repo.go` | Modify | Update `DispatchTaskRepo` interface for result submission semantics. |
| `internal/data/models.go` | Modify | Persist retry fields on `CaichipDispatchTask`. |
| `internal/data/dispatch_task_repo.go` | Modify | Enqueue retry config, lease reclaim logic, task result writes. |
| `internal/data/dispatch_task_retry.go` | Create | Small repo helpers for retry transitions and failure reasons. |
| `internal/data/dispatch_task_retry_test.go` | Create | Focused retry transition tests without bloating existing test files. |
| `internal/biz/db_task_scheduler.go` | Modify | Call the new repo result API and keep lease-mismatch translation. |
| `internal/service/agent.go` | Modify | Read `error_message` from proto and pass it to the scheduler. |
| `internal/service/agent_task_result_retry_test.go` | Create | Verify `TaskResult(status=failed)` passes through failure reason and no longer implies immediate finish. |

---

### Task 1: Add Retry Policy Configuration and Pure Unit Tests

**Files:**
- Create: `internal/biz/dispatch_retry_policy.go`
- Create: `internal/biz/dispatch_retry_policy_test.go`
- Modify: `internal/conf/conf.proto`
- Modify: `internal/conf/conf.pb.go`
- Modify: `internal/biz/agent_hub.go`

- [ ] **Step 1: Write the failing retry-policy tests**

```go
func TestDispatchRetryPolicyFromBootstrap_Defaults(t *testing.T) {
	p := DispatchRetryPolicyFromBootstrap(&conf.Bootstrap{})
	if p.RetryMax != 3 {
		t.Fatalf("expected default retry max 3, got %d", p.RetryMax)
	}
	if !reflect.DeepEqual(p.BackoffSec, []int{60, 300, 900}) {
		t.Fatalf("unexpected default backoff: %+v", p.BackoffSec)
	}
}

func TestDispatchRetryPolicy_DelayForAttempt(t *testing.T) {
	p := DispatchRetryPolicy{RetryMax: 3, BackoffSec: []int{60, 300}}
	d, ok := p.DelayForFailedAttempt(1)
	if !ok || d != 60*time.Second {
		t.Fatalf("attempt 1 should retry in 60s, got %v %v", d, ok)
	}
	d, ok = p.DelayForFailedAttempt(3)
	if !ok || d != 300*time.Second {
		t.Fatalf("attempt 3 should reuse last backoff, got %v %v", d, ok)
	}
	_, ok = p.DelayForFailedAttempt(4)
	if ok {
		t.Fatal("attempt 4 should be terminal when retry_max=3")
	}
}
```

- [ ] **Step 2: Run the biz tests to verify they fail**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... -run 'DispatchRetryPolicy' -count=1
```

Expected: FAIL because `DispatchRetryPolicyFromBootstrap` and `DelayForFailedAttempt` do not exist.

- [ ] **Step 3: Implement the minimal retry-policy helper and config fields**

```go
type DispatchRetryPolicy struct {
	RetryMax   int
	BackoffSec []int
}

func DispatchRetryPolicyFromBootstrap(b *conf.Bootstrap) DispatchRetryPolicy {
	p := DispatchRetryPolicy{RetryMax: 3, BackoffSec: []int{60, 300, 900}}
	if b == nil || b.Agent == nil {
		return p
	}
	if b.Agent.DispatchRetryMax >= 0 {
		p.RetryMax = int(b.Agent.DispatchRetryMax)
	}
	if xs := sanitizeRetryBackoff(b.Agent.DispatchRetryBackoffSec); len(xs) > 0 {
		p.BackoffSec = xs
	}
	return p
}

func (p DispatchRetryPolicy) DelayForFailedAttempt(failedAttempt int) (time.Duration, bool) {
	if failedAttempt <= 0 || failedAttempt > p.RetryMax {
		return 0, false
	}
	idx := failedAttempt - 1
	if idx >= len(p.BackoffSec) {
		idx = len(p.BackoffSec) - 1
	}
	return time.Duration(p.BackoffSec[idx]) * time.Second, true
}
```

Update `message Agent` in `internal/conf/conf.proto`:

```proto
  int32 dispatch_retry_max = 10;
  repeated int32 dispatch_retry_backoff_sec = 11;
```

Extend `QueuedTask` in `internal/biz/agent_hub.go`:

```go
	RetryMax         *int   `json:"-"`
	RetryBackoffSec  []int  `json:"-"`
```

- [ ] **Step 4: Regenerate config bindings and rerun the tests**

Run:

```powershell
protoc --proto_path=./internal/conf --go_out=paths=source_relative:./internal/conf ./internal/conf/conf.proto
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... -run 'DispatchRetryPolicy' -count=1
```

Expected: PASS for the new retry-policy tests.

- [ ] **Step 5: Commit**

```powershell
git add -- 'internal/conf/conf.proto' 'internal/conf/conf.pb.go' 'internal/biz/agent_hub.go' 'internal/biz/dispatch_retry_policy.go' 'internal/biz/dispatch_retry_policy_test.go'
git commit -m "feat(agent): add dispatch retry policy config"
```

---

### Task 2: Extend Agent TaskResult Protocol and Service Input Plumbing

**Files:**
- Modify: `api/agent/v1/agent.proto`
- Modify: `api/agent/v1/agent.pb.go`
- Modify: `api/agent/v1/agent_http.pb.go`
- Modify: `api/agent/v1/agent_grpc.pb.go`
- Modify: `internal/biz/agent_hub.go`
- Modify: `internal/service/agent.go`
- Create: `internal/service/agent_task_result_retry_test.go`

- [ ] **Step 1: Write the failing service test for `error_message` passthrough**

```go
func TestAgentService_TaskResultPassesErrorMessage(t *testing.T) {
	sched := &stubTaskScheduler{}
	svc := NewAgentService(nil, sched, nil, nil, nil, nil, nil, testAgentConf(), log.DefaultLogger)

	_, err := svc.TaskResult(context.Background(), &v1.TaskResultRequest{
		AgentId:      "agent-1",
		TaskId:       "task-1",
		LeaseId:      "lease-1",
		Status:       "failed",
		ErrorMessage: "site login failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sched.lastResult == nil || sched.lastResult.ErrorMessage != "site login failed" {
		t.Fatalf("unexpected result payload: %+v", sched.lastResult)
	}
}
```

- [ ] **Step 2: Run the targeted service test to verify it fails**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service/... -run 'TaskResultPassesErrorMessage' -count=1
```

Expected: FAIL because `error_message` is not on the proto or `TaskResultIn`.

- [ ] **Step 3: Add the proto field and wire it through the service**

In `api/agent/v1/agent.proto`:

```proto
  string error_message = 16;
```

In `internal/biz/agent_hub.go`:

```go
	ErrorMessage string `json:"error_message,omitempty"`
```

In `internal/service/agent.go`:

```go
	in := &biz.TaskResultIn{
		TaskID:       req.GetTaskId(),
		AgentID:      req.GetAgentId(),
		LeaseID:      req.GetLeaseId(),
		Status:       req.GetStatus(),
		Attempt:      attempt,
		Stdout:       req.GetStdout(),
		ErrorMessage: strings.TrimSpace(req.GetErrorMessage()),
	}
```

- [ ] **Step 4: Regenerate API bindings and rerun the service test**

Run:

```powershell
make api
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service/... -run 'TaskResultPassesErrorMessage' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add -- 'api/agent/v1/agent.proto' 'api/agent/v1/agent.pb.go' 'api/agent/v1/agent_http.pb.go' 'api/agent/v1/agent_grpc.pb.go' 'internal/biz/agent_hub.go' 'internal/service/agent.go' 'internal/service/agent_task_result_retry_test.go'
git commit -m "feat(agent): plumb task result error message"
```

---

### Task 3: Add Failing Repo Tests for Retry and Terminal Failure Transitions

**Files:**
- Create: `internal/data/dispatch_task_retry_test.go`
- Modify: `internal/biz/repo.go`
- Modify: `internal/biz/db_task_scheduler.go`

- [ ] **Step 1: Write the failing data tests for failed-result retry behavior**

```go
func TestDispatchTaskRepo_SubmitLeasedResultFailedSchedulesRetry(t *testing.T) {
	repo, db := newDispatchTaskRepoForTest(t)
	taskID, leaseID := seedLeasedDispatchTask(t, db, CaichipDispatchTask{
		TaskID:             "task-retry",
		State:              "leased",
		Attempt:            1,
		RetryMax:           3,
		RetryBackoffJSON:   mustJSON([]int{60, 300, 900}),
	})

	err := repo.SubmitLeasedResult(context.Background(), &biz.TaskResultIn{
		TaskID: taskID, LeaseID: leaseID, Status: "failed", ErrorMessage: "proxy rejected",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := loadDispatchTask(t, db, taskID)
	if got.State != "pending" || got.Attempt != 2 || got.LastError.String != "proxy rejected" {
		t.Fatalf("unexpected task after retry scheduling: %+v", got)
	}
	if got.NextClaimAt == nil {
		t.Fatal("expected next_claim_at to be set")
	}
}

func TestDispatchTaskRepo_SubmitLeasedResultFailedExhaustsToTerminal(t *testing.T) {
	repo, db := newDispatchTaskRepoForTest(t)
	taskID, leaseID := seedLeasedDispatchTask(t, db, CaichipDispatchTask{
		TaskID:           "task-terminal",
		State:            "leased",
		Attempt:          4,
		RetryMax:         3,
		RetryBackoffJSON: mustJSON([]int{60, 300, 900}),
	})

	err := repo.SubmitLeasedResult(context.Background(), &biz.TaskResultIn{
		TaskID: taskID, LeaseID: leaseID, Status: "failed", ErrorMessage: "captcha loop",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := loadDispatchTask(t, db, taskID)
	if got.State != "failed_terminal" || got.ResultStatus.String != "failed_terminal" {
		t.Fatalf("unexpected terminal task: %+v", got)
	}
}
```

- [ ] **Step 2: Run the data tests to verify they fail**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data/... -run 'DispatchTaskRepo_(SubmitLeasedResult|ReclaimStaleLeases)' -count=1
```

Expected: FAIL because `SubmitLeasedResult` does not exist and the repo still treats failures as finished.

- [ ] **Step 3: Change the repo interface and scheduler call site before implementation**

In `internal/biz/repo.go`:

```go
	SubmitLeasedResult(ctx context.Context, in *TaskResultIn) error
```

Replace the old method in `internal/biz/db_task_scheduler.go`:

```go
	err := s.dispatch.SubmitLeasedResult(ctx, in)
	if errors.Is(err, ErrDispatchLeaseMismatch) {
		return ErrLeaseReassigned
	}
```

- [ ] **Step 4: Rerun the targeted tests to keep them red for the right reason**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data/... -run 'DispatchTaskRepo_(SubmitLeasedResult|ReclaimStaleLeases)' -count=1
```

Expected: FAIL inside repo implementation, not from missing interface symbols in unrelated packages.

- [ ] **Step 5: Commit**

```powershell
git add -- 'internal/biz/repo.go' 'internal/biz/db_task_scheduler.go' 'internal/data/dispatch_task_retry_test.go'
git commit -m "test(agent): cover dispatch retry and terminal failure transitions"
```

---

### Task 4: Implement Persisted Retry State, Repo Transitions, and Lease Reclaim Budgeting

**Files:**
- Create: `docs/schema/migrations/20260421_dispatch_task_retry_limit.sql`
- Modify: `internal/data/models.go`
- Modify: `internal/data/dispatch_task_repo.go`
- Create: `internal/data/dispatch_task_retry.go`

- [ ] **Step 1: Add the schema and model fields**

Migration:

```sql
ALTER TABLE t_caichip_dispatch_task
  ADD COLUMN retry_max INT NOT NULL DEFAULT 3 AFTER attempt,
  ADD COLUMN retry_backoff_json JSON NULL AFTER retry_max;
```

Model fields in `internal/data/models.go`:

```go
	RetryMax         int            `gorm:"column:retry_max;not null;default:3"`
	RetryBackoffJSON []byte         `gorm:"column:retry_backoff_json;type:json"`
```

- [ ] **Step 2: Persist retry overrides on enqueue**

In `caichipDispatchRowFromQueuedTask`:

```go
	policy := biz.DispatchRetryPolicyFromBootstrap(nil)
	if t.RetryMax != nil || len(t.RetryBackoffSec) > 0 {
		policy = biz.DispatchRetryPolicy{
			RetryMax:   valueOrDefault(t.RetryMax, policy.RetryMax),
			BackoffSec: normalizeBackoff(t.RetryBackoffSec, policy.BackoffSec),
		}
	}
	row.RetryMax = policy.RetryMax
	row.RetryBackoffJSON = mustJSONBytes(policy.BackoffSec)
```

- [ ] **Step 3: Implement a small helper for failure transitions**

In `internal/data/dispatch_task_retry.go`:

```go
func dispatchFailureReason(in *biz.TaskResultIn) string {
	if strings.TrimSpace(in.ErrorMessage) != "" {
		return strings.TrimSpace(in.ErrorMessage)
	}
	if s := summarizeStdout(strings.TrimSpace(in.Stdout)); s != "" {
		return s
	}
	return "task failed"
}

func (r *DispatchTaskRepo) transitionFailure(tx *gorm.DB, row *CaichipDispatchTask, reason string, now time.Time) error {
	policy := policyFromRowOrDefault(r.retryPolicy, row)
	delay, ok := policy.DelayForFailedAttempt(row.Attempt)
	if ok {
		return tx.Model(&CaichipDispatchTask{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
			"state":              dispatchStatePending,
			"attempt":            row.Attempt + 1,
			"next_claim_at":      now.Add(delay),
			"last_error":         reason,
			"lease_id":           gorm.Expr("NULL"),
			"leased_to_agent_id": gorm.Expr("NULL"),
			"leased_at":          gorm.Expr("NULL"),
			"lease_deadline_at":  gorm.Expr("NULL"),
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}).Error
	}
	return tx.Model(&CaichipDispatchTask{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
		"state":              "failed_terminal",
		"result_status":      "failed_terminal",
		"finished_at":        now,
		"last_error":         reason,
		"lease_id":           gorm.Expr("NULL"),
		"leased_to_agent_id": gorm.Expr("NULL"),
		"leased_at":          gorm.Expr("NULL"),
		"lease_deadline_at":  gorm.Expr("NULL"),
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}).Error
}
```

- [ ] **Step 4: Replace `FinishLeased` and update stale-lease reclaim to use the helper**

Core branching:

```go
func (r *DispatchTaskRepo) SubmitLeasedResult(ctx context.Context, in *biz.TaskResultIn) error {
	if strings.EqualFold(strings.TrimSpace(in.Status), "success") {
		return r.finishLeasedSuccess(ctx, in.TaskID, in.LeaseID, in.Status)
	}
	return r.failLeasedWithRetry(ctx, in)
}
```

And in `ReclaimStaleLeases`, load candidate rows, then for each leased row call:

```go
reason := "LEASE_EXPIRED"
if holderOffline {
	reason = "AGENT_OFFLINE_RECLAIMED"
}
if err := r.transitionFailure(tx, &row, reason, now); err != nil {
	return err
}
```

- [ ] **Step 5: Run data tests and the targeted scheduler tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data/... -run 'DispatchTaskRepo_(SubmitLeasedResult|ReclaimStaleLeases)' -count=1
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... -run 'DispatchRetryPolicy' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add -- 'docs/schema/migrations/20260421_dispatch_task_retry_limit.sql' 'internal/data/models.go' 'internal/data/dispatch_task_repo.go' 'internal/data/dispatch_task_retry.go'
git commit -m "feat(agent): add dispatch retry limit and terminal failure state"
```

---

### Task 5: Run End-to-End Verification for Service, Repo, and Build

**Files:**
- Modify: `internal/service/agent_test.go` or keep focused tests in `internal/service/agent_task_result_retry_test.go`
- Verify only: generated protobuf files and previous task outputs

- [ ] **Step 1: Add one end-to-end service-level retry assertion if missing**

```go
func TestDBTaskScheduler_SubmitTaskResultMapsLeaseMismatch(t *testing.T) {
	dispatch := &stubDispatchRepo{err: biz.ErrDispatchLeaseMismatch}
	s := newDBTaskScheduler(nil, dispatch, nil, testAgentConf())
	err := s.SubmitTaskResult(&TaskResultIn{TaskID: "t1", LeaseID: "l1", Status: "failed"})
	if err != ErrLeaseReassigned {
		t.Fatalf("expected ErrLeaseReassigned, got %v", err)
	}
}
```

- [ ] **Step 2: Run the focused verification suite**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... -run 'DispatchRetryPolicy|SubmitTaskResult' -count=1
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data/... -run 'DispatchTaskRepo_(SubmitLeasedResult|ReclaimStaleLeases)' -count=1
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service/... -run 'TaskResultPassesErrorMessage|TaskHeartbeatPull' -count=1
& 'C:\Program Files\Go\bin\go.exe' build ./cmd/server/...
```

Expected: PASS for all targeted tests and a successful server build.

- [ ] **Step 3: Run broader regression tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1
```

Expected: PASS, or document any pre-existing unrelated failures before claiming completion.

- [ ] **Step 4: Commit**

```powershell
git add -- 'internal/service/agent_task_result_retry_test.go' 'internal/service/agent.go' 'internal/biz/db_task_scheduler.go' 'internal/biz/repo.go'
git commit -m "test(agent): verify dispatch retry result flow"
```

---

## Self-Review

### Spec coverage

- Retry cap, retry backoff, and per-task overrides: Task 1 and Task 4
- `failed_terminal` state: Task 4
- `error_message` propagation and `last_error` precedence: Task 2 and Task 4
- Lease reclaim sharing retry budget: Task 4
- Verification and regression coverage: Task 5

### Placeholder scan

- No `TODO` / `TBD`
- Each task includes exact file paths, code snippets, and commands
- No “same as above” references without concrete content

### Type consistency

- `QueuedTask.RetryMax` / `RetryBackoffSec` introduced in Task 1 and used consistently later
- `TaskResultIn.ErrorMessage` introduced in Task 2 and consumed in Task 4
- Repo API renamed to `SubmitLeasedResult` in Task 3 and used consistently afterward
