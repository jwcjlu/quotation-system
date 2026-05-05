package biz

import "testing"

func TestSessionLinesNeedingPhase1MfrCleaning(t *testing.T) {
	mTI := "TI"
	mEmpty := ""
	canon := "MFR_TI"
	tests := []struct {
		name  string
		lines []LinePhase1CleaningSnap
		want  []LinePhase1CleaningNeed
	}{
		{
			name: "excludes_empty_mfr",
			lines: []LinePhase1CleaningSnap{
				{LineNo: 1, Mfr: &mEmpty, ManufacturerCanonicalID: nil},
				{LineNo: 2, Mfr: &mTI, ManufacturerCanonicalID: nil},
			},
			want: []LinePhase1CleaningNeed{{LineNo: 2, Mfr: "TI"}},
		},
		{
			name: "excludes_when_canonical_set",
			lines: []LinePhase1CleaningSnap{
				{LineNo: 1, Mfr: &mTI, ManufacturerCanonicalID: &canon},
			},
			want: nil,
		},
		{
			name: "nil_mfr_skipped",
			lines: []LinePhase1CleaningSnap{
				{LineNo: 1, Mfr: nil, ManufacturerCanonicalID: nil},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SessionLinesNeedingPhase1MfrCleaning(tt.lines)
			if len(got) != len(tt.want) {
				t.Fatalf("len(got)=%d len(want)=%d got=%#v want=%#v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("idx %d got %#v want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
