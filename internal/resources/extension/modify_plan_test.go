package extension

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestModifyExtensionPlanRequiresArchiveForCreate(t *testing.T) {
	t.Parallel()

	config := extensionPlanModel()
	config.SourcePath = types.StringNull()
	config.SourceSHA256 = types.StringNull()
	req := extensionModifyPlanRequest(t, config, extensionModel{}, config, false, true)
	var resp resource.ModifyPlanResponse

	modifyExtensionPlan(context.Background(), req, &resp)

	assertExtensionPlanDiagnosticPath(t, resp.Diagnostics, path.Root("source_path"))
	assertExtensionPlanDiagnosticPath(t, resp.Diagnostics, path.Root("source_sha256"))
}

func TestModifyExtensionPlanRequiresArchiveForImmutableReplacement(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		change       func(*extensionModel)
		omitChecksum bool
	}{
		"name": {
			change:       func(plan *extensionModel) { plan.Name = types.StringValue("New") },
			omitChecksum: true,
		},
		"project_id": {
			change:       func(plan *extensionModel) { plan.ProjectID = types.StringValue("project_new") },
			omitChecksum: true,
		},
		"source_sha256": {
			change: func(plan *extensionModel) { plan.SourceSHA256 = types.StringValue(extensionChecksum("b")) },
		},
	}

	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state := extensionPlanModel()
			plan := state
			change.change(&plan)
			config := plan
			config.SourcePath = types.StringNull()
			if change.omitChecksum {
				config.SourceSHA256 = types.StringNull()
			}
			req := extensionModifyPlanRequest(t, config, state, plan, true, true)
			var resp resource.ModifyPlanResponse

			modifyExtensionPlan(context.Background(), req, &resp)

			assertExtensionPlanDiagnosticPath(t, resp.Diagnostics, path.Root("source_path"))
			if change.omitChecksum {
				assertExtensionPlanDiagnosticPath(t, resp.Diagnostics, path.Root("source_sha256"))
			}
		})
	}
}

func TestModifyExtensionPlanAllowsCompleteCreateOrReplacement(t *testing.T) {
	t.Parallel()

	state := extensionPlanModel()
	plan := state
	plan.Name = types.StringValue("New")
	config := plan
	config.SourcePath = types.StringValue("extension.zip")
	req := extensionModifyPlanRequest(t, config, state, plan, true, true)
	var resp resource.ModifyPlanResponse

	modifyExtensionPlan(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

func TestModifyExtensionPlanDoesNotRequireArchiveForStableStateOrDestroy(t *testing.T) {
	t.Parallel()

	state := extensionPlanModel()
	config := state
	config.SourcePath = types.StringNull()
	config.SourceSHA256 = types.StringNull()

	t.Run("stable state", func(t *testing.T) {
		t.Parallel()
		req := extensionModifyPlanRequest(t, config, state, state, true, true)
		var resp resource.ModifyPlanResponse
		modifyExtensionPlan(context.Background(), req, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("destroy", func(t *testing.T) {
		t.Parallel()
		req := extensionModifyPlanRequest(t, config, state, extensionModel{}, true, false)
		var resp resource.ModifyPlanResponse
		modifyExtensionPlan(context.Background(), req, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
	})
}

func TestModifyExtensionPlanDefersUnknownArchiveInputs(t *testing.T) {
	t.Parallel()

	config := extensionPlanModel()
	config.SourcePath = types.StringUnknown()
	config.SourceSHA256 = types.StringUnknown()
	req := extensionModifyPlanRequest(t, config, extensionModel{}, config, false, true)
	var resp resource.ModifyPlanResponse

	modifyExtensionPlan(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics for deferred inputs: %v", resp.Diagnostics)
	}
}

func TestModifyExtensionPlanRequiresArchiveForUnknownReplacementValue(t *testing.T) {
	t.Parallel()

	state := extensionPlanModel()
	plan := state
	plan.Name = types.StringUnknown()
	config := plan
	config.SourcePath = types.StringNull()
	config.SourceSHA256 = types.StringNull()
	req := extensionModifyPlanRequest(t, config, state, plan, true, true)
	var resp resource.ModifyPlanResponse

	modifyExtensionPlan(context.Background(), req, &resp)

	assertExtensionPlanDiagnosticPath(t, resp.Diagnostics, path.Root("source_path"))
	assertExtensionPlanDiagnosticPath(t, resp.Diagnostics, path.Root("source_sha256"))
}

func extensionModifyPlanRequest(t *testing.T, config, state, plan extensionModel, hasState, hasPlan bool) resource.ModifyPlanRequest {
	t.Helper()
	ctx := context.Background()
	schema := extensionSchema()

	configValue := encodeExtensionPlanModel(t, config)
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: schema, Raw: configValue},
		State:  tfsdk.State{Schema: schema},
		Plan:   tfsdk.Plan{Schema: schema},
	}
	if hasState {
		if diags := req.State.Set(ctx, state); diags.HasError() {
			t.Fatalf("set extension state: %v", diags)
		}
	} else {
		req.State.RemoveResource(ctx)
	}
	if hasPlan {
		if diags := req.Plan.Set(ctx, plan); diags.HasError() {
			t.Fatalf("set extension plan: %v", diags)
		}
	} else {
		req.Plan.Raw = tftypes.NewValue(schema.Type().TerraformType(ctx), nil)
	}
	return req
}

func encodeExtensionPlanModel(t *testing.T, model extensionModel) tftypes.Value {
	t.Helper()
	var encoded tfsdk.Plan
	encoded.Schema = extensionSchema()
	if diags := encoded.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("encode extension model: %v", diags)
	}
	return encoded.Raw
}

func extensionPlanModel() extensionModel {
	return extensionModel{
		ID:           types.StringValue("extension_123"),
		Name:         types.StringValue("Extension"),
		ProjectID:    types.StringValue("project_123"),
		SourcePath:   types.StringNull(),
		SourceSHA256: types.StringValue(extensionChecksum("a")),
	}
}

func extensionChecksum(character string) string {
	return strings.Repeat(character, 64)
}

func assertExtensionPlanDiagnosticPath(t *testing.T, diags diag.Diagnostics, want path.Path) {
	t.Helper()
	for _, diagnostic := range diags {
		withPath, ok := diagnostic.(diag.DiagnosticWithPath)
		if ok && withPath.Path().Equal(want) {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want path %s", diags, want)
}
