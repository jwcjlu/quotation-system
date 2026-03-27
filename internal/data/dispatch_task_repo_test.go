package data

import (
	"database/sql"
	"encoding/json"
	"testing"

	"caichip/internal/biz"
)

func TestDispatchRowToQueued(t *testing.T) {
	tags, _ := json.Marshal([]string{"r=cn"})
	d := &CaichipDispatchTask{
		TaskID:       "tid-1",
		Queue:        "default",
		ScriptID:     "demo",
		Version:      "1.0.0",
		TimeoutSec:   120,
		Attempt:      2,
		RequiredTags: tags,
		EntryFile:    sql.NullString{String: "main.py", Valid: true},
	}
	q := dispatchModelToQueued(d)
	if q.TaskID != "tid-1" || q.ScriptID != "demo" || len(q.RequiredTags) != 1 || q.RequiredTags[0] != "r=cn" {
		t.Fatalf("unexpected queued task: %+v", q)
	}
	if q.EntryFile == nil || *q.EntryFile != "main.py" {
		t.Fatalf("entry: %+v", q.EntryFile)
	}
	meta := &biz.AgentSchedulingMeta{
		Queue:   "default",
		Tags:    map[string]struct{}{"r=cn": {}},
		Scripts: map[string]biz.InstalledScript{"demo": {ScriptID: "demo", Version: "v1.0.0", EnvStatus: "ready"}},
	}
	if !biz.MatchTaskForAgent(meta, q) {
		t.Fatal("expected match")
	}
}

func TestRunningBusySets(t *testing.T) {
	a, b := runningBusySets([]biz.RunningTaskReport{{TaskID: "a", ScriptID: "s1"}})
	if len(a) != 1 || len(b) != 1 {
		t.Fatal(a, b)
	}
}
