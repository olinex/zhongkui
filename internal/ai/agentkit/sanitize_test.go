package agentkit

import (
	"encoding/json"
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/ai/contract"
)

func TestSanitizeBusinessDataCleansAndKeepsInput(t *testing.T) {
	params := testParams(map[string]string{
		"sanitize.maxItems": "0",
		"sanitize.maxBytes": "0",
	})
	data := contract.BusinessData{
		"nil":   nil,
		"empty": "",
		"list":  []string{},
		"map":   map[string]any{},
		"zero":  0,
		"false": false,
		"keep":  "v",
		"nested": map[string]any{
			"keep":  "inner",
			"drop":  "",
			"empty": []any{},
		},
	}
	out, err := SanitizeBusinessData(data, params)
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	for _, key := range []string{"nil", "empty", "list", "map"} {
		if _, ok := out[key]; ok {
			t.Errorf("kept empty key %q", key)
		}
	}
	for _, key := range []string{"zero", "false", "keep", "nested"} {
		if _, ok := out[key]; !ok {
			t.Errorf("dropped valid key %q", key)
		}
	}
	nested := out["nested"].(map[string]any)
	if _, ok := nested["drop"]; ok {
		t.Errorf("nested empty entry was kept")
	}
	if nested["keep"] != "inner" {
		t.Errorf("nested keep = %v", nested["keep"])
	}
	if _, ok := data["nil"]; !ok {
		t.Errorf("input map was mutated")
	}
}

func TestSanitizeBusinessDataDropsLargestUntilFits(t *testing.T) {
	params := testParams(map[string]string{
		"sanitize.maxItems": "0",
		"sanitize.maxBytes": "1100",
	})
	big := strings.Repeat("x", 1000)
	data := contract.BusinessData{"big1": big, "big2": big, "small": "s"}
	out, err := SanitizeBusinessData(data, params)
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	if len(payload) > 1100 {
		t.Errorf("sanitized size = %d, want <= 1100", len(payload))
	}
	if _, ok := out["small"]; !ok {
		t.Errorf("short key was dropped")
	}
	keptBig := 0
	for _, key := range []string{"big1", "big2"} {
		if _, ok := out[key]; ok {
			keptBig++
		}
	}
	if keptBig != 1 {
		t.Errorf("kept big entries = %d, want 1", keptBig)
	}
}

func TestSanitizeBusinessDataMaxItems(t *testing.T) {
	params := testParams(map[string]string{
		"sanitize.maxItems": "2",
		"sanitize.maxBytes": "0",
	})
	out, err := SanitizeBusinessData(contract.BusinessData{
		"a": "1", "b": "2", "c": "3",
	}, params)
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
}
