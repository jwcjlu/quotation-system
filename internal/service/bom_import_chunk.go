package service

import (
	"sort"

	"caichip/internal/biz"
)

func splitRowsWithHeader(rows [][]string, chunkSize int) [][][]string {
	if len(rows) <= 1 {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = llmChunkSize
	}

	header := append([]string(nil), rows[0]...)
	dataRows := rows[1:]
	out := make([][][]string, 0, (len(dataRows)+chunkSize-1)/chunkSize)
	for start := 0; start < len(dataRows); start += chunkSize {
		end := start + chunkSize
		if end > len(dataRows) {
			end = len(dataRows)
		}
		chunk := make([][]string, 0, 1+(end-start))
		chunk = append(chunk, header)
		for i := start; i < end; i++ {
			chunk = append(chunk, dataRows[i])
		}
		out = append(out, chunk)
	}
	return out
}

// normalizeChunkLineNos maps chunk-local line numbers back to absolute Excel row
// numbers and keeps the final sequence unique and ordered.
func normalizeChunkLineNos(lines []biz.BomImportLine, chunkStartDataIdx, chunkDataLen int) []biz.BomImportLine {
	if len(lines) == 0 || chunkDataLen <= 0 {
		return lines
	}

	firstExcelRow := 2 + chunkStartDataIdx
	lastExcelRow := firstExcelRow + chunkDataLen - 1
	used := make(map[int]struct{}, len(lines))
	nextCandidate := firstExcelRow
	for i := range lines {
		n := lines[i].LineNo
		switch {
		case n >= firstExcelRow && n <= lastExcelRow:
		case n >= 2 && n <= chunkDataLen+1:
			n = firstExcelRow + (n - 2)
		default:
			n = firstExcelRow + i
		}
		if n < firstExcelRow {
			n = firstExcelRow
		}
		if n > lastExcelRow {
			n = lastExcelRow
		}
		if _, ok := used[n]; ok {
			for nextCandidate <= lastExcelRow {
				if _, taken := used[nextCandidate]; !taken {
					n = nextCandidate
					break
				}
				nextCandidate++
			}
		}
		used[n] = struct{}{}
		lines[i].LineNo = n
		if n >= nextCandidate {
			nextCandidate = n + 1
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].LineNo < lines[j].LineNo })
	return lines
}
