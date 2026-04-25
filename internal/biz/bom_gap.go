package biz

import "fmt"

const (
	LineAvailabilityNoData                = "no_data"
	LineAvailabilityCollectionUnavailable = "collection_unavailable"
	LineAvailabilityNoMatchAfterFilter    = "no_match_after_filter"

	LineGapNoData                = "NO_DATA"
	LineGapCollectionUnavailable = "COLLECTION_UNAVAILABLE"
	LineGapNoMatchAfterFilter    = "NO_MATCH_AFTER_FILTER"

	LineGapOpen               = "open"
	LineGapManualQuoteAdded   = "manual_quote_added"
	LineGapSubstituteSelected = "substitute_selected"
	LineGapResolved           = "resolved"
	LineGapIgnored            = "ignored"
)

type BOMLineGap struct {
	ID               uint64
	SessionID        string
	LineID           int64
	LineNo           int
	Mpn              string
	GapType          string
	ReasonCode       string
	ReasonDetail     string
	Status           string
	ResolutionNote   string
	SubstituteMpn    string
	SubstituteReason string
}

func (g BOMLineGap) ActiveKey() string {
	if g.Status != LineGapOpen {
		return ""
	}
	return fmt.Sprintf("%s:%d:%s", g.SessionID, g.LineID, g.GapType)
}

func CanTransitionLineGap(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case LineGapOpen:
		return to == LineGapManualQuoteAdded || to == LineGapSubstituteSelected ||
			to == LineGapResolved || to == LineGapIgnored
	case LineGapManualQuoteAdded, LineGapSubstituteSelected:
		return to == LineGapResolved || to == LineGapIgnored
	default:
		return false
	}
}

func AvailabilityStatusToGapType(status string) string {
	switch status {
	case LineAvailabilityNoData:
		return LineGapNoData
	case LineAvailabilityCollectionUnavailable:
		return LineGapCollectionUnavailable
	case LineAvailabilityNoMatchAfterFilter:
		return LineGapNoMatchAfterFilter
	default:
		return ""
	}
}
