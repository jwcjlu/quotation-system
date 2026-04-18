package pdftext

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
)

var literalTextPattern = regexp.MustCompile(`\((?:\\.|[^\\()])*\)`)
var textObjPattern = regexp.MustCompile(`(?s)BT(.*?)ET`)
var showTextTjPattern = regexp.MustCompile(`\((?:\\.|[^\\()])*\)\s*Tj`)
var showTextTJPattern = regexp.MustCompile(`\[(.*?)\]\s*TJ`)

// ReadBodyHeadFromFile 提取 PDF 正文并截取前 maxChars 个字符。
func ReadBodyHeadFromFile(filePath string, maxChars int) (string, error) {
	pdfData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	body := extractBodyText(pdfData)
	if body == "" {
		return "", fmt.Errorf("no readable body text found in pdf")
	}
	return truncateByRunes(body, maxChars), nil
}

func extractBodyText(pdfData []byte) string {
	streams := extractStreams(pdfData)
	var parts []string
	for _, stream := range streams {
		t := extractTextFromTextObjects(stream)
		if t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return cleanText(strings.Join(parts, "\n"))
}

func extractTextFromTextObjects(content string) string {
	blocks := textObjPattern.FindAllStringSubmatch(content, -1)
	if len(blocks) == 0 {
		return ""
	}
	lines := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if len(b) < 2 {
			continue
		}
		txt := extractTextLiteralsByOperators(b[1])
		txt = strings.TrimSpace(txt)
		if txt == "" {
			continue
		}
		if !looksLikeReadableText(txt) {
			continue
		}
		lines = append(lines, txt)
	}
	return strings.Join(lines, "\n")
}

func extractTextLiteralsByOperators(textObj string) string {
	segments := make([]string, 0, 32)
	for _, m := range showTextTjPattern.FindAllString(textObj, -1) {
		s := extractTextLiterals(m)
		if s != "" {
			segments = append(segments, s)
		}
	}
	for _, m := range showTextTJPattern.FindAllStringSubmatch(textObj, -1) {
		if len(m) < 2 {
			continue
		}
		s := extractTextLiterals(m[1])
		if s != "" {
			segments = append(segments, s)
		}
	}
	return strings.Join(segments, "\n")
}

func extractStreams(pdfData []byte) []string {
	var streams []string
	cursor := 0
	for {
		start := bytes.Index(pdfData[cursor:], []byte("stream"))
		if start < 0 {
			break
		}
		start += cursor + len("stream")
		if start < len(pdfData) && pdfData[start] == '\r' {
			start++
		}
		if start < len(pdfData) && pdfData[start] == '\n' {
			start++
		}
		endRel := bytes.Index(pdfData[start:], []byte("endstream"))
		if endRel < 0 {
			break
		}
		end := start + endRel
		raw := bytes.TrimSpace(pdfData[start:end])
		if len(raw) > 0 {
			streams = append(streams, string(raw))
			if zr, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
				decoded, readErr := io.ReadAll(zr)
				_ = zr.Close()
				if readErr == nil && len(decoded) > 0 {
					streams = append(streams, string(decoded))
				}
			}
		}
		cursor = end + len("endstream")
	}
	return streams
}

func extractTextLiterals(content string) string {
	matches := literalTextPattern.FindAllString(content, -1)
	if len(matches) == 0 {
		return ""
	}
	lines := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		txt := unescapePDFLiteral(m[1 : len(m)-1])
		txt = strings.TrimSpace(txt)
		if txt == "" {
			continue
		}
		lines = append(lines, txt)
	}
	return strings.Join(lines, "\n")
}

func unescapePDFLiteral(s string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\(`, `(`,
		`\)`, `)`,
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
		`\b`, "\b",
		`\f`, "\f",
	)
	return replacer.Replace(s)
}

func cleanText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		if unicode.IsPrint(r) {
			b.WriteRune(r)
			lastSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

func looksLikeReadableText(s string) bool {
	if s == "" {
		return false
	}
	var total int
	var ok int
	for _, r := range s {
		total++
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.In(r, unicode.Han) {
			ok++
		}
	}
	if total == 0 {
		return false
	}
	return float64(ok)/float64(total) >= 0.70
}

func truncateByRunes(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
