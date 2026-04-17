package data

import "testing"

func TestLLMExtractClient_ParseStrictJSON(t *testing.T) {
	client := NewHsLLMExtractClient(nil)
	t.Run("empty response", func(t *testing.T) {
		if _, err := client.ParseStrictJSON("   "); err == nil {
			t.Fatalf("expected empty response to fail")
		}
	})

	t.Run("non json response", func(t *testing.T) {
		if _, err := client.ParseStrictJSON("not-json"); err == nil {
			t.Fatalf("expected non-json to fail")
		}
	})

	t.Run("multiple json objects", func(t *testing.T) {
		raw := `{"tech_category":"a","component_name":"b","package_form":"c","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"f","quote":"q","page":1}]} {"x":1}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected multiple json objects to fail")
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		raw := `{"tech_category":"a","component_name":"b","package_form":"c","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"f","quote":"q","page":1}],"unknown":1}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected unknown field to fail")
		}
	})

	t.Run("missing required top-level key", func(t *testing.T) {
		raw := `{"tech_category":"","component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]}}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected missing evidence key to fail")
		}
	})

	t.Run("missing required key_specs key", func(t *testing.T) {
		raw := `{"tech_category":"","component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":""},"evidence":[]}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected missing key_specs.other key to fail")
		}
	})

	t.Run("invalid evidence struct", func(t *testing.T) {
		raw := `{"tech_category":"a","component_name":"b","package_form":"c","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"","quote":"q","page":1}]}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected invalid evidence to fail")
		}
	})

	t.Run("missing fields but valid", func(t *testing.T) {
		// spec: allow empty strings and empty evidence.
		raw := `{"tech_category":"","component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[]}`
		got, err := client.ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("expected empty fields allowed, got err=%v", err)
		}
		if got == nil {
			t.Fatalf("expected non-nil result")
		}
	})

	t.Run("evidence page zero allowed", func(t *testing.T) {
		raw := `{"tech_category":"","component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"component_name","quote":"MOSFET","page":0}]}`
		if _, err := client.ParseStrictJSON(raw); err != nil {
			t.Fatalf("expected evidence page=0 allowed, got err=%v", err)
		}
	})

	t.Run("valid path", func(t *testing.T) {
		raw := `{"tech_category":"半导体器件","component_name":"MOSFET","package_form":"SOT-23","key_specs":{"voltage":"30V","current":"5A","power":"","frequency":"","temperature":"125C","other":["Rds(on)"]},"evidence":[{"field":"component_name","quote":"N-Channel MOSFET","page":1}]}`
		got, err := client.ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("expected valid json success, got err=%v", err)
		}
		if got.TechCategory != "半导体器件" || got.ComponentName != "MOSFET" {
			t.Fatalf("unexpected parsed content")
		}
	})
}
