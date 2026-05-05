package service

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"caichip/internal/biz"
	"caichip/internal/data"

	"github.com/panjf2000/ants/v2"
)

// mfrReadLineTask 一行 × 一个合并 MPN 键，对应一次 ListBomQuoteItemsForSessionLineRead。
type mfrReadLineTask struct {
	line data.BomSessionLine
	mk   string
}

func mfrReadLineTasks(lines []data.BomSessionLine) []mfrReadLineTask {
	var tasks []mfrReadLineTask
	for _, line := range lines {
		for _, mk := range mergeMpnKeysForBOMLine(line) {
			tasks = append(tasks, mfrReadLineTask{line: line, mk: mk})
		}
	}
	return tasks
}

// parallelMfrReadLineTasks 对 tasks 做有界并发；单任务直接用 ctx 串行。
// execute 返回 stop=true 时取消其余任务（无 error）；返回 error 时记录首个错误并取消。
func parallelMfrReadLineTasks(ctx context.Context, tasks []mfrReadLineTask, execute func(workCtx context.Context, t mfrReadLineTask) (stop bool, err error)) error {
	if len(tasks) == 0 {
		return nil
	}
	if len(tasks) == 1 {
		_, err := execute(ctx, tasks[0])
		return err
	}

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	var firstErr error
	setErr := func(e error) {
		if e == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = e
			cancel()
		}
	}

	workers := len(tasks)
	if workers > maxBomMatchLineWorkers {
		workers = maxBomMatchLineWorkers
	}
	pool, err := ants.NewPool(workers)
	if err != nil {
		return err
	}
	defer pool.Release()

	var wg sync.WaitGroup
	for _, t := range tasks {
		if err := ctx.Err(); err != nil {
			return err
		}
		wg.Add(1)
		t := t
		submitErr := pool.Submit(func() {
			defer wg.Done()
			if workCtx.Err() != nil {
				return
			}
			mu.Lock()
			fe := firstErr
			mu.Unlock()
			if fe != nil {
				return
			}
			stop, exErr := execute(workCtx, t)
			if exErr != nil {
				if workCtx.Err() != nil && errors.Is(exErr, context.Canceled) {
					return
				}
				setErr(exErr)
				return
			}
			if stop {
				cancel()
			}
		})
		if submitErr != nil {
			wg.Done()
			return submitErr
		}
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return err
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

func mfrReviewQuoteItemFromBomQuoteReadRow(r biz.BomQuoteItemReadRow, lineID int64) *biz.MfrReviewQuoteItem {
	var canon *string
	if c := strings.TrimSpace(r.ManufacturerCanonicalID); c != "" {
		canon = &c
	}
	st := strings.TrimSpace(r.ManufacturerReviewStatus)
	if st == "" {
		st = biz.MfrReviewPending
	}
	lid := lineID
	return &biz.MfrReviewQuoteItem{
		ID:                       r.ItemID,
		LineID:                   &lid,
		Manufacturer:             strings.TrimSpace(r.Manufacturer),
		ManufacturerCanonicalID:  canon,
		ManufacturerReviewStatus: st,
	}
}

type pickUpMfrReview struct {
	readRows      []pickUpMfrReviewItem
	task          mfrReadLineTask
	appendPending func(biz.MfrReviewQuoteItem)
}

func newPickUpMfrReview(readRows []biz.BomQuoteItemReadRow, task mfrReadLineTask, appendPending func(biz.MfrReviewQuoteItem)) pickUpMfrReview {
	var rows []pickUpMfrReviewItem
	for _, row := range readRows {
		s, err := strconv.Atoi(row.Stock)
		if err != nil {
			continue
		}
		if float64(s) < *task.line.Qty {
			continue
		}
		if row.ManufacturerReviewStatus == biz.MfrReviewRejected {
			continue
		}
		rows = append(rows, pickUpMfrReviewItem{
			readRow: row,
			price:   ParsePriceTier(row.PriceTiers).lowestPrice(int(*task.line.Qty)),
		})

	}
	return pickUpMfrReview{readRows: rows, task: task, appendPending: appendPending}
}

func (pick pickUpMfrReview) pickUp() {
	sort.Slice(pick.readRows, func(i, j int) bool {
		return pick.readRows[i].price < pick.readRows[j].price
	})
	paas := true
	acceptedCount := 0
	pendingCount := 0
	for _, row := range pick.readRows {
		if row.readRow.ManufacturerReviewStatus == biz.MfrReviewAccepted {
			acceptedCount++
			continue
		} else {
			paas = false
		}

		if (paas && acceptedCount >= 3) || pendingCount+acceptedCount >= 5 {
			return
		}

		lineNo := int64(pick.task.line.LineNo)
		if row.readRow.ManufacturerReviewStatus == biz.MfrReviewPending {
			pick.appendPending(biz.MfrReviewQuoteItem{
				ID:                       row.readRow.ItemID,
				LineID:                   &lineNo,
				Manufacturer:             strings.TrimSpace(row.readRow.Manufacturer),
				ManufacturerCanonicalID:  pick.task.line.ManufacturerCanonicalID,
				ManufacturerReviewStatus: biz.MfrReviewPending,
			})
			pendingCount++
		}

	}
}

type pickUpMfrReviewItem struct {
	readRow biz.BomQuoteItemReadRow
	price   float64
}

type priceTiers []priceTier

func (tiers priceTiers) lowestPrice(count int) float64 {
	if len(tiers) == 0 {
		return math.MaxInt16
	}
	for _, tier := range tiers {
		if tier.MinimumQuantity > count {
			return tier.Price
		}
	}
	return math.MaxInt16
}

type priceTier struct {
	MinimumQuantity int
	Price           float64
	Currency        string
}

var priceTierRe = regexp.MustCompile(`(\d+)\+\s*([￥¥$])?\s*([\d.]+)`)

func ParsePriceTier(tiers string) priceTiers {
	matches := priceTierRe.FindAllStringSubmatch(tiers, -1)
	if len(matches) == 0 {
		return nil
	}
	parsed := make([]priceTier, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		minimumQuantity, _ := strconv.Atoi(match[1])
		price, _ := strconv.ParseFloat(match[3], 64)
		parsed = append(parsed, priceTier{
			MinimumQuantity: minimumQuantity,
			Price:           price,
			Currency:        strings.TrimSpace(match[2]),
		})
	}
	if len(parsed) == 0 {
		return nil
	}
	return parsed
}
