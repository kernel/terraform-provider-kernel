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
	if result.UncertainErr != nil {
		t.Fatalf("uncertain error = %v, want successful create", result.UncertainErr)
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
		resource      *projectResource
		wantSummary   string
		wantDetail    string
		wantUncertain bool
		wantReason    string
		wantID        string
		wantName      string
	}{
		"missing client": {
			resource:    &projectResource{},
			wantSummary: "Missing Kernel Client",
			wantDetail:  "The Kernel provider was not configured before using the project resource.",
		},
		"client error response": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return nil, conflictError
				},
			}),
			wantSummary: "Create Kernel Project",
			wantDetail:  "409 Conflict",
		},
		"transport error after possible commit": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return nil, errors.New("response timeout")
				},
			}),
			wantUncertain: true,
			wantReason:    "response timeout",
		},
		"transport error without detail": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return nil, errors.New("")
				},
			}),
			wantUncertain: true,
		},
		"empty success response": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return nil, nil
				},
			}),
			wantUncertain: true,
			wantReason:    "Kernel returned an empty project response.",
		},
		"malformed success preserves valid id": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return &malformedSuccess, nil
				},
			}),
			wantUncertain: true,
			wantReason:    "missing or invalid field name",
			wantID:        "project_123",
			wantName:      "Project",
		},
		"mismatched success name preserves planned name": {
			resource: newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return &mismatchedSuccess, nil
				},
			}),
			wantUncertain: true,
			wantReason:    `Kernel returned project name "Different" instead of the requested name.`,
			wantID:        "project_123",
			wantName:      "Project",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, diags := test.resource.create(context.Background(), projectModel{Name: types.StringValue("Project")})
			if test.wantSummary != "" {
				if len(diags) != 1 {
					t.Fatalf("diagnostics = %v, want one error", diags)
				}
				if got := diags[0].Summary(); got != test.wantSummary {
					t.Fatalf("diagnostic summary = %q, want %q", got, test.wantSummary)
				}
				if detail := diags[0].Detail(); !strings.Contains(detail, test.wantDetail) {
					t.Fatalf("diagnostic detail = %q, want it to contain %q", detail, test.wantDetail)
				}
			} else if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got := result.UncertainErr != nil; got != test.wantUncertain {
				t.Fatalf("uncertain = %t, want %t (error: %v)", got, test.wantUncertain, result.UncertainErr)
			}
			if test.wantReason != "" && (result.UncertainErr == nil || !strings.Contains(result.UncertainErr.Error(), test.wantReason)) {
				t.Fatalf("uncertain error = %v, want it to contain %q", result.UncertainErr, test.wantReason)
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
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	for _, reason := range []string{
		"missing or invalid field id",
		"missing or invalid field name",
	} {
		if result.UncertainErr == nil || !strings.Contains(result.UncertainErr.Error(), reason) {
			t.Fatalf("uncertain error = %v, want it to contain %q", result.UncertainErr, reason)
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
