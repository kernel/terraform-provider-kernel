package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ deploymentClient = kernelclient.Clients{}

type fakeDeploymentClient struct {
	defaultProjectID string
	get              func(context.Context, string, string) (*kernel.DeploymentGetResponse, error)
}

func (f fakeDeploymentClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeDeploymentClient) GetDeployment(ctx context.Context, projectID, id string) (*kernel.DeploymentGetResponse, error) {
	if f.get == nil {
		return nil, errors.New("unexpected deployment get")
	}
	return f.get(ctx, projectID, id)
}

func TestDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()
	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_deployment" {
		t.Fatalf("TypeName = %q, want kernel_deployment", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "project_id", "entrypoint_rel_path", "region", "status", "status_reason", "env_var_keys", "created_at", "updated_at"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s", name)
		}
	}
	for _, name := range []string{"env_vars", "source", "logs", "events", "app_name", "version", "actions"} {
		if _, ok := schema.Schema.Attributes[name]; ok {
			t.Fatalf("schema must not expose %s", name)
		}
	}
}

func TestReadResolvesProjectScopeAndOmitsSecretValues(t *testing.T) {
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
			ds := newDataSourceWithClient(fakeDeploymentClient{
				defaultProjectID: test.defaultProjectID,
				get: func(ctx context.Context, projectID, id string) (*kernel.DeploymentGetResponse, error) {
					gotProjectID = projectID
					deployment := deploymentForTest(t, id)
					return &deployment, nil
				},
			})

			state, diags := ds.read(context.Background(), deploymentModel{
				ID:        types.StringValue("deployment_123"),
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
			assertStringSet(t, state.EnvVarKeys, []string{"API_TOKEN", "REGION"})
		})
	}
}

func TestReadRejectsInvalidIDAndMismatchedResponse(t *testing.T) {
	t.Parallel()

	for name, id := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
		"empty":   types.StringValue(""),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ds := newDataSourceWithClient(fakeDeploymentClient{})
			_, diags := ds.read(context.Background(), deploymentModel{ID: id, ProjectID: types.StringNull()})
			if !diags.HasError() || !diagnosticsContain(diags, "known, non-empty") {
				t.Fatalf("diagnostics = %v, want invalid id", diags)
			}
		})
	}

	ds := newDataSourceWithClient(fakeDeploymentClient{
		get: func(context.Context, string, string) (*kernel.DeploymentGetResponse, error) {
			deployment := deploymentForTest(t, "deployment_other")
			return &deployment, nil
		},
	})
	_, diags := ds.read(context.Background(), validConfig())
	if !diags.HasError() || !diagnosticsContain(diags, "ID Mismatch") {
		t.Fatalf("diagnostics = %v, want id mismatch", diags)
	}
}

func TestReadDiagnosesClientAndResponseFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		client deploymentClient
		want   string
	}{
		"missing client": {
			client: nil,
			want:   "Missing Kernel Client",
		},
		"client error": {
			client: fakeDeploymentClient{get: func(context.Context, string, string) (*kernel.DeploymentGetResponse, error) {
				return nil, errors.New("read failed")
			}},
			want: "read failed",
		},
		"empty response": {
			client: fakeDeploymentClient{get: func(context.Context, string, string) (*kernel.DeploymentGetResponse, error) {
				return nil, nil
			}},
			want: "empty deployment response",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ds := newDataSourceWithClient(test.client)
			_, diags := ds.read(context.Background(), validConfig())
			if !diags.HasError() || !diagnosticsContain(diags, test.want) {
				t.Fatalf("diagnostics = %v, want %q", diags, test.want)
			}
		})
	}
}

func TestFlattenDeploymentRejectsMalformedRequiredFields(t *testing.T) {
	t.Parallel()

	valid := map[string]any{
		"id":                  "deployment_123",
		"created_at":          "2026-07-11T12:00:00Z",
		"updated_at":          "2026-07-11T12:01:00Z",
		"region":              "aws.us-east-1a",
		"status":              "running",
		"entrypoint_rel_path": "src/index.ts",
		"status_reason":       "ready",
		"env_vars":            map[string]string{"TOKEN": ""},
	}

	for _, field := range []string{"id", "created_at", "region", "status"} {
		t.Run("missing "+field, func(t *testing.T) {
			t.Parallel()
			payload := cloneMap(valid)
			delete(payload, field)
			_, diags := flattenDeployment(context.Background(), deploymentFromPayload(t, payload))
			if !diags.HasError() {
				t.Fatalf("expected diagnostics for missing %s", field)
			}
		})
	}

	t.Run("invalid updated_at", func(t *testing.T) {
		t.Parallel()
		deployment := deploymentFromPayload(t, valid)
		deployment.UpdatedAt = deployment.CreatedAt
		deployment.JSON.UpdatedAt = respjson.NewInvalidField("invalid")
		_, diags := flattenDeployment(context.Background(), deployment)
		if !diags.HasError() || !diagnosticsContain(diags, "updated_at") {
			t.Fatalf("diagnostics = %v, want invalid updated_at", diags)
		}
	})

	t.Run("invalid env_vars", func(t *testing.T) {
		t.Parallel()
		deployment := deploymentFromPayload(t, valid)
		deployment.EnvVars = nil
		deployment.JSON.EnvVars = respjson.NewInvalidField(`{"TOKEN":123}`)
		_, diags := flattenDeployment(context.Background(), deployment)
		if !diags.HasError() || !diagnosticsContain(diags, "env_vars") {
			t.Fatalf("diagnostics = %v, want invalid env_vars", diags)
		}
	})

	t.Run("mismatched env_vars keys", func(t *testing.T) {
		t.Parallel()
		deployment := deploymentFromPayload(t, valid)
		deployment.EnvVars = map[string]string{"OTHER": ""}
		_, diags := flattenDeployment(context.Background(), deployment)
		if !diags.HasError() || !diagnosticsContain(diags, "env_vars") {
			t.Fatalf("diagnostics = %v, want mismatched env_vars", diags)
		}
	})
}

func TestFlattenDeploymentNormalizesNullableMetadata(t *testing.T) {
	t.Parallel()

	deployment := deploymentFromPayload(t, map[string]any{
		"id":                  "deployment_123",
		"created_at":          "2026-07-11T12:00:00Z",
		"updated_at":          nil,
		"region":              "aws.us-east-1a",
		"status":              "queued",
		"entrypoint_rel_path": "",
		"status_reason":       nil,
		"env_vars":            nil,
	})
	state, diags := flattenDeployment(context.Background(), deployment)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.EntrypointRelPath.IsNull() || !state.StatusReason.IsNull() || !state.UpdatedAt.IsNull() {
		t.Fatalf("optional state = %v/%v/%v, want null", state.EntrypointRelPath, state.StatusReason, state.UpdatedAt)
	}
	assertStringSet(t, state.EnvVarKeys, nil)
}

func validConfig() deploymentModel {
	return deploymentModel{ID: types.StringValue("deployment_123"), ProjectID: types.StringNull()}
}

func deploymentForTest(t *testing.T, id string) kernel.DeploymentGetResponse {
	t.Helper()
	return deploymentFromPayload(t, map[string]any{
		"id":                  id,
		"created_at":          "2026-07-11T12:00:00Z",
		"updated_at":          "2026-07-11T12:01:00Z",
		"region":              "aws.us-east-1a",
		"status":              "running",
		"entrypoint_rel_path": "src/index.ts",
		"status_reason":       "ready",
		"env_vars": map[string]string{
			"API_TOKEN": "must-not-enter-state",
			"REGION":    "must-not-enter-state",
		},
	})
}

func deploymentFromPayload(t *testing.T, payload map[string]any) kernel.DeploymentGetResponse {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal deployment: %v", err)
	}
	var deployment kernel.DeploymentGetResponse
	if err := json.Unmarshal(raw, &deployment); err != nil {
		t.Fatalf("unmarshal deployment: %v", err)
	}
	return deployment
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func assertStringSet(t *testing.T, set types.Set, want []string) {
	t.Helper()
	var got []string
	diags := set.ElementsAs(context.Background(), &got, false)
	if diags.HasError() {
		t.Fatalf("decode string set: %v", diags)
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

func diagnosticsContain(diags diag.Diagnostics, want string) bool {
	for _, diagnostic := range diags {
		if strings.Contains(diagnostic.Summary(), want) || strings.Contains(diagnostic.Detail(), want) {
			return true
		}
	}
	return false
}
