package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"gorm.io/gorm"
)

const (
	llmChunkSize       = 300
	llmChunkRetryTimes = 2
)

func (s *BomService) UploadBOM(ctx context.Context, req *v1.UploadBOMRequest) (*v1.UploadBOMReply, error) {
	sid := strings.TrimSpace(req.GetSessionId())
	if sid == "" {
		return nil, notImplemented("请使用 session_id 将 Excel 导入到 bom_session_line")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}

	parseModeRaw := strings.TrimSpace(req.GetParseMode())
	pmLower := strings.ToLower(parseModeRaw)
	started, err := s.session.TryStartImport(ctx, sid, "import started")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		return nil, err
	}
	if !started {
		return nil, kerrors.Conflict("BOM_IMPORT_IN_PROGRESS", "import already in progress for this session")
	}

	if pmLower == "llm" {
		if s.openai == nil {
			err := kerrors.BadRequest("BOM_LLM_DISABLED", "parse_mode=llm 需要在配置中设置 openai.api_key")
			s.markImportFailed(ctx, sid, "BOM_LLM_DISABLED", err.Error())
			return nil, err
		}
		file := append([]byte(nil), req.GetFile()...)
		go s.runLLMImportJob(context.WithoutCancel(ctx), sid, file, parseModeRaw)
		return &v1.UploadBOMReply{
			BomId:         sid,
			Accepted:      true,
			ImportStatus:  biz.BOMImportStatusParsing,
			ImportMessage: "import started",
		}, nil
	}

	lines, ierrs := s.parseSyncImportLines(req.GetFile(), req.GetColumnMapping())
	if len(ierrs) > 0 {
		err := kerrors.BadRequest("BOM_IMPORT_PARSE", ierrs[0].Error())
		s.markImportFailed(ctx, sid, "BOM_IMPORT_PARSE", ierrs[0].Error())
		return nil, err
	}
	if code, err := s.finishImportedLines(ctx, sid, lines, parseModeRaw); err != nil {
		s.markImportFailed(ctx, sid, code, err.Error())
		return nil, err
	}

	items := make([]*v1.ParsedItem, 0, len(lines))
	for i, ln := range lines {
		var q int32
		if ln.Qty != nil {
			q = int32(*ln.Qty)
		}
		items = append(items, &v1.ParsedItem{
			Index:               int32(i + 1),
			Model:               ln.Mpn,
			UnifiedModel:        ln.UnifiedMpn,
			ReferenceDesignator: ln.ReferenceDesignator,
			SubstituteModel:     ln.SubstituteMpn,
			Remark:              ln.Remark,
			Description:         ln.Description,
			Manufacturer:        ln.Mfr,
			Package:             ln.Package,
			Quantity:            q,
		})
	}
	return &v1.UploadBOMReply{
		BomId:         sid,
		Items:         items,
		Total:         int32(len(items)),
		Accepted:      true,
		ImportStatus:  biz.BOMImportStatusReady,
		ImportMessage: "import completed",
	}, nil
}

func (s *BomService) parseSyncImportLines(file []byte, columnMapping map[string]string) ([]biz.BomImportLine, []biz.BomImportError) {
	return biz.ParseBomImportRowsWithColumnMapping(bytes.NewReader(file), false, columnMapping)
}

func (s *BomService) runLLMImportJob(ctx context.Context, sid string, file []byte, parseModeRaw string) {
	if !s.setParsingStage(ctx, sid, 15, biz.BOMImportStageValidating, "validating input") {
		return
	}

	rows, ferrs := biz.ReadBomImportFirstSheetFromReader(bytes.NewReader(file))
	if len(ferrs) > 0 {
		s.markImportFailed(ctx, sid, "BOM_IMPORT_PARSE", ferrs[0].Error())
		return
	}
	if len(rows) > biz.MaxBomLLMSheetRows {
		msg := fmt.Sprintf("工作表行数超过 llm 模式上限 %d，请拆分文件", biz.MaxBomLLMSheetRows)
		s.markImportFailed(ctx, sid, "BOM_LLM_LIMIT", msg)
		return
	}
	if len(rows) < 2 {
		s.markImportFailed(ctx, sid, "BOM_IMPORT_PARSE", "empty data rows")
		return
	}
	if !s.setParsingStage(ctx, sid, 25, biz.BOMImportStageHeaderInfer, "preparing header inference") {
		return
	}

	chunks := splitRowsWithHeader(rows, llmChunkSize)
	allLines := make([]biz.BomImportLine, 0, len(rows)-1)
	chunkTotal := len(chunks)
	chunkStartDataIdx := 0
	for idx, chunkRows := range chunks {
		progress := 25 + (60*(idx+1))/chunkTotal
		message := fmt.Sprintf("chunk %d/%d", idx+1, chunkTotal)
		if !s.setParsingStage(ctx, sid, progress, biz.BOMImportStageChunkParsing, message) {
			return
		}

		lines, err := s.parseLLMChunk(ctx, chunkRows, idx+1)
		if err != nil {
			s.markImportFailed(ctx, sid, "BOM_LLM_CHUNK_FAILED", err.Error())
			return
		}
		lines = normalizeChunkLineNos(lines, chunkStartDataIdx, len(chunkRows)-1)
		allLines = append(allLines, lines...)
		chunkStartDataIdx += len(chunkRows) - 1
	}
	if len(allLines) == 0 {
		s.markImportFailed(ctx, sid, "BOM_IMPORT_PARSE", "no valid rows after chunk parsing")
		return
	}
	if !s.setParsingStage(ctx, sid, 90, biz.BOMImportStagePersisting, "persisting parsed lines") {
		return
	}
	if code, err := s.finishImportedLines(ctx, sid, allLines, parseModeRaw); err != nil {
		s.markImportFailed(ctx, sid, code, err.Error())
	}
}

func (s *BomService) parseLLMChunk(ctx context.Context, chunkRows [][]string, chunkIndex int) ([]biz.BomImportLine, error) {
	user := biz.BuildBomLLMUserPrompt(chunkRows)
	if len(user) > biz.MaxBomLLMPromptBytes {
		return nil, fmt.Errorf("chunk %d prompt too large", chunkIndex)
	}

	var lastParseErr error
	for attempt := 0; attempt <= llmChunkRetryTimes; attempt++ {
		raw, err := s.callLLMChat(ctx, biz.BomLLMSystemPrompt(), user)
		if err != nil {
			if attempt == llmChunkRetryTimes {
				return nil, fmt.Errorf("chunk %d chat failed: %w", chunkIndex, err)
			}
			continue
		}
		lines, ierrs := biz.ParseBomImportLinesFromLLMJSON(raw)
		if len(ierrs) == 0 {
			return lines, nil
		}
		lastParseErr = ierrs[0]
	}
	return nil, fmt.Errorf("chunk %d parse failed: %w", chunkIndex, lastParseErr)
}

func (s *BomService) finishImportedLines(ctx context.Context, sid string, lines []biz.BomImportLine, parseModeRaw string) (string, error) {
	var pmPtr *string
	if parseModeRaw != "" {
		pmPtr = &parseModeRaw
	}
	cleanedLines, err := s.canonicalizeBomImportLines(ctx, lines)
	if err != nil {
		return "BOM_IMPORT_MFR_CANONICALIZE_FAILED", err
	}
	lines = cleanedLines
	if _, err := s.session.ReplaceSessionLines(ctx, sid, lines, pmPtr); err != nil {
		return "BOM_IMPORT_PERSIST", err
	}
	if err := s.search.CancelAllTasksBySession(ctx, sid); err != nil {
		return "BOM_IMPORT_TASK_RESET", err
	}
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return "BOM_IMPORT_SESSION_LOAD", err
	}
	pairs := buildMpnPlatformPairs(lines, view.PlatformIDs)
	if err := s.search.UpsertPendingTasks(ctx, sid, view.BizDate, view.SelectionRevision, pairs); err != nil {
		return "BOM_IMPORT_TASK_UPSERT", err
	}

	completedMsg := "import completed"
	if err := s.session.UpdateImportState(ctx, sid, biz.BOMImportStatePatch{
		Status:   biz.BOMImportStatusReady,
		Progress: 100,
		Stage:    biz.BOMImportStageDone,
		Message:  &completedMsg,
	}); err != nil {
		return "BOM_IMPORT_STATUS_UPDATE", err
	}
	s.tryMergeDispatchSession(ctx, sid)
	return "", nil
}

func (s *BomService) setParsingStage(ctx context.Context, sid string, progress int, stage, message string) bool {
	msg := strings.TrimSpace(message)
	return s.session.UpdateImportState(ctx, sid, biz.BOMImportStatePatch{
		Status:   biz.BOMImportStatusParsing,
		Progress: progress,
		Stage:    stage,
		Message:  &msg,
	}) == nil
}

func (s *BomService) markImportFailed(ctx context.Context, sid, code, detail string) {
	code = strings.TrimSpace(code)
	detail = strings.TrimSpace(detail)
	if code == "" {
		code = "BOM_IMPORT_FAILED"
	}
	if detail == "" {
		detail = "import failed"
	}

	msg := "import failed"
	_ = s.session.UpdateImportState(ctx, sid, biz.BOMImportStatePatch{
		Status:    biz.BOMImportStatusFailed,
		Progress:  0,
		Stage:     biz.BOMImportStageFailed,
		Message:   &msg,
		ErrorCode: &code,
		Error:     &detail,
	})
}

func (s *BomService) callLLMChat(ctx context.Context, system, user string) (string, error) {
	if s.llmChatFn != nil {
		return s.llmChatFn(ctx, system, user)
	}
	return s.openai.Chat(ctx, system, user)
}
