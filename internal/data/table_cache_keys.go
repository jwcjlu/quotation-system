package data

import (
	"strconv"
	"strings"
)

const (
	prefixAsAuthAgent   = "asauth:agent:"
	prefixAsAuthPair    = "asauth:pair:"
	prefixBomPlat       = "bomplat:"
	keyBomPlatAll       = "bomplat:all"
	prefixAgInstAgent   = "aginst:agent:"
	prefixMfrAliasNorm  = "mfalias:norm:"
	prefixMfrAliasCanon = "mfalias:canonicals:"
)

// KeyAsAuthAgent 某 Agent 下凭据列表（ListByAgent 摘要，无密码）。
func KeyAsAuthAgent(agentID string) string {
	return prefixAsAuthAgent + strings.TrimSpace(agentID)
}

// KeyAsAuthPair 单行密文缓存键（username + password_cipher，解密在 Cached 层）。
func KeyAsAuthPair(agentID, scriptID string) string {
	return prefixAsAuthPair + strings.TrimSpace(agentID) + ":" + strings.TrimSpace(scriptID)
}

// KeyBomPlatformAll 全表 List 缓存。
func KeyBomPlatformAll() string { return keyBomPlatAll }

// KeyBomPlatformOne 单行 Get 缓存（可选）。
func KeyBomPlatformOne(platformID string) string {
	return prefixBomPlat + strings.TrimSpace(platformID)
}

// KeyAgInstAgent 某 Agent 已安装脚本列表。
func KeyAgInstAgent(agentID string) string {
	return prefixAgInstAgent + strings.TrimSpace(agentID)
}

// KeyMfrAliasNorm CanonicalID 点查。
func KeyMfrAliasNorm(aliasNorm string) string {
	return prefixMfrAliasNorm + strings.TrimSpace(aliasNorm)
}

// KeyMfrAliasCanonicalsList ListDistinctCanonicals（带 limit）。
func KeyMfrAliasCanonicalsList(limit int) string {
	return prefixMfrAliasCanon + "limit:" + strconv.Itoa(limit)
}
