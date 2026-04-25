package biz

const (
	MatchRunSaved      = "saved"
	MatchRunSuperseded = "superseded"
	MatchRunCanceled   = "canceled"

	MatchResultAutoMatch       = "auto_match"
	MatchResultManualQuote     = "manual_quote"
	MatchResultSubstituteMatch = "substitute_match"
	MatchResultUnresolved      = "unresolved"
)

type BOMMatchResultItemDraft struct {
	LineID                   int64
	LineNo                   int
	SourceType               string
	MatchStatus              string
	GapID                    uint64
	QuoteItemID              uint64
	PlatformID               string
	DemandMpn                string
	DemandMfr                string
	DemandPackage            string
	DemandQty                float64
	MatchedMpn               string
	MatchedMfr               string
	MatchedPackage           string
	Stock                    int64
	LeadTime                 string
	UnitPrice                float64
	Subtotal                 float64
	Currency                 string
	OriginalMpn              string
	SubstituteMpn            string
	SubstituteReason         string
	CodeTS                   string
	ControlMark              string
	ImportTaxImpOrdinaryRate string
	ImportTaxImpDiscountRate string
	ImportTaxImpTempRate     string
	SnapshotJSON             []byte
}

func SummarizeMatchRunItems(items []BOMMatchResultItemDraft) (total float64, matched int, unresolved int) {
	for _, it := range items {
		if it.SourceType == MatchResultUnresolved {
			unresolved++
			continue
		}
		matched++
		total += it.Subtotal
	}
	return total, matched, unresolved
}

func MatchResultSourceFromMatchStatus(matchStatus string, manual bool, substitute bool) string {
	if matchStatus == "no_match" || matchStatus == "" {
		return MatchResultUnresolved
	}
	if substitute {
		return MatchResultSubstituteMatch
	}
	if manual {
		return MatchResultManualQuote
	}
	return MatchResultAutoMatch
}
