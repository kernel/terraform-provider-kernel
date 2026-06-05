package datasources

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func hasDiagnosticSummary(diags diag.Diagnostics, want string) bool {
	for _, diagnostic := range diags {
		if diagnostic.Summary() == want {
			return true
		}
	}
	return false
}

func TestResolveIDNameSelector(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		id      types.String
		name    types.String
		want    IDNameSelector
		wantErr string
	}{
		"neither set falls through to the provider default": {
			id:   types.StringNull(),
			name: types.StringNull(),
			want: IDNameSelector{},
		},
		"id only": {
			id:   types.StringValue("proj-1"),
			name: types.StringNull(),
			want: IDNameSelector{HasID: true},
		},
		"name only": {
			id:   types.StringNull(),
			name: types.StringValue("Production"),
			want: IDNameSelector{HasName: true},
		},
		"both set conflict": {
			id:      types.StringValue("proj-1"),
			name:    types.StringValue("Production"),
			wantErr: "Conflicting Project Selectors",
		},
		"unknown id": {
			id:      types.StringUnknown(),
			name:    types.StringNull(),
			wantErr: "Unknown Project Selector",
		},
		"unknown name": {
			id:      types.StringNull(),
			name:    types.StringUnknown(),
			wantErr: "Unknown Project Selector",
		},
		"empty id": {
			id:      types.StringValue(""),
			name:    types.StringNull(),
			wantErr: "Empty Project ID",
		},
		"empty name": {
			id:      types.StringNull(),
			name:    types.StringValue(""),
			wantErr: "Empty Project Name",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			selector, diags := ResolveIDNameSelector("Project", "kernel_project", test.id, test.name)
			if test.wantErr != "" {
				if !diags.HasError() {
					t.Fatalf("expected diagnostics containing %q", test.wantErr)
				}
				if !hasDiagnosticSummary(diags.Errors(), test.wantErr) {
					t.Fatalf("diagnostics = %v, want summary %q", diags, test.wantErr)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if selector != test.want {
				t.Fatalf("selector = %+v, want %+v", selector, test.want)
			}
		})
	}
}

func TestFieldPresent(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw  string
		want bool
	}{
		"absent":                  {raw: "", want: false},
		"json null":               {raw: "null", want: false},
		"json null with padding":  {raw: "  null ", want: false},
		"string value":            {raw: `"x"`, want: true},
		"non-null non-string raw": {raw: "123", want: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := FieldPresent(test.raw); got != test.want {
				t.Fatalf("FieldPresent(%q) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestValidResponseString(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw   string
		valid bool
		value string
		want  bool
	}{
		"raw matches decoded value":  {raw: `"proj-1"`, valid: true, value: "proj-1", want: true},
		"absent raw":                 {raw: "", valid: true, value: "proj-1", want: false},
		"json null raw":              {raw: "null", valid: true, value: "proj-1", want: false},
		"invalid field flag":         {raw: `"proj-1"`, valid: false, value: "proj-1", want: false},
		"empty decoded value":        {raw: `""`, valid: true, value: "", want: false},
		"wrong json type":            {raw: "123", valid: true, value: "123", want: false},
		"raw disagrees with decoded": {raw: `"proj-1"`, valid: true, value: "proj-2", want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := ValidResponseString(test.raw, test.valid, test.value); got != test.want {
				t.Fatalf("ValidResponseString(%q, %v, %q) = %v, want %v", test.raw, test.valid, test.value, got, test.want)
			}
		})
	}
}

func TestValidResponseTime(t *testing.T) {
	t.Parallel()

	stamp := time.Date(2026, time.June, 5, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		raw   string
		valid bool
		value time.Time
		want  bool
	}{
		"raw matches decoded value":  {raw: `"2026-06-05T12:00:00Z"`, valid: true, value: stamp, want: true},
		"absent raw":                 {raw: "", valid: true, value: stamp, want: false},
		"json null raw":              {raw: "null", valid: true, value: stamp, want: false},
		"invalid field flag":         {raw: `"2026-06-05T12:00:00Z"`, valid: false, value: stamp, want: false},
		"zero decoded value":         {raw: `"2026-06-05T12:00:00Z"`, valid: true, value: time.Time{}, want: false},
		"non-timestamp raw":          {raw: "123", valid: true, value: stamp, want: false},
		"raw disagrees with decoded": {raw: `"2026-06-05T12:00:00Z"`, valid: true, value: stamp.Add(time.Second), want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := ValidResponseTime(test.raw, test.valid, test.value); got != test.want {
				t.Fatalf("ValidResponseTime(%q, %v, %v) = %v, want %v", test.raw, test.valid, test.value, got, test.want)
			}
		})
	}
}
