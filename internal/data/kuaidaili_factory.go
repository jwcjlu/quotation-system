package data

import (
	"strings"

	"caichip/internal/conf"
	"caichip/pkg/kuaidaili"

	"github.com/go-kratos/kratos/v2/log"
)

// NewKuaidailiClient 由配置创建客户端；未启用或缺密钥时返回 nil。
func NewKuaidailiClient(px *conf.BootstrapProxy, logger log.Logger) *kuaidaili.Client {
	if px == nil || px.Kuaidaili == nil || !px.Kuaidaili.Enabled {
		return nil
	}
	k := px.Kuaidaili
	num := int(k.Num)
	if num <= 0 {
		num = 1
	}
	c, err := kuaidaili.NewClient(kuaidaili.Config{
		BaseURL:     strings.TrimSpace(k.BaseURL),
		SecretID:    strings.TrimSpace(k.SecretID),
		SignType:    strings.TrimSpace(k.SignType),
		SecretKey:   strings.TrimSpace(k.SecretKey),
		SecretToken: strings.TrimSpace(k.SecretToken),
		Num:         num,
		Area:        strings.TrimSpace(k.Area),
		FAuth:       int(k.FAuth),
		MaxQPS:      k.MaxQPS,
	})
	if err != nil {
		log.NewHelper(logger).Warnf("kuaidaili client disabled: %v", err)
		return nil
	}
	return c
}
