package browserpool

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func TestNormalizeChromePolicyJSON(t *testing.T) {
	got, diags := normalizeChromePolicyJSON(`{
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

	gotAgain, diags := normalizeChromePolicyJSON(`{"RestoreOnStartup":4,"Nested":{"enabled":true},"HomepageLocation":"https://example.com"}`)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotAgain != want {
		t.Fatalf("equivalent JSON did not normalize deterministically\ngot:  %s\nwant: %s", gotAgain, want)
	}
}

func TestChromePolicyJSONPreservesLargeNumbers(t *testing.T) {
	input := `{"LargeInteger":9007199254740993}`

	got, diags := normalizeChromePolicyJSON(input)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got != input {
		t.Fatalf("normalized JSON mismatch\ngot:  %s\nwant: %s", got, input)
	}
}

func TestChromePolicyTypeNormalizesKnownValue(t *testing.T) {
	got, diags := chromePolicyType{}.ValueFromString(context.Background(), basetypes.NewStringValue(`{
		"B": 2,
		"A": 1
	}`))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	stringValue, diags := got.ToStringValue(context.Background())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if stringValue.ValueString() != `{"A":1,"B":2}` {
		t.Fatalf("normalized value = %q, want canonical JSON object", stringValue.ValueString())
	}
}

func TestChromePolicyValueSemanticEquals(t *testing.T) {
	left := chromePolicyValue{StringValue: basetypes.NewStringValue(`{"B":2,"A":1}`)}
	right := basetypes.NewStringValue(`{
		"A": 1,
		"B": 2
	}`)

	equal, diags := left.StringSemanticEquals(context.Background(), right)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !equal {
		t.Fatal("expected semantically equivalent JSON objects to compare equal")
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
			if got, diags := normalizeChromePolicyJSON(input); !diags.HasError() {
				t.Fatalf("normalizeChromePolicyJSON(%q) = %q without error", input, got)
			}
		})
	}
}
