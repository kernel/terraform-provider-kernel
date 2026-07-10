package project

import (
	"context"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestFrameworkImportWritesStateForRead(t *testing.T) {
	t.Parallel()

	r := &projectResource{}
	var resp tfresource.ImportStateResponse
	resp.State.Schema = projectSchema()
	resp.State.RemoveResource(context.Background())

	r.ImportState(context.Background(), tfresource.ImportStateRequest{ID: "project_123"}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var state projectModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get import state: %v", diags)
	}
	if got := state.ID.ValueString(); got != "project_123" {
		t.Fatalf("state id = %q, want project_123", got)
	}
	if !state.Name.IsUnknown() {
		t.Fatalf("state name = %v, want unknown until Read", state.Name)
	}
}

func TestFrameworkImportRejectsEmptyIDWithoutState(t *testing.T) {
	t.Parallel()

	r := &projectResource{}
	var resp tfresource.ImportStateResponse
	resp.State.Schema = projectSchema()
	resp.State.RemoveResource(context.Background())

	r.ImportState(context.Background(), tfresource.ImportStateRequest{}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid import ID diagnostic")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state = %v, want absent state", resp.State.Raw)
	}
}
