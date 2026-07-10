package project

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ projectClient = kernelclient.Clients{}

type fakeProjectClient struct {
	create func(context.Context, kernel.ProjectNewParams) (*kernel.Project, error)
	get    func(context.Context, string) (*kernel.Project, error)
	update func(context.Context, string, kernel.ProjectUpdateParams) (*kernel.Project, error)
	delete func(context.Context, string) error
}

func (f fakeProjectClient) CreateProject(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
	if f.create == nil {
		return nil, errors.New("unexpected create")
	}
	return f.create(ctx, params)
}

func (f fakeProjectClient) GetProject(ctx context.Context, id string) (*kernel.Project, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, id)
}

func (f fakeProjectClient) UpdateProject(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
	if f.update == nil {
		return nil, errors.New("unexpected update")
	}
	return f.update(ctx, id, params)
}

func (f fakeProjectClient) DeleteProject(ctx context.Context, id string) error {
	if f.delete == nil {
		return errors.New("unexpected delete")
	}
	return f.delete(ctx, id)
}

func TestCreateProjectCreatesAndFlattensState(t *testing.T) {
	t.Parallel()

	var gotParams kernel.ProjectNewParams
	r := newResourceWithClient(fakeProjectClient{
		create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
			gotParams = params
			project := projectForTest(t, `{"id":"project_123","name":"Project"}`)
			return &project, nil
		},
	})

	result, diags := r.create(context.Background(), projectModel{Name: types.StringValue("Project")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result.Status != projectCreateSucceeded {
		t.Fatalf("create status = %v, want succeeded", result.Status)
	}
	if got, want := gotParams.CreateProjectRequest.Name, "Project"; got != want {
		t.Fatalf("create name = %q, want %q", got, want)
	}
	if got, want := result.State.ID.ValueString(), "project_123"; got != want {
		t.Fatalf("state id = %q, want %q", got, want)
	}
	if got, want := result.State.Name.ValueString(), "Project"; got != want {
		t.Fatalf("state name = %q, want %q", got, want)
	}
}

func TestCreateProjectReturnsDiagnostics(t *testing.T) {
	t.Parallel()

	conflictError := projectAPIError(t, http.StatusConflict, `{}`)
	malformedSuccess := projectForTest(t, `{"id":"project_123"}`)
	mismatchedSuccess := projectForTest(t, `{"id":"project_123","name":"Different"}`)
	tests := map[string]struct {
		resource    *projectResource
		wantStatus  projectCreateStatus
		wantSummary string
		wantDetail  string
		wantID      string
		wantName    string
	}{
		"missing client": {
			resource:    &projectResource{},
			wantStatus:  projectCreateFailed,
			wantSummary: "Missing Kernel Client",
			wantDetail:  "The Kernel provider was not configured before using the project resource.",
		},
		"client error response": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return nil, conflictError
				},
			}),
			wantStatus:  projectCreateFailed,
			wantSummary: "Create Kernel Project",
			wantDetail:  "409 Conflict",
		},
		"transport error after possible commit": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return nil, errors.New("response timeout")
				},
			}),
			wantStatus:  projectCreateUncertain,
			wantSummary: "Kernel Project Creation Outcome Uncertain",
			wantDetail:  "response timeout",
		},
		"empty success response": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return nil, nil
				},
			}),
			wantStatus:  projectCreateUncertain,
			wantSummary: "Kernel Project Creation Outcome Uncertain",
			wantDetail:  "Kernel returned an empty project response.",
		},
		"malformed success preserves valid id": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return &malformedSuccess, nil
				},
			}),
			wantStatus:  projectCreateUncertain,
			wantSummary: "Kernel Project Creation Outcome Uncertain",
			wantDetail:  "missing or invalid field name",
			wantID:      "project_123",
			wantName:    "Project",
		},
		"mismatched success name preserves planned name": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return &mismatchedSuccess, nil
				},
			}),
			wantStatus:  projectCreateUncertain,
			wantSummary: "Kernel Project Creation Outcome Uncertain",
			wantDetail:  `Kernel returned project name "Different" instead of the requested name.`,
			wantID:      "project_123",
			wantName:    "Project",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, diags := test.resource.create(context.Background(), projectModel{Name: types.StringValue("Project")})
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %v, want one error", diags)
			}
			if result.Status != test.wantStatus {
				t.Fatalf("create status = %v, want %v", result.Status, test.wantStatus)
			}
			if got := diags[0].Summary(); got != test.wantSummary {
				t.Fatalf("diagnostic summary = %q, want %q", got, test.wantSummary)
			}
			detail := diags[0].Detail()
			if !strings.Contains(detail, test.wantDetail) {
				t.Fatalf("diagnostic detail = %q, want it to contain %q", detail, test.wantDetail)
			}
			if test.wantStatus == projectCreateUncertain && !strings.Contains(detail, "import its project ID before applying again") {
				t.Fatalf("uncertain diagnostic detail = %q, want import guidance", detail)
			}
			if got := result.State.ID.ValueString(); got != test.wantID {
				t.Fatalf("state id = %q, want %q", got, test.wantID)
			}
			if got := result.State.Name.ValueString(); got != test.wantName {
				t.Fatalf("state name = %q, want %q", got, test.wantName)
			}
		})
	}
}

func TestCreateProjectReportsEveryMalformedResponseField(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
			project := projectForTest(t, `{}`)
			return &project, nil
		},
	})

	result, diags := r.create(context.Background(), projectModel{Name: types.StringValue("Project")})
	if result.Status != projectCreateUncertain {
		t.Fatalf("create status = %v, want uncertain", result.Status)
	}
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one uncertain-outcome error", diags)
	}
	for _, reason := range []string{
		"missing or invalid field id",
		"missing or invalid field name",
	} {
		if detail := diags[0].Detail(); !strings.Contains(detail, reason) {
			t.Fatalf("diagnostic detail = %q, want it to contain %q", detail, reason)
		}
	}
}

func projectAPIError(t *testing.T, status int, raw string) *kernel.Error {
	t.Helper()

	var apiError kernel.Error
	if err := json.Unmarshal([]byte(raw), &apiError); err != nil {
		t.Fatalf("unmarshal API error: %v", err)
	}
	apiError.StatusCode = status
	apiError.Request = &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/org/projects"},
	}
	apiError.Response = &http.Response{StatusCode: status, Status: http.StatusText(status)}
	return &apiError
}
