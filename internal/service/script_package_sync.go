package service

import (
	"path"
	"strings"

	v1 "caichip/api/agent/v1"
	"caichip/internal/biz"
	"caichip/internal/pkg/versionutil"
)

func needsScriptSync(pub *biz.PublishedScriptMeta, row *v1.ScriptRow) bool {
	if pub == nil {
		return false
	}
	if row == nil {
		return true
	}
	if !versionutil.Equal(pub.Version, row.GetVersion()) {
		return true
	}
	rowSha := strings.TrimSpace(row.GetPackageSha256())
	if rowSha == "" {
		return false
	}
	return !strings.EqualFold(rowSha, pub.SHA256)
}

func buildDownloadURL(publicBaseURL, urlPrefix, storageRelPath string) string {
	pref := strings.TrimSpace(urlPrefix)
	if pref == "" {
		pref = "/static/agent-scripts"
	}
	if !strings.HasPrefix(pref, "/") {
		pref = "/" + pref
	}
	rel := path.Join(strings.TrimPrefix(pref, "/"), strings.TrimSpace(storageRelPath))
	full := "/" + rel
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		return full
	}
	return base + full
}

func buildSyncActionsForPlatform(
	published []biz.PublishedScriptMeta,
	reported []*v1.ScriptRow,
	publicBaseURL, urlPrefix string,
) []*v1.SyncAction {
	rep := make(map[string]*v1.ScriptRow)
	for _, row := range reported {
		if row == nil {
			continue
		}
		sid := strings.TrimSpace(row.GetScriptId())
		if sid == "" {
			continue
		}
		rep[sid] = row
	}
	var out []*v1.SyncAction
	for i := range published {
		pub := &published[i]
		if !strings.EqualFold(strings.TrimSpace(pub.Status), "published") {
			continue
		}
		if !needsScriptSync(pub, rep[pub.ScriptID]) {
			continue
		}
		url := buildDownloadURL(publicBaseURL, urlPrefix, pub.StorageRelPath)
		out = append(out, &v1.SyncAction{
			Action:        "download",
			ScriptId:      pub.ScriptID,
			Version:       pub.Version,
			PackageSha256: pub.SHA256,
			Reason:        "version or checksum mismatch / not installed",
			Download: &v1.DownloadSpec{
				Method: "GET",
				Url:    url,
			},
		})
	}
	return out
}
