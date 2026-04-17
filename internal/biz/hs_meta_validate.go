package biz

import (
	"errors"
	"strings"
	"unicode"
)

// ValidateCoreHS6 校验核心码为 6 位十进制数字。
func ValidateCoreHS6(s string) error {
	t := strings.TrimSpace(s)
	if len(t) != 6 {
		return errors.New("core_hs6 须为 6 位数字")
	}
	for _, r := range t {
		if r < '0' || r > '9' {
			return errors.New("core_hs6 须为 6 位数字")
		}
	}
	return nil
}

// NormalizeHsMetaText trim；元器件名不允许空。
func NormalizeHsMetaText(s string) string {
	return strings.TrimSpace(s)
}

// ValidateHsMetaComponentName 非空且长度上限。
func ValidateHsMetaComponentName(s string) error {
	t := NormalizeHsMetaText(s)
	if t == "" {
		return errors.New("component_name 不能为空")
	}
	if len([]rune(t)) > 128 {
		return errors.New("component_name 过长")
	}
	for _, r := range t {
		if unicode.IsControl(r) {
			return errors.New("component_name 含非法控制字符")
		}
	}
	return nil
}
