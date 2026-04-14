package biz

type HSClassifyRequest struct {
	TradeDirection  string
	DeclarationDate string
	Model           string
	ProductNameCN   string
	ProductNameEN   string
	Manufacturer    string
	Brand           string
	Package         string
	Description     string
	CategoryHint    string
}

type HSClassifyCandidate struct {
	HSCode                  string
	Score                   float64
	Reason                  string
	Evidence                []string
	RequiredElementsMissing []string
}

type HSFinalSuggestion struct {
	HSCode            string
	Confidence        float64
	ReviewRequired    bool
	ReviewReasonCodes []string
}

type HSClassifyTrace struct {
	RuleHits           []string
	RetrievalRefs      []string
	SourceSnapshotTime string
	LLMVersion         string
	PolicyVersionID    string
}

type HSClassifyResult struct {
	Candidates      []HSClassifyCandidate
	FinalSuggestion HSFinalSuggestion
	Trace           HSClassifyTrace
}
