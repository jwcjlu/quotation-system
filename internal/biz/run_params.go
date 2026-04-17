package biz

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ExpandRunParams 将平台配置的键值对象展开为传给爬虫进程的 argv 片段（不含 entry、不含每任务动态的 --model）。
//
// 约定：键为 snake_case，展开为 --snake-case；bool true 仅追加标志；false/nil 忽略；string 空忽略；
// number 使用十进制字符串；其它类型报错。
func ExpandRunParams(m map[string]interface{}) ([]string, error) {
	if len(m) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		flag := "--" + strings.ReplaceAll(k, "_", "-")
		v := m[k]
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case bool:
			if t {
				out = append(out, flag)
			}
		case float64:
			out = append(out, flag, strconv.FormatFloat(t, 'f', -1, 64))
		case json.Number:
			out = append(out, flag, string(t))
		case string:
			if strings.TrimSpace(t) == "" {
				continue
			}
			out = append(out, flag, t)
		default:
			return nil, fmt.Errorf("run_params key %q: unsupported type %T", k, v)
		}
	}
	return out, nil
}

// ExpandRunParamsJSON 解析 JSON 对象字节后展开；空或 null 返回 (nil, nil)。
func ExpandRunParamsJSON(raw []byte) ([]string, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("run_params json: %w", err)
	}
	return ExpandRunParams(m)
}

// DefaultBOMSearchArgvSuffix 在 run_params 展开结果之后追加的每任务参数（型号）。
func DefaultBOMSearchArgvSuffix(mpnNorm string) []string {
	return []string{"--model", strings.TrimSpace(mpnNorm)}
}

// MergeBOMSearchArgv run_params 展开段 + 每任务 --model；若展开为空则使用与历史硬编码一致的默认。
func MergeBOMSearchArgv(runParamsJSON []byte, mpnNorm string) ([]string, error) {
	extra, err := ExpandRunParamsJSON(runParamsJSON)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		extra = []string{"--parse-workers", "8"}
	}
	suf := DefaultBOMSearchArgvSuffix(mpnNorm)
	out := make([]string, 0, len(extra)+len(suf))
	out = append(out, extra...)
	out = append(out, suf...)
	return out, nil
}
