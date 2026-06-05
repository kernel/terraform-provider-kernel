package project

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ projectClient = kernelclient.Clients{}

type fakeProjectClient struct {
	defaultProjectID string
	get              func(context.Context, string) (*kernel.Project, error)
	list             func(context.Context, string, int64) (kernelclient.ProjectPage, error)
}

func (f fakeProjectClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeProjectClient) GetProject(ctx context.Context, id string) (*kernel.Project, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, id)
}

func (f fakeProjectClient) ListProjectPage(ctx context.Context, query string, offset int64) (kernelclient.ProjectPage, error) {
	if f.list == nil {
		return kernelclient.ProjectPage{}, errors.New("unexpected list")
	}
	return f.list(ctx, query, offset)
}

func TestDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()

	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_project" {
		t.Fatalf("TypeName = %q, want kernel_project", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "name", "status", "created_at", "updated_at"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s attribute", name)
		}
	}
}

func TestReadFallsBackToDefaultProjectID(t *testing.T) {
	t.Parallel()

	var gotID string
	ds := newDataSourceWithClient(fakeProjectClient{
		defaultProjectID: "project-current",
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			gotID = id
			project := projectForTest("project-current", "Current")
			return &project, nil
		},
	})

	state, diags := ds.read(context.Background(), projectModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotID != "project-current" {
		t.Fatalf("GetProject id = %q, want project-current", gotID)
	}
	if state.ID.ValueString() != "project-current" {
		t.Fatalf("state id = %q, want project-current", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Current" {
		t.Fatalf("state name = %q, want Current", state.Name.ValueString())
	}
}

func TestReadSetsTerraformState(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProjectClient{
		list: listProjectPages(t, "Target", map[int64]kernelclient.ProjectPage{
			0: projectPage(projectForTest("project-target", "Target")),
		}),
	})

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	req := datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    projectConfigValue(tftypes.NewValue(tftypes.String, nil), tftypes.NewValue(tftypes.String, "Target")),
		},
	}
	resp := datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}

	ds.Read(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state projectModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected state diagnostics: %v", resp.Diagnostics)
	}
	if state.ID.ValueString() != "project-target" {
		t.Fatalf("state id = %q, want project-target", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Target" {
		t.Fatalf("state name = %q, want Target", state.Name.ValueString())
	}
	if state.Status.ValueString() != string(kernel.ProjectStatusActive) {
		t.Fatalf("state status = %q, want active", state.Status.ValueString())
	}
}

func TestReadLooksUpExactProjectName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProjectClient{
		list: listProjectPages(t, "Target", map[int64]kernelclient.ProjectPage{
			0:   projectPageWithNext(100, projectForTest("project-other", "Other")),
			100: projectPage(projectForTest("project-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), projectModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "project-target" {
		t.Fatalf("state id = %q, want project-target", state.ID.ValueString())
	}
}

func TestReadRejectsAmbiguousProjectName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProjectClient{
		list: listProjectPages(t, "Target", map[int64]kernelclient.ProjectPage{
			0:   projectPageWithNext(100, projectForTest("project-a", "Target")),
			100: projectPage(projectForTest("project-b", "Target")),
		}),
	})

	_, diags := ds.read(context.Background(), projectModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for ambiguous project name")
	}
}

func TestReadRejectsInvalidProjectLookupCandidate(t *testing.T) {
	t.Parallel()

	invalid := projectForTest("project-invalid", "Target")
	invalid.Name = "123"
	invalid.JSON.Name = respjson.NewInvalidField("123")

	ds := newDataSourceWithClient(fakeProjectClient{
		list: listProjectPages(t, "Target", map[int64]kernelclient.ProjectPage{
			0: projectPage(invalid),
		}),
	})

	_, diags := ds.read(context.Background(), projectModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid project lookup candidate")
	}
}

func TestReadRejectsIncompleteProjectResponse(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProjectClient{
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			project := projectForTest("project-current", "Current")
			project.UpdatedAt = time.Time{}
			return &project, nil
		},
	})

	_, diags := ds.read(context.Background(), projectModel{ID: types.StringValue("project-current")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for incomplete project response")
	}
}

func TestReadRejectsInvalidProjectResponseField(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProjectClient{
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			project := projectForTest("project-current", "Current")
			project.ID = "123"
			project.JSON.ID = respjson.NewInvalidField("123")
			return &project, nil
		},
	})

	_, diags := ds.read(context.Background(), projectModel{ID: types.StringValue("project-current")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid project response field")
	}
}

func TestReadRejectsMissingProjectSelector(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProjectClient{})

	_, diags := ds.read(context.Background(), projectModel{})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing project selector")
	}
}

func TestReadRejectsEmptyProjectSelectors(t *testing.T) {
	t.Parallel()

	tests := map[string]projectModel{
		"empty id": {
			ID: types.StringValue(""),
		},
		"empty name with provider fallback": {
			Name: types.StringValue(""),
		},
	}

	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			called := false
			ds := newDataSourceWithClient(fakeProjectClient{
				defaultProjectID: "project-current",
				get: func(ctx context.Context, id string) (*kernel.Project, error) {
					called = true
					return nil, errors.New("should not read project")
				},
			})

			_, diags := ds.read(context.Background(), config)
			if !diags.HasError() {
				t.Fatal("expected diagnostics for empty selector")
			}
			if called {
				t.Fatal("GetProject was called for empty selector")
			}
		})
	}
}

func projectForTest(id, name string) kernel.Project {
	body := `{"id":"` + id + `","name":"` + name + `","status":"active","created_at":"2026-06-05T12:00:00Z","updated_at":"2026-06-05T12:00:00Z"}`

	var project kernel.Project
	if err := json.Unmarshal([]byte(body), &project); err != nil {
		panic(err)
	}
	return project
}

func projectConfigValue(id, name tftypes.Value) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"id":         tftypes.String,
				"name":       tftypes.String,
				"status":     tftypes.String,
				"created_at": tftypes.String,
				"updated_at": tftypes.String,
			},
		},
		map[string]tftypes.Value{
			"id":         id,
			"name":       name,
			"status":     tftypes.NewValue(tftypes.String, nil),
			"created_at": tftypes.NewValue(tftypes.String, nil),
			"updated_at": tftypes.NewValue(tftypes.String, nil),
		},
	)
}

func listProjectPages(t *testing.T, wantQuery string, pages map[int64]kernelclient.ProjectPage) func(context.Context, string, int64) (kernelclient.ProjectPage, error) {
	t.Helper()

	return func(ctx context.Context, query string, offset int64) (kernelclient.ProjectPage, error) {
		if query != wantQuery {
			t.Fatalf("ListProjectPage query = %q, want %q", query, wantQuery)
		}

		page, ok := pages[offset]
		if !ok {
			t.Fatalf("unexpected project page offset: %d", offset)
		}
		return page, nil
	}
}

func projectPage(projects ...kernel.Project) kernelclient.ProjectPage {
	return kernelclient.ProjectPage{Items: projects}
}

func projectPageWithNext(nextOffset int64, projects ...kernel.Project) kernelclient.ProjectPage {
	return kernelclient.ProjectPage{
		Items:       projects,
		NextOffset:  nextOffset,
		HasNextPage: true,
	}
}
