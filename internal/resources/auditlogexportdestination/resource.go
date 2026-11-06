package auditlogexportdestination

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ tfresource.Resource                = (*destinationResource)(nil)
	_ tfresource.ResourceWithConfigure   = (*destinationResource)(nil)
	_ tfresource.ResourceWithImportState = (*destinationResource)(nil)
)

type destinationClient interface {
	CreateAuditLogExportDestination(context.Context, kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error)
	GetAuditLogExportDestination(context.Context, string) (*kernel.AuditLogExportDestination, error)
	UpdateAuditLogExportDestination(context.Context, string, kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error)
	DeleteAuditLogExportDestination(context.Context, string) error
}

type destinationResource struct {
	client destinationClient
}

func newResourceWithClient(client destinationClient) *destinationResource {
	return &destinationResource{client: client}
}

func NewResource() tfresource.Resource {
	return &destinationResource{}
}

func (r *destinationResource) Metadata(ctx context.Context, req tfresource.MetadataRequest, resp *tfresource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_log_export_destination"
}

func (r *destinationResource) Schema(ctx context.Context, req tfresource.SchemaRequest, resp *tfresource.SchemaResponse) {
	resp.Schema = destinationSchema()
}

func (r *destinationResource) Configure(ctx context.Context, req tfresource.ConfigureRequest, resp *tfresource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(destinationClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the audit log export destination durable client contract.",
		)
		return
	}

	r.client = client
}

func (r *destinationResource) Create(ctx context.Context, req tfresource.CreateRequest, resp *tfresource.CreateResponse) {
	var plan destinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}

	created, err := r.client.CreateAuditLogExportDestination(ctx, expandCreate(plan))
	if err != nil {
		if definiteFailure(err) {
			resp.Diagnostics.AddError("Create Kernel Audit Log Export Destination", errorDetail(err))
			return
		}
		addUncertainCreateDiagnostic(&resp.Diagnostics, plan.Bucket.ValueString(), err.Error())
		return
	}
	if created == nil {
		addUncertainCreateDiagnostic(&resp.Diagnostics, plan.Bucket.ValueString(), "Kernel returned an empty audit log export destination response.")
		return
	}

	state, diags := flatten(*created, plan.Prefix)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Keep the created destination in state even if activation fails.
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Kernel creates destinations paused; activate only when requested.
	if !plan.Status.IsUnknown() && !plan.Status.Equal(state.Status) {
		params := kernel.AuditLogExportDestinationUpdateParams{
			UpdateAuditLogExportDestinationRequest: kernel.UpdateAuditLogExportDestinationRequestParam{
				Status: kernel.UpdateAuditLogExportDestinationRequestStatus(plan.Status.ValueString()),
			},
		}
		updated, err := r.client.UpdateAuditLogExportDestination(ctx, state.ID.ValueString(), params)
		if err != nil || updated == nil {
			detail := "Kernel returned an empty response."
			if err != nil {
				detail = errorDetail(err)
			}
			resp.Diagnostics.AddError(
				"Set Kernel Audit Log Export Destination Status",
				"Kernel created audit log export destination "+strconv.Quote(state.ID.ValueString())+" but did not set its status to "+
					strconv.Quote(plan.Status.ValueString())+". Run `terraform untaint` on the resource and apply again to set the status without replacing it. "+detail,
			)
			return
		}
		state, diags = flatten(*updated, plan.Prefix)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *destinationResource) Read(ctx context.Context, req tfresource.ReadRequest, resp *tfresource.ReadResponse) {
	var state destinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}

	id := state.ID.ValueString()
	remote, err := r.client.GetAuditLogExportDestination(ctx, id)
	if err != nil {
		if projectscope.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Kernel Audit Log Export Destination", errorDetail(err))
		return
	}
	if remote == nil {
		resp.Diagnostics.AddError(
			"Read Kernel Audit Log Export Destination",
			"Kernel returned an empty response while reading audit log export destination "+strconv.Quote(id)+".",
		)
		return
	}

	next, diags := flatten(*remote, state.Prefix)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if next.ID.ValueString() != id {
		resp.Diagnostics.AddError(
			"Invalid Kernel Audit Log Export Destination Response",
			"Kernel returned audit log export destination ID "+strconv.Quote(next.ID.ValueString())+" while reading Terraform state ID "+strconv.Quote(id)+".",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *destinationResource) Update(ctx context.Context, req tfresource.UpdateRequest, resp *tfresource.UpdateResponse) {
	var plan, state destinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}

	id := state.ID.ValueString()
	remote, err := r.client.UpdateAuditLogExportDestination(ctx, id, expandUpdate(plan, state))
	if err != nil {
		resp.Diagnostics.AddError("Update Kernel Audit Log Export Destination", errorDetail(err))
		return
	}
	if remote == nil {
		resp.Diagnostics.AddError(
			"Update Kernel Audit Log Export Destination",
			"Kernel returned an empty response while updating audit log export destination "+strconv.Quote(id)+".",
		)
		return
	}

	next, diags := flatten(*remote, plan.Prefix)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if next.ID.ValueString() != id {
		resp.Diagnostics.AddError(
			"Invalid Kernel Audit Log Export Destination Response",
			"Kernel returned audit log export destination ID "+strconv.Quote(next.ID.ValueString())+" while updating "+strconv.Quote(id)+".",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *destinationResource) Delete(ctx context.Context, req tfresource.DeleteRequest, resp *tfresource.DeleteResponse) {
	var state destinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}

	err := r.client.DeleteAuditLogExportDestination(ctx, state.ID.ValueString())
	if err != nil && !projectscope.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete Kernel Audit Log Export Destination", errorDetail(err))
	}
}

func (r *destinationResource) ImportState(ctx context.Context, req tfresource.ImportStateRequest, resp *tfresource.ImportStateResponse) {
	tfresource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *destinationResource) configured(diags *diag.Diagnostics) bool {
	if r.client == nil {
		diags.AddError(
			"Missing Kernel Client",
			"The Kernel provider was not configured before using the audit log export destination resource.",
		)
		return false
	}
	return true
}

func definiteFailure(err error) bool {
	var apiError *kernel.Error
	return errors.As(err, &apiError) && apiError.StatusCode >= http.StatusBadRequest && apiError.StatusCode < http.StatusInternalServerError
}

func errorDetail(err error) string {
	var apiError *kernel.Error
	if errors.As(err, &apiError) && apiError.StatusCode == http.StatusForbidden {
		return "Audit log export destinations are organization-scoped and need an API key that is not bound to a project, on a Kernel plan that includes audit logs. Underlying error: " + err.Error()
	}
	return err.Error()
}

func addUncertainCreateDiagnostic(diags *diag.Diagnostics, bucket, reason string) {
	diags.AddError(
		"Kernel Audit Log Export Destination Creation Outcome Uncertain",
		"Kernel may have created an audit log export destination for bucket "+strconv.Quote(bucket)+", but Terraform did not receive a confirmation. "+
			"Check Kernel for the destination. If it exists, import its ID before applying again; otherwise retry the apply. "+
			"Reason: "+reason,
	)
}
