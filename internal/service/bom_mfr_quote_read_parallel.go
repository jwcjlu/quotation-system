package service

import (
	"context"
	"errors"
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

func appendPendingMfrFromBOMQuoteReadRows(readRows []biz.BomQuoteItemReadRow, lineID int64, appendPending func(biz.MfrReviewQuoteItem)) {
	for _, r := range readRows {
		st := strings.TrimSpace(r.ManufacturerReviewStatus)
		if st != biz.MfrReviewPending && st != "" {
			continue
		}
		if r.ItemID == 0 {
			continue
		}
		var canon *string
		if c := strings.TrimSpace(r.ManufacturerCanonicalID); c != "" {
			canon = &c
		}
		lid := lineID
		appendPending(biz.MfrReviewQuoteItem{
			ID:                       r.ItemID,
			LineID:                   &lid,
			Manufacturer:             strings.TrimSpace(r.Manufacturer),
			ManufacturerCanonicalID:  canon,
			ManufacturerReviewStatus: biz.MfrReviewPending,
		})
	}
}
