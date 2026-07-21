package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ appClient = kernelclient.Clients{}

type fakeAppClient struct {
	defaultProjectID string
	list             func(context.Context, string, string, string, int64) (kernelclient.AppPage, error)
}

func (f fakeAppClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeAppClient) ListAppPage(ctx context.Context, projectID, appName, version string, offset int64) (kernelclient.AppPage, error) {
	if f.list == nil {
		return kernelclient.AppPage{}, errors.New("unexpected app list")
	}
	return f.list(ctx, projectID, appName, version, offset)
}

func TestDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()
	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_app" {
		t.Fatalf("TypeName = %q, want kernel_app", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "app_name", "version", "project_id", "deployment_id", "region", "actions", "env_var_keys"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s", name)
		}
	}
	for _, name := range []string{"env_vars", "input_schema", "output_schema", "status", "logs"} {
		if _, ok := schema.Schema.Attributes[name]; ok {
			t.Fatalf("schema must not expose %s", name)
		}
	}
}

func TestReadSetsTerraformStateWithoutSecretValues(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeAppClient{
		list: listAppPages(t, "demo", "v1", map[int64]kernelclient.AppPage{
			0: appPage(appForTest(t, "app-version-1", "demo", "v1")),
		}),
	})

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	req := datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    appConfigValue("demo", "v1"),
		},
	}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	ds.Read(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state appModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected state diagnostics: %v", resp.Diagnostics)
	}
	if state.ID.ValueString() != "app-version-1" || state.DeploymentID.ValueString() != "deployment-1" {
		t.Fatalf("state ids = %q/%q", state.ID.ValueString(), state.DeploymentID.ValueString())
	}
	assertStringSet(t, state.Actions, []string{"health", "run"})
	assertStringSet(t, state.EnvVarKeys, []string{"API_TOKEN", "REGION"})
}

func TestReadResolvesProjectScope(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configProjectID  types.String
		defaultProjectID string
		wantProjectID    string
	}{
		"explicit wins": {
			configProjectID:  types.StringValue("project_explicit"),
			defaultProjectID: "project_default",
			wantProjectID:    "project_explicit",
		},
		"provider default": {
			configProjectID:  types.StringNull(),
			defaultProjectID: "project_default",
			wantProjectID:    "project_default",
		},
		"api key binding": {
			configProjectID: types.StringNull(),
			wantProjectID:   "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var gotProjectID string
			ds := newDataSourceWithClient(fakeAppClient{
				defaultProjectID: test.defaultProjectID,
				list: func(ctx context.Context, projectID, appName, version string, offset int64) (kernelclient.AppPage, error) {
					gotProjectID = projectID
					return appPage(appForTest(t, "app-version-1", appName, version)), nil
				},
			})
			state, diags := ds.read(context.Background(), appModel{
				AppName:   types.StringValue("demo"),
				Version:   types.StringValue("v1"),
				ProjectID: test.configProjectID,
			})
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if gotProjectID != test.wantProjectID {
				t.Fatalf("project = %q, want %q", gotProjectID, test.wantProjectID)
			}
			if !state.ProjectID.Equal(test.configProjectID) {
				t.Fatalf("state project_id = %v, want %v", state.ProjectID, test.configProjectID)
			}
		})
	}
}

func TestReadScansPagesAndDeduplicatesByID(t *testing.T) {
	t.Parallel()

	target := appForTest(t, "app-version-1", "demo", "v1")
	ds := newDataSourceWithClient(fakeAppClient{
		list: listAppPages(t, "demo", "v1", map[int64]kernelclient.AppPage{
			0:   appPageWithNext(100, appForTest(t, "other", "other", "v1"), target),
			100: appPage(target),
		}),
	})

	state, diags := ds.read(context.Background(), validConfig())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "app-version-1" {
		t.Fatalf("id = %q, want app-version-1", state.ID.ValueString())
	}
}

func TestReadDiagnosesMissingAndAmbiguousMatches(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pages map[int64]kernelclient.AppPage
		want  string
	}{
		"missing": {
			pages: map[int64]kernelclient.AppPage{0: appPage()},
			want:  "No running Kernel app",
		},
		"ambiguous": {
			pages: map[int64]kernelclient.AppPage{0: appPage(
				appForTest(t, "app-version-1", "demo", "v1"),
				appForTest(t, "app-version-2", "demo", "v1"),
			)},
			want: "multiple running Kernel apps",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ds := newDataSourceWithClient(fakeAppClient{list: listAppPages(t, "demo", "v1", test.pages)})
			_, diags := ds.read(context.Background(), validConfig())
			if !diags.HasError() || !containsDiagnostic(diags, test.want) {
				t.Fatalf("diagnostics = %v, want %q", diags, test.want)
			}
		})
	}
}

func TestFlattenAppRejectsMalformedRequiredFields(t *testing.T) {
	t.Parallel()

	valid := map[string]any{
		"id":         "app-version-1",
		"app_name":   "demo",
		"version":    "v1",
		"region":     "aws.us-east-1a",
		"deployment": "deployment-1",
		"actions":    []any{map[string]any{"name": "run"}},
		"env_vars":   map[string]string{"TOKEN": ""},
	}

	for _, field := range []string{"id", "app_name", "version", "region", "deployment", "actions", "env_vars"} {
		field := field
		t.Run("missing "+field, func(t *testing.T) {
			t.Parallel()
			payload := cloneMap(valid)
			delete(payload, field)
			_, diags := flattenApp(context.Background(), appFromPayload(t, payload))
			if !diags.HasError() {
				t.Fatalf("expected diagnostics for missing %s", field)
			}
		})
	}

	t.Run("duplicate action names", func(t *testing.T) {
		t.Parallel()
		payload := cloneMap(valid)
		payload["actions"] = []any{map[string]any{"name": "run"}, map[string]any{"name": "run"}}
		_, diags := flattenApp(context.Background(), appFromPayload(t, payload))
		if !diags.HasError() {
			t.Fatal("expected diagnostics for duplicate action names")
		}
	})

	t.Run("null environment variables", func(t *testing.T) {
		t.Parallel()
		payload := cloneMap(valid)
		payload["env_vars"] = nil
		state, diags := flattenApp(context.Background(), appFromPayload(t, payload))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		assertStringSet(t, state.EnvVarKeys, nil)
	})
}

func TestReadRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeAppClient{})
	for name, config := range map[string]appModel{
		"missing app name": {AppName: types.StringNull(), Version: types.StringValue("v1")},
		"unknown version":  {AppName: types.StringValue("demo"), Version: types.StringUnknown()},
		"unknown project":  {AppName: types.StringValue("demo"), Version: types.StringValue("v1"), ProjectID: types.StringUnknown()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := ds.read(context.Background(), config)
			if !diags.HasError() {
				t.Fatal("expected diagnostics")
			}
		})
	}
}

func validConfig() appModel {
	return appModel{AppName: types.StringValue("demo"), Version: types.StringValue("v1"), ProjectID: types.StringNull()}
}

func appForTest(t *testing.T, id, appName, version string) kernel.AppListResponse {
	t.Helper()
	return appFromPayload(t, map[string]any{
		"id":         id,
		"app_name":   appName,
		"version":    version,
		"region":     "aws.us-east-1a",
		"deployment": "deployment-1",
		"actions": []any{
			map[string]any{"name": "run", "input_schema": map[string]any{"type": "object"}, "output_schema": nil},
			map[string]any{"name": "health", "input_schema": nil, "output_schema": map[string]any{"type": "boolean"}},
		},
		"env_vars": map[string]string{"API_TOKEN": "must-not-enter-state", "REGION": "us-east"},
	})
}

func appFromPayload(t *testing.T, payload map[string]any) kernel.AppListResponse {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal app: %v", err)
	}
	var app kernel.AppListResponse
	if err := json.Unmarshal(raw, &app); err != nil {
		t.Fatalf("unmarshal app: %v", err)
	}
	return app
}

func cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func listAppPages(t *testing.T, appName, version string, pages map[int64]kernelclient.AppPage) func(context.Context, string, string, string, int64) (kernelclient.AppPage, error) {
	t.Helper()
	return func(ctx context.Context, projectID, gotName, gotVersion string, offset int64) (kernelclient.AppPage, error) {
		if gotName != appName || gotVersion != version {
			t.Fatalf("lookup = %q/%q, want %q/%q", gotName, gotVersion, appName, version)
		}
		page, ok := pages[offset]
		if !ok {
			t.Fatalf("unexpected app page offset %d", offset)
		}
		return page, nil
	}
}

func appPage(apps ...kernel.AppListResponse) kernelclient.AppPage {
	return kernelclient.AppPage{Items: apps}
}

func appPageWithNext(next int64, apps ...kernel.AppListResponse) kernelclient.AppPage {
	return kernelclient.AppPage{Items: apps, NextOffset: next, HasNextPage: true}
}

func appConfigValue(appName, version string) tftypes.Value {
	setType := tftypes.Set{ElementType: tftypes.String}
	return tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"id":            tftypes.String,
			"app_name":      tftypes.String,
			"version":       tftypes.String,
			"project_id":    tftypes.String,
			"deployment_id": tftypes.String,
			"region":        tftypes.String,
			"actions":       setType,
			"env_var_keys":  setType,
		}},
		map[string]tftypes.Value{
			"id":            tftypes.NewValue(tftypes.String, nil),
			"app_name":      tftypes.NewValue(tftypes.String, appName),
			"version":       tftypes.NewValue(tftypes.String, version),
			"project_id":    tftypes.NewValue(tftypes.String, nil),
			"deployment_id": tftypes.NewValue(tftypes.String, nil),
			"region":        tftypes.NewValue(tftypes.String, nil),
			"actions":       tftypes.NewValue(setType, nil),
			"env_var_keys":  tftypes.NewValue(setType, nil),
		},
	)
}

func assertStringSet(t *testing.T, set types.Set, want []string) {
	t.Helper()
	var got []string
	diags := set.ElementsAs(context.Background(), &got, false)
	if diags.HasError() {
		t.Fatalf("decode set: %v", diags)
	}
	if len(got) != len(want) {
		t.Fatalf("set = %v, want %v", got, want)
	}
	wanted := make(map[string]bool, len(want))
	for _, value := range want {
		wanted[value] = true
	}
	for _, value := range got {
		if !wanted[value] {
			t.Fatalf("set = %v, want %v", got, want)
		}
	}
}

func containsDiagnostic(diags diag.Diagnostics, text string) bool {
	for _, diagnostic := range diags {
		if strings.Contains(diagnostic.Detail(), text) {
			return true
		}
	}
	return false
}
