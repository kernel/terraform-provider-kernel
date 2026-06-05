package projectscope

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

// apiError builds a kernel.Error the way the SDK does: Error() formats the
// request and response, so a bare struct without them panics.
func apiError(status int) *kernel.Error {
	return &kernel.Error{
		StatusCode: status,
		Request: &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/browser_pools"},
		},
		Response: &http.Response{StatusCode: status},
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		attribute        types.String
		defaultProjectID string
		want             string
	}{
		"explicit attribute wins over default": {
			attribute:        types.StringValue("proj_attr"),
			defaultProjectID: "proj_default",
			want:             "proj_attr",
		},
		"null attribute falls back to default": {
			attribute:        types.StringNull(),
			defaultProjectID: "proj_default",
			want:             "proj_default",
		},
		"unknown attribute falls back to default": {
			attribute:        types.StringUnknown(),
			defaultProjectID: "proj_default",
			want:             "proj_default",
		},
		"nothing set resolves to unscoped": {
			attribute:        types.StringNull(),
			defaultProjectID: "",
			want:             "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := Resolve(test.attribute, test.defaultProjectID); got != test.want {
				t.Fatalf("Resolve() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStateValue(t *testing.T) {
	t.Parallel()

	if got := StateValue(""); !got.IsNull() {
		t.Fatalf("StateValue(\"\") = %v, want null", got)
	}
	if got := StateValue("proj_a"); !got.Equal(types.StringValue("proj_a")) {
		t.Fatalf("StateValue(proj_a) = %v, want proj_a", got)
	}
}

func TestAddError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		projectID    string
		err          error
		wantSummary  string
		wantInDetail string
	}{
		"scoped forbidden becomes project access diagnostic": {
			projectID:    "proj_a",
			err:          apiError(http.StatusForbidden),
			wantSummary:  "Kernel Project Access Denied",
			wantInDetail: "cannot access project proj_a",
		},
		"scoped forbidden keeps the underlying error": {
			projectID:    "proj_a",
			err:          apiError(http.StatusForbidden),
			wantSummary:  "Kernel Project Access Denied",
			wantInDetail: "Underlying error:",
		},
		"unscoped forbidden stays generic": {
			projectID:   "",
			err:         apiError(http.StatusForbidden),
			wantSummary: "Create Kernel Browser Pool",
		},
		"scoped non-forbidden stays generic": {
			projectID:   "proj_a",
			err:         errors.New("api failed"),
			wantSummary: "Create Kernel Browser Pool",
		},
		"scoped project_not_found becomes project diagnostic": {
			projectID:    "proj_a",
			err:          codedAPIError(http.StatusNotFound, "project_not_found"),
			wantSummary:  "Kernel Project Not Found",
			wantInDetail: "project proj_a was not found or is inactive",
		},
		"scoped resource not_found stays generic": {
			projectID:   "proj_a",
			err:         codedAPIError(http.StatusNotFound, "not_found"),
			wantSummary: "Create Kernel Browser Pool",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var diags diag.Diagnostics
			AddError(&diags, "Create Kernel Browser Pool", test.projectID, test.err)

			if got, want := len(diags.Errors()), 1; got != want {
				t.Fatalf("error count = %d, want %d", got, want)
			}
			d := diags.Errors()[0]
			if d.Summary() != test.wantSummary {
				t.Fatalf("summary = %q, want %q", d.Summary(), test.wantSummary)
			}
			if test.wantInDetail != "" && !strings.Contains(d.Detail(), test.wantInDetail) {
				t.Fatalf("detail %q does not contain %q", d.Detail(), test.wantInDetail)
			}
		})
	}
}

func codedAPIError(status int, code string) *kernel.Error {
	apiErr := apiError(status)
	raw := `{"code":"` + code + `","message":"test"}`
	if err := json.Unmarshal([]byte(raw), apiErr); err != nil {
		panic(err)
	}
	return apiErr
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want bool
	}{
		"coded resource not_found":          {codedAPIError(http.StatusNotFound, "not_found"), true},
		"project_not_found is not resource": {codedAPIError(http.StatusNotFound, "project_not_found"), false},
		"uncoded 404 is not resource":       {apiError(http.StatusNotFound), false},
		"coded 403 is not not-found":        {codedAPIError(http.StatusForbidden, "not_found"), false},
		"plain error is not not-found":      {errors.New("boom"), false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := IsNotFound(test.err); got != test.want {
				t.Fatalf("IsNotFound() = %v, want %v", got, test.want)
			}
		})
	}
}
