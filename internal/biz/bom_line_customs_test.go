package biz

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubMapRepo struct {
	byKey map[string]*HsModelMappingRecord
}

func (s *stubMapRepo) DBOk() bool { return true }

func (s *stubMapRepo) GetConfirmedByModelManufacturer(_ context.Context, model, manufacturer string) (*HsModelMappingRecord, error) {
	k := model + "|" + manufacturer
	return s.byKey[k], nil
}

func (s *stubMapRepo) Save(context.Context, *HsModelMappingRecord) error {
	return errors.New("not implemented")
}

type stubItemRepo struct {
	items map[string]*HsItemRecord
}

func (s *stubItemRepo) DBOk() bool { return true }

func (s *stubItemRepo) List(context.Context, HsItemListFilter) ([]HsItemRecord, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s *stubItemRepo) GetByCodeTS(context.Context, string) (*HsItemRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *stubItemRepo) MapByCodeTS(_ context.Context, codeTSList []string) (map[string]*HsItemRecord, error) {
	out := make(map[string]*HsItemRecord)
	for _, c := range codeTSList {
		if v, ok := s.items[c]; ok {
			out[c] = v
		}
	}
	return out, nil
}

type stubDailyRepo struct {
	rows map[string]*HsTaxRateDailyRecord
	up   []*HsTaxRateDailyRecord
}

func (s *stubDailyRepo) DBOk() bool { return true }

func (s *stubDailyRepo) GetManyByBizDate(_ context.Context, _ time.Time, codeTSList []string) (map[string]*HsTaxRateDailyRecord, error) {
	out := make(map[string]*HsTaxRateDailyRecord)
	for _, c := range codeTSList {
		if v, ok := s.rows[c]; ok {
			out[c] = v
		}
	}
	return out, nil
}

func (s *stubDailyRepo) Upsert(_ context.Context, row *HsTaxRateDailyRecord) error {
	s.up = append(s.up, row)
	s.rows[row.CodeTS] = row
	return nil
}

type stubTaxAPI struct {
	byCode map[string]*TaxRateFetchResult
	err    map[string]error
}

func (s *stubTaxAPI) FetchByCodeTS(_ context.Context, codeTS string, _ int) (*TaxRateFetchResult, error) {
	if s.err != nil {
		if e, ok := s.err[codeTS]; ok {
			return nil, e
		}
	}
	return s.byCode[codeTS], nil
}

func TestFillBomLineCustoms_foundAndNotMapped(t *testing.T) {
	mfrST := "ST"
	lines := []BomLineCustomsLine{
		{LineNo: 1, Mpn: "STM32", Mfr: &mfrST},
		{LineNo: 2, Mpn: "UNKNOWN", Mfr: nil},
	}
	mapRepo := &stubMapRepo{byKey: map[string]*HsModelMappingRecord{
		"STM32|ST": {CodeTS: "8542399000"},
	}}
	itemRepo := &stubItemRepo{items: map[string]*HsItemRecord{
		"8542399000": {ControlMark: "A"},
	}}
	daily := &stubDailyRepo{rows: map[string]*HsTaxRateDailyRecord{}}
	tax := &stubTaxAPI{byCode: map[string]*TaxRateFetchResult{
		"8542399000": {Items: []TaxRateAPIItemRow{{
			CodeTS: "8542399000", GName: "其他集成电路", ImpOrdinaryRate: "24%",
		}}},
	}}
	clock := time.Date(2026, 4, 19, 12, 0, 0, 0, time.Local)
	got, err := FillBomLineCustoms(context.Background(), lines, mapRepo, itemRepo, daily, tax, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].HsCodeStatus != HsCodeStatusFound || got[0].CodeTS != "8542399000" {
		t.Fatalf("line1 %+v", got[0])
	}
	if got[0].ControlMark != "A" || got[0].ImportTaxImpOrdinaryRate != "24%" {
		t.Fatalf("line1 tax/control %+v", got[0])
	}
	if got[1].HsCodeStatus != HsCodeStatusNotMapped {
		t.Fatalf("line2 %+v", got[1])
	}
}

func TestFillBomLineCustoms_codeInvalid(t *testing.T) {
	mfr := "X"
	lines := []BomLineCustomsLine{{LineNo: 1, Mpn: "M1", Mfr: &mfr}}
	mapRepo := &stubMapRepo{byKey: map[string]*HsModelMappingRecord{
		"M1|X": {CodeTS: "bad"},
	}}
	itemRepo := &stubItemRepo{items: map[string]*HsItemRecord{}}
	daily := &stubDailyRepo{rows: map[string]*HsTaxRateDailyRecord{}}
	tax := &stubTaxAPI{}
	got, err := FillBomLineCustoms(context.Background(), lines, mapRepo, itemRepo, daily, tax, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].HsCodeStatus != HsCodeStatusCodeInvalid {
		t.Fatalf("got %+v", got[0])
	}
}

func TestFillBomLineCustoms_itemMissingStillTax(t *testing.T) {
	mfr := "ST"
	lines := []BomLineCustomsLine{{LineNo: 3, Mpn: "STM32", Mfr: &mfr}}
	mapRepo := &stubMapRepo{byKey: map[string]*HsModelMappingRecord{
		"STM32|ST": {CodeTS: "8542399000"},
	}}
	itemRepo := &stubItemRepo{items: map[string]*HsItemRecord{}}
	daily := &stubDailyRepo{rows: map[string]*HsTaxRateDailyRecord{}}
	tax := &stubTaxAPI{byCode: map[string]*TaxRateFetchResult{
		"8542399000": {Items: []TaxRateAPIItemRow{{
			CodeTS: "8542399000", ImpOrdinaryRate: "10%",
		}}},
	}}
	got, err := FillBomLineCustoms(context.Background(), lines, mapRepo, itemRepo, daily, tax, func() time.Time {
		return time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].CustomsErrors) != 1 || got[0].CustomsErrors[0] != CustomsErrHSItemMissing {
		t.Fatalf("errs=%v", got[0].CustomsErrors)
	}
	if got[0].ImportTaxImpOrdinaryRate != "10%" {
		t.Fatalf("tax %+v", got[0])
	}
}
