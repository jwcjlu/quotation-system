package biz

import "strings"

const (
	// UserUploadDatasheetURLPrefix 用户上传占位 URL 前缀（禁止走 HTTP 下载）。
	UserUploadDatasheetURLPrefix = "user-upload://"
	// ManualDescriptionOnlyURLPrefix 仅文本旁路、无本地 PDF 时的占位 URL。
	ManualDescriptionOnlyURLPrefix = "manual-description-only://"
)

// IsUserUploadDatasheetURL 是否为 user-upload 占位 URL。
func IsUserUploadDatasheetURL(raw string) bool {
	s := strings.TrimSpace(strings.ToLower(raw))
	return strings.HasPrefix(s, strings.ToLower(UserUploadDatasheetURLPrefix))
}

// IsBlockedHTTPCDatasheetURL 不应由 HTTP 下载器拉取的 URL（占位或仅文本）。
func IsBlockedHTTPCDatasheetURL(raw string) bool {
	s := strings.TrimSpace(strings.ToLower(raw))
	if strings.HasPrefix(s, strings.ToLower(UserUploadDatasheetURLPrefix)) {
		return true
	}
	return strings.HasPrefix(s, strings.ToLower(ManualDescriptionOnlyURLPrefix))
}
