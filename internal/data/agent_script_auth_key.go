package data

import (
	"encoding/base64"
	"os"
	"strings"

	"caichip/internal/conf"
)

// ResolveAgentScriptAuthKey 解析 32 字节 AES 密钥：环境变量 CAICHIP_AGENT_SCRIPT_AUTH_KEY（base64）优先，否则 bootstrap.agent_script_auth.aes_key_base64。
func ResolveAgentScriptAuthKey(bc *conf.Bootstrap) []byte {
	if s := strings.TrimSpace(os.Getenv("CAICHIP_AGENT_SCRIPT_AUTH_KEY")); s != "" {
		if k := decodeKeyB64(s); len(k) == 32 {
			return k
		}
	}
	if bc != nil && bc.GetAgentScriptAuth() != nil {
		if s := strings.TrimSpace(bc.GetAgentScriptAuth().GetAesKeyBase64()); s != "" {
			if k := decodeKeyB64(s); len(k) == 32 {
				return k
			}
		}
	}
	return nil
}

func decodeKeyB64(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) == 0 {
		return nil
	}
	return b
}
