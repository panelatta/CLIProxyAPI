package thinking

import "testing"

func TestParseSuffix(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantModel string
		wantRaw   string
		wantHas   bool
	}{
		{
			name:      "parenthesized level suffix",
			model:     "gpt-5.2(high)",
			wantModel: "gpt-5.2",
			wantRaw:   "high",
			wantHas:   true,
		},
		{
			name:      "at-sign level suffix",
			model:     "gpt-5.4@low",
			wantModel: "gpt-5.4",
			wantRaw:   "low",
			wantHas:   true,
		},
		{
			name:      "hash level suffix",
			model:     "gpt-5.4#low",
			wantModel: "gpt-5.4",
			wantRaw:   "low",
			wantHas:   true,
		},
		{
			name:      "no suffix",
			model:     "gpt-5.4",
			wantModel: "gpt-5.4",
			wantRaw:   "",
			wantHas:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSuffix(tt.model)
			if got.ModelName != tt.wantModel {
				t.Fatalf("ModelName = %q, want %q", got.ModelName, tt.wantModel)
			}
			if got.RawSuffix != tt.wantRaw {
				t.Fatalf("RawSuffix = %q, want %q", got.RawSuffix, tt.wantRaw)
			}
			if got.HasSuffix != tt.wantHas {
				t.Fatalf("HasSuffix = %v, want %v", got.HasSuffix, tt.wantHas)
			}
		})
	}
}
