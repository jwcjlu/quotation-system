package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ownerSubjectFromContext 预留：与统一身份中间件对接后从 ctx 解析 subject。
func ownerSubjectFromContext(_ context.Context) string {
	return ""
}

func randomUploadIDHex() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// UploadHsManualDatasheet 接收 PDF 字节并写入 staging 表（gRPC/程序内调用；HTTP multipart 见 server 路由）。
func (s *HsResolveService) UploadHsManualDatasheet(ctx context.Context, req *v1.UploadHsManualDatasheetRequest) (*v1.UploadHsManualDatasheetReply, error) {
	if s == nil {
		return nil, kerrors.InternalServer("HS_RESOLVE_INTERNAL", "nil service")
	}
	if s.manualUpload == nil || !s.manualUpload.DBOk() {
		return nil, kerrors.ServiceUnavailable("HS_RESOLVE_DISABLED", "manual datasheet upload is not configured")
	}
	if strings.TrimSpace(s.manualStagingDir) == "" {
		return nil, kerrors.ServiceUnavailable("HS_RESOLVE_DISABLED", "manual staging dir not configured")
	}
	body := req.GetFile()
	maxB := s.hsResolveCfg.ManualUploadMaxBytesOrDefault()
	if len(body) == 0 {
		return nil, kerrors.BadRequest("HS_RESOLVE_BAD_REQUEST", "empty file")
	}
	if len(body) > maxB {
		return nil, kerrors.BadRequest("HS_RESOLVE_BAD_REQUEST", fmt.Sprintf("file too large (max %d bytes)", maxB))
	}
	if len(body) < 5 || string(body[:5]) != "%PDF-" {
		return nil, kerrors.BadRequest("HS_RESOLVE_BAD_REQUEST", "file is not a PDF (missing %PDF- header)")
	}
	uploadID, err := randomUploadIDHex()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.manualStagingDir, 0o755); err != nil {
		return nil, err
	}
	localPath := filepath.Join(s.manualStagingDir, uploadID+".pdf")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	ttl := time.Duration(s.hsResolveCfg.ManualUploadTTLSecondsOrDefault()) * time.Second
	exp := time.Now().Add(ttl).UTC()
	owner := ownerSubjectFromContext(ctx)
	row := &biz.HsManualDatasheetUploadRecord{
		UploadID:     uploadID,
		LocalPath:    localPath,
		SHA256:       sha,
		ExpiresAt:    exp,
		OwnerSubject: owner,
	}
	if err := s.manualUpload.Create(ctx, row); err != nil {
		_ = os.Remove(localPath)
		return nil, err
	}
	return &v1.UploadHsManualDatasheetReply{
		UploadId:      uploadID,
		ExpiresAtUnix: exp.Unix(),
		ContentSha256: sha,
	}, nil
}
