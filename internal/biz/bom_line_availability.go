package biz

const (
	LineAvailabilityReady                 = "ready"
	LineAvailabilityNoData                = "no_data"
	LineAvailabilityCollectionUnavailable = "collection_unavailable"
	LineAvailabilityNoMatchAfterFilter    = "no_match_after_filter"
	LineAvailabilityCollecting            = "collecting"
)

type PlatformAvailabilityFact struct {
	PlatformID            string
	TaskState             string
	HasRawQuote           bool
	HasUsableQuote        bool
	NoData                bool
	CollectionUnavailable bool
	ReasonCode            string
	Message               string
	AutoAttempt           int
	ManualAttempt         int
}

type LineAvailabilityInput struct {
	LineNo    int
	MpnNorm   string
	Platforms []PlatformAvailabilityFact
}

type LineAvailability struct {
	LineNo                   int
	MpnNorm                  string
	Status                   string
	ReasonCode               string
	Reason                   string
	HasUsableQuote           bool
	RawQuotePlatformCount    int
	UsableQuotePlatformCount int
	ResolutionStatus         string
	PlatformFacts            []PlatformAvailabilityFact
}

type LineAvailabilitySummary struct {
	LineTotal                      int
	ReadyLineCount                 int
	GapLineCount                   int
	NoDataLineCount                int
	CollectionUnavailableLineCount int
	NoMatchAfterFilterLineCount    int
	CollectingLineCount            int
}

func ClassifyLineAvailability(input LineAvailabilityInput) LineAvailability {
	availability := LineAvailability{
		LineNo:           input.LineNo,
		MpnNorm:          input.MpnNorm,
		Status:           LineAvailabilityCollectionUnavailable,
		ReasonCode:       "COLLECTION_UNAVAILABLE",
		Reason:           "collection unavailable",
		PlatformFacts:    input.Platforms,
		ResolutionStatus: "open",
	}
	if len(input.Platforms) == 0 {
		return availability
	}

	allNoData := true
	hasCollectionUnavailable := false
	hasCollecting := false

	for _, platform := range input.Platforms {
		if !isPlatformTerminal(platform.TaskState) {
			hasCollecting = true
		}
		if platform.HasUsableQuote {
			availability.HasUsableQuote = true
			availability.UsableQuotePlatformCount++
		}
		if platform.HasRawQuote {
			availability.RawQuotePlatformCount++
		}
		if platform.CollectionUnavailable || platform.TaskState == "failed_terminal" {
			hasCollectionUnavailable = true
		}
		if !platform.NoData {
			allNoData = false
		}
	}

	switch {
	case hasCollecting:
		setLineAvailabilityStatus(&availability, LineAvailabilityCollecting)
	case availability.HasUsableQuote:
		setLineAvailabilityStatus(&availability, LineAvailabilityReady)
	case availability.RawQuotePlatformCount > 0:
		setLineAvailabilityStatus(&availability, LineAvailabilityNoMatchAfterFilter)
	case hasCollectionUnavailable:
		setLineAvailabilityStatus(&availability, LineAvailabilityCollectionUnavailable)
	case allNoData:
		setLineAvailabilityStatus(&availability, LineAvailabilityNoData)
	}
	return availability
}

func SummarizeLineAvailability(lines []LineAvailability) LineAvailabilitySummary {
	var summary LineAvailabilitySummary
	summary.LineTotal = len(lines)

	for _, line := range lines {
		switch line.Status {
		case LineAvailabilityReady:
			summary.ReadyLineCount++
		case LineAvailabilityNoData:
			summary.NoDataLineCount++
			summary.GapLineCount++
		case LineAvailabilityCollectionUnavailable:
			summary.CollectionUnavailableLineCount++
			summary.GapLineCount++
		case LineAvailabilityNoMatchAfterFilter:
			summary.NoMatchAfterFilterLineCount++
			summary.GapLineCount++
		case LineAvailabilityCollecting:
			summary.CollectingLineCount++
		}
	}

	return summary
}

func (s LineAvailabilitySummary) HasStrictBlockingGap() bool {
	return s.NoDataLineCount+s.CollectionUnavailableLineCount+s.NoMatchAfterFilterLineCount > 0
}

func setLineAvailabilityStatus(availability *LineAvailability, status string) {
	availability.Status = status

	switch status {
	case LineAvailabilityReady:
		availability.ReasonCode = "READY"
		availability.Reason = "usable quote available"
	case LineAvailabilityNoData:
		availability.ReasonCode = "NO_DATA"
		availability.Reason = "all platforms reported no data"
	case LineAvailabilityNoMatchAfterFilter:
		availability.ReasonCode = "NO_MATCH_AFTER_FILTER"
		availability.Reason = "raw quote exists but no usable quote remains"
	case LineAvailabilityCollecting:
		availability.ReasonCode = "COLLECTING"
		availability.Reason = "platform collection is still running"
	default:
		availability.ReasonCode = "COLLECTION_UNAVAILABLE"
		availability.Reason = "collection unavailable"
	}
}
