package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestMatchReadinessError_BlocksWhenImportParsing(t *testing.T) {
	session := &bomSessionRepoStub{sessionExists: true}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	err := svc.matchReadinessError(context.Background(), "sid-guard", &biz.BOMSessionView{
		SessionID:     "sid-guard",
		ImportStatus:  biz.BOMImportStatusParsing,
		PlatformIDs:   []string{"digikey"},
		ReadinessMode: biz.ReadinessLenient,
	}, nil)
	if err == nil {
		t.Fatalf("expected BOM_NOT_READY when import is parsing")
	}
	se := kerrors.FromError(err)
	if se == nil || se.Code != 503 || se.Reason != "BOM_NOT_READY" {
		t.Fatalf("unexpected error: %+v", se)
	}
}

func TestRunLLMImportJob_ChunkParsingCallsChatMultipleTimes(t *testing.T) {
	session := &bomSessionRepoStub{
		sessionExists: true,
		view: &biz.BOMSessionView{
			SessionID:         "sid-chunk",
			BizDate:           time.Now(),
			PlatformIDs:       []string{"digikey"},
			SelectionRevision: 1,
		},
	}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, &data.OpenAIChat{}, nil, nil, nil, nil, nil, nil, nil, nil)

	var callCount int32
	svc.llmChatFn = func(ctx context.Context, system, user string) (string, error) {
		atomic.AddInt32(&callCount, 1)
		return `{"items":[{"line_no":2,"model":"LM358","manufacturer":"","package":"","params":"","quantity":1,"raw_text":""}]}`, nil
	}

	svc.runLLMImportJob(context.Background(), "sid-chunk", makeBOMExcelWithRows(t, 301), "llm")

	if atomic.LoadInt32(&callCount) < 2 {
		t.Fatalf("expected chunked llm calls >=2, got %d", callCount)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.patches) == 0 || session.patches[len(session.patches)-1].Status != biz.BOMImportStatusReady {
		t.Fatalf("expected final ready status, patches=%+v", session.patches)
	}
	if len(session.replacedLineNos) < 2 {
		t.Fatalf("expected >1 chunk parsed lines, got lineNos=%v", session.replacedLineNos)
	}
	seen := make(map[int]struct{}, len(session.replacedLineNos))
	prev := -1
	for _, ln := range session.replacedLineNos {
		if _, ok := seen[ln]; ok {
			t.Fatalf("line_no duplicated after chunk normalization: %v", session.replacedLineNos)
		}
		seen[ln] = struct{}{}
		if prev >= 0 && ln <= prev {
			t.Fatalf("line_no not strictly increasing: %v", session.replacedLineNos)
		}
		prev = ln
	}
}

func TestRunLLMImportJob_ChunkFailAfterRetriesSetsFailed(t *testing.T) {
	session := &bomSessionRepoStub{
		sessionExists: true,
		view: &biz.BOMSessionView{
			SessionID:         "sid-fail",
			BizDate:           time.Now(),
			PlatformIDs:       []string{"digikey"},
			SelectionRevision: 1,
		},
	}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, &data.OpenAIChat{}, nil, nil, nil, nil, nil, nil, nil, nil)

	var callCount int32
	svc.llmChatFn = func(ctx context.Context, system, user string) (string, error) {
		atomic.AddInt32(&callCount, 1)
		return "", fmt.Errorf("mock llm failure")
	}

	svc.runLLMImportJob(context.Background(), "sid-fail", makeBOMExcelWithRows(t, 1), "llm")

	if atomic.LoadInt32(&callCount) != int32(llmChunkRetryTimes+1) {
		t.Fatalf("expected retry attempts %d, got %d", llmChunkRetryTimes+1, callCount)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.patches) == 0 {
		t.Fatalf("expected failed status patch")
	}
	last := session.patches[len(session.patches)-1]
	if last.Status != biz.BOMImportStatusFailed {
		t.Fatalf("expected failed status, got %+v", last)
	}
	if last.ErrorCode == nil || *last.ErrorCode != "BOM_LLM_CHUNK_FAILED" {
		t.Fatalf("expected BOM_LLM_CHUNK_FAILED, got %+v", last.ErrorCode)
	}
}
