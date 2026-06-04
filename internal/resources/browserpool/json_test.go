package browserpool

import (
	"encoding/json"
	"testing"
)

func TestNormalizeChromePolicyJSON(t *testing.T) {
	got, diags := NormalizeChromePolicyJSON(`{
		"HomepageLocation": "https://example.com",
		"RestoreOnStartup": 4,
		"Nested": {"enabled": true}
	}`)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	want := `{"HomepageLocation":"https://example.com","Nested":{"enabled":true},"RestoreOnStartup":4}`
	if got != want {
		t.Fatalf("normalized JSON mismatch\ngot:  %s\nwant: %s", got, want)
	}

	gotAgain, diags := NormalizeChromePolicyJSON(`{"RestoreOnStartup":4,"Nested":{"enabled":true},"HomepageLocation":"https://example.com"}`)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotAgain != want {
		t.Fatalf("equivalent JSON did not normalize deterministically\ngot:  %s\nwant: %s", gotAgain, want)
	}
}

func TestChromePolicyJSONPreservesLargeNumbers(t *testing.T) {
	input := `{"LargeInteger":9007199254740993}`

	got, diags := NormalizeChromePolicyJSON(input)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got != input {
		t.Fatalf("normalized JSON mismatch\ngot:  %s\nwant: %s", got, input)
	}

	policy, diags := decodeChromePolicyJSON(input)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	number, ok := policy["LargeInteger"].(json.Number)
	if !ok {
		t.Fatalf("LargeInteger has type %T, want json.Number", policy["LargeInteger"])
	}
	if number.String() != "9007199254740993" {
		t.Fatalf("LargeInteger = %s, want exact JSON number", number.String())
	}
}

func TestNormalizeChromePolicyJSONRejectsInvalidAndNonObject(t *testing.T) {
	for _, input := range []string{
		`{`,
		`[]`,
		`"policy"`,
		`true`,
		`{"A":1}{"B":2}`,
	} {
		t.Run(input, func(t *testing.T) {
			if got, diags := NormalizeChromePolicyJSON(input); !diags.HasError() {
				t.Fatalf("NormalizeChromePolicyJSON(%q) = %q without error", input, got)
			}
		})
	}
}
