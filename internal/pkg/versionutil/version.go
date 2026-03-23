// Package versionutil 实现需求文档 §6.5：version.txt / 任务 version 规范化比对。
package versionutil

import "strings"

// Normalize 去首尾空白；若以 v/V 开头且表示版本号则去掉前缀（与需求 §6.5 一致）。
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == 'v' || s[0] == 'V') {
		// 简单约定：v 前缀后仍有内容则去掉单字符前缀
		return s[1:]
	}
	return s
}

// Equal 规范化后字符串相等。
func Equal(a, b string) bool {
	return Normalize(a) == Normalize(b)
}
