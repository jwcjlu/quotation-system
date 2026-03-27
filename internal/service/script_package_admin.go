package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"caichip/internal/conf"
	"caichip/internal/data"

	"github.com/go-kratos/kratos/v2/log"
)

// ScriptPackageAdmin 脚本包管理端上传/发布（鉴权独立于 Agent api_keys）。
type ScriptPackageAdmin struct {
	bc   *conf.Bootstrap
	repo *data.AgentScriptPackageRepo
	log  *log.Helper
}

// NewScriptPackageAdmin ...
func NewScriptPackageAdmin(bc *conf.Bootstrap, repo *data.AgentScriptPackageRepo, logger log.Logger) *ScriptPackageAdmin {
	return &ScriptPackageAdmin{bc: bc, repo: repo, log: log.NewHelper(logger)}
}

// Enabled 是否配置了管理端密钥（用于注册路由）。
func (a *ScriptPackageAdmin) Enabled() bool {
	if a == nil || a.bc == nil || a.bc.ScriptAdmin == nil {
		return false
	}
	for _, k := range a.bc.ScriptAdmin.ApiKeys {
		if strings.TrimSpace(k) != "" {
			return true
		}
	}
	return false
}

// ValidateAdminKey 校验管理端 API Key。
func (a *ScriptPackageAdmin) ValidateAdminKey(authBearer, xAPIKey string) bool {
	if a == nil || a.bc == nil || a.bc.ScriptAdmin == nil {
		return false
	}
	keys := make(map[string]struct{})
	for _, k := range a.bc.ScriptAdmin.ApiKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			keys[k] = struct{}{}
		}
	}
	if len(keys) == 0 {
		return false
	}
	if x := strings.TrimSpace(xAPIKey); x != "" {
		_, ok := keys[x]
		return ok
	}
	const p = "Bearer "
	if strings.HasPrefix(authBearer, p) {
		token := strings.TrimSpace(authBearer[len(p):])
		_, ok := keys[token]
		return ok
	}
	return false
}

func (a *ScriptPackageAdmin) scriptStore() *conf.ScriptStore {
	if a.bc == nil {
		return nil
	}
	return a.bc.ScriptStore
}

// MaxUploadBytes multipart 上限。
func (a *ScriptPackageAdmin) MaxUploadBytes() int64 {
	st := a.scriptStore()
	if st == nil || st.MaxUploadMb <= 0 {
		return 64 << 20
	}
	return int64(st.MaxUploadMb) << 20
}

// StoreRoot 落盘根目录。
func (a *ScriptPackageAdmin) StoreRoot() (string, error) {
	st := a.scriptStore()
	if st == nil || strings.TrimSpace(st.Root) == "" {
		return "", errors.New("script_store.root not configured")
	}
	return filepath.Clean(st.Root), nil
}

func safePathSeg(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, string(os.PathSeparator), "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "..", "_")
	return s
}

// UploadPackage multipart：file + script_id + version；可选 release_notes、package_sha256（分发觉以 script_id 为维，旧 platform_id 已废弃）。
func (a *ScriptPackageAdmin) UploadPackage(ctx context.Context, r *http.Request) (id int64, downloadPath string, shaOut string, err error) {
	if a.repo == nil {
		return 0, "", "", data.ErrScriptStoreUnavailable
	}
	root, err := a.StoreRoot()
	if err != nil {
		return 0, "", "", err
	}
	if err := r.ParseMultipartForm(a.MaxUploadBytes()); err != nil {
		return 0, "", "", fmt.Errorf("parse multipart: %w", err)
	}
	scriptID := safePathSeg(r.FormValue("script_id"))
	version := safePathSeg(r.FormValue("version"))
	if scriptID == "" || version == "" {
		return 0, "", "", errors.New("script_id, version required")
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return 0, "", "", fmt.Errorf("file required: %w", err)
	}
	defer func() { _ = file.Close() }()

	filename := strings.TrimSpace(hdr.Filename)
	if filename == "" {
		filename = "package.zip"
	}
	filename = filepath.Base(filename)

	wantSHA := strings.ToLower(strings.TrimSpace(r.FormValue("package_sha256")))
	body, err := io.ReadAll(io.LimitReader(file, a.MaxUploadBytes()+1))
	if err != nil {
		return 0, "", "", err
	}
	if int64(len(body)) > a.MaxUploadBytes() {
		return 0, "", "", errors.New("file too large")
	}
	sum := sha256.Sum256(body)
	gotSHA := hex.EncodeToString(sum[:])
	if wantSHA != "" && gotSHA != wantSHA {
		return 0, "", "", fmt.Errorf("sha256 mismatch: form claims %s, file is %s", wantSHA, gotSHA)
	}

	relDir := filepath.Join(scriptID, version)
	absDir := filepath.Join(root, relDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return 0, "", "", err
	}
	absFile := filepath.Join(absDir, filename)
	if err := os.WriteFile(absFile, body, 0644); err != nil {
		return 0, "", "", err
	}
	storageRel := filepath.ToSlash(filepath.Join(relDir, filename))

	entryFile := strings.TrimSpace(r.FormValue("entry_file"))
	if entryFile == "" {
		entryFile = fmt.Sprintf("%s_crawler.py", scriptID)
	} else {
		entryFile = filepath.Base(entryFile)
	}

	notes := strings.TrimSpace(r.FormValue("release_notes"))
	rec := &data.AgentScriptPackage{
		ScriptID:       scriptID,
		Version:        version,
		SHA256:         gotSHA,
		StorageRelPath: storageRel,
		Filename:       filename,
		EntryFile:      entryFile,
		Status:         "uploaded",
		ReleaseNotes:   notes,
	}
	id, err = a.repo.Insert(ctx, rec)
	if err != nil {
		_ = os.Remove(absFile)
		return 0, "", "", err
	}
	return id, storageRel, gotSHA, nil
}

// PublishPackage 将行置为当前发布。
func (a *ScriptPackageAdmin) PublishPackage(ctx context.Context, id int64) error {
	if a.repo == nil {
		return data.ErrScriptStoreUnavailable
	}
	return a.repo.SetPublished(ctx, id)
}

// GetCurrentPublished 查询当前发布元数据（按 script_id）。
func (a *ScriptPackageAdmin) GetCurrentPublished(ctx context.Context, scriptID string) (*data.AgentScriptPackage, error) {
	if a.repo == nil {
		return nil, data.ErrScriptStoreUnavailable
	}
	return a.repo.GetPublished(ctx, scriptID)
}

// ListPackages 分页审计列表。
func (a *ScriptPackageAdmin) ListPackages(ctx context.Context, offset, limit int) ([]*data.AgentScriptPackage, error) {
	if a.repo == nil {
		return nil, data.ErrScriptStoreUnavailable
	}
	return a.repo.ListPackages(ctx, offset, limit)
}

// GetPackage 按 id。
func (a *ScriptPackageAdmin) GetPackage(ctx context.Context, id int64) (*data.AgentScriptPackage, error) {
	if a.repo == nil {
		return nil, data.ErrScriptStoreUnavailable
	}
	return a.repo.GetByID(ctx, id)
}

// URLPrefixForPublicPath 静态 URL 路径前缀（含前导 /）。
func (a *ScriptPackageAdmin) URLPrefixForPublicPath() string {
	if a == nil || a.bc == nil || a.bc.ScriptStore == nil {
		return "/static/agent-scripts"
	}
	p := strings.TrimSpace(a.bc.ScriptStore.UrlPrefix)
	if p == "" {
		return "/static/agent-scripts"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
