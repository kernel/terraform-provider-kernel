package browserpool

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

func TestNormalizeChromePolicyJSONDoesNotHTMLEscape(t *testing.T) {
	got, diags := normalizeChromePolicyJSON(`{"URLAllowlist":"https://a.com?x=1&y=2","Tag":"<b>"}`)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	want := `{"Tag":"<b>","URLAllowlist":"https://a.com?x=1&y=2"}`
	if got != want {
		t.Fatalf("normalized JSON HTML-escaped <, >, or &\ngot:  %s\nwant: %s", got, want)
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

// ValueFromString stores the chrome_policy string verbatim; canonicalization is
// handled by semantic equality, not by rewriting state.
func TestChromePolicyTypeStoresRawValue(t *testing.T) {
	raw := `{
		"B": 2,
		"A": 1
	}`
	got, diags := chromePolicyType{}.ValueFromString(context.Background(), basetypes.NewStringValue(raw))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	stringValue, diags := got.ToStringValue(context.Background())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if stringValue.ValueString() != raw {
		t.Fatalf("ValueFromString stored %q, want the raw input unchanged", stringValue.ValueString())
	}
}

// ValueFromTerraform must never return a Go error for malformed chrome_policy
// content: such an error surfaces as the framework's "report this to the
// provider developer" diagnostic and pre-empts the validator. Bad JSON is a
// user error reported by chromePolicyJSONValidator, so conversion must succeed
// and store the raw string regardless of content.
func TestChromePolicyTypeValueFromTerraformToleratesAnyString(t *testing.T) {
	for _, input := range []string{`{not json`, `[]`, `"scalar"`, `{"A":1}`} {
		t.Run(input, func(t *testing.T) {
			got, err := chromePolicyType{}.ValueFromTerraform(context.Background(), tftypes.NewValue(tftypes.String, input))
			if err != nil {
				t.Fatalf("ValueFromTerraform(%q) returned a conversion error (must defer to the validator): %v", input, err)
			}
			value, ok := got.(chromePolicyValue)
			if !ok {
				t.Fatalf("ValueFromTerraform(%q) = %T, want chromePolicyValue", input, got)
			}
			if value.ValueString() != input {
				t.Fatalf("ValueFromTerraform(%q) stored %q, want the raw string", input, value.ValueString())
			}
		})
	}
}

func TestChromePolicyValueSemanticEquals(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		want        bool
	}{
		{"reordered keys and whitespace are equal", `{"B":2,"A":1}`, "{\n\t\"A\": 1,\n\t\"B\": 2\n}", true},
		{"different value not equal", `{"A":1}`, `{"A":2}`, false},
		{"different key not equal", `{"A":1}`, `{"B":1}`, false},
		{"invalid left treated as not equal", `{not json`, `{"A":1}`, false},
		{"invalid right treated as not equal", `{"A":1}`, `not json`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := chromePolicyValue{StringValue: basetypes.NewStringValue(tt.left)}
			equal, diags := left.StringSemanticEquals(context.Background(), basetypes.NewStringValue(tt.right))
			if diags.HasError() {
				t.Fatalf("StringSemanticEquals raised diagnostics (it must never error): %v", diags)
			}
			if equal != tt.want {
				t.Fatalf("StringSemanticEquals(%q, %q) = %v, want %v", tt.left, tt.right, equal, tt.want)
			}
		})
	}
}

func TestChromePolicyValueEqual(t *testing.T) {
	a := chromePolicyValue{StringValue: basetypes.NewStringValue(`{"A":1}`)}

	if !a.Equal(chromePolicyValue{StringValue: basetypes.NewStringValue(`{"A":1}`)}) {
		t.Fatal("identical raw values should be Equal")
	}
	// Equal is literal, not semantic: reordered-but-equivalent JSON is NOT Equal
	// (that distinction is what StringSemanticEquals exists for).
	if a.Equal(chromePolicyValue{StringValue: basetypes.NewStringValue(`{"A":2}`)}) {
		t.Fatal("different raw values should not be Equal")
	}
	if a.Equal(basetypes.NewStringValue(`{"A":1}`)) {
		t.Fatal("a plain StringValue should not be Equal to a chromePolicyValue")
	}
}

func TestNormalizeChromePolicyJSONRejectsInvalidAndNonObject(t *testing.T) {
	for _, input := range []string{
		`{`,
		`[]`,
		`"policy"`,
		`true`,
		`null`,
		`{"A":1}{"B":2}`,
	} {
		t.Run(input, func(t *testing.T) {
			if got, diags := normalizeChromePolicyJSON(input); !diags.HasError() {
				t.Fatalf("normalizeChromePolicyJSON(%q) = %q without error", input, got)
			}
		})
	}
}
