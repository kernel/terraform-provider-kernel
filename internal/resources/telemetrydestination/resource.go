package telemetrydestination

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
	CreateTelemetryDestination(context.Context, kernel.TelemetryDestinationNewParams) (*kernel.OtlpDestination, error)
	GetTelemetryDestination(context.Context, string) (*kernel.OtlpDestination, error)
	UpdateTelemetryDestination(context.Context, string, kernel.TelemetryDestinationUpdateParams) (*kernel.OtlpDestination, error)
	DeleteTelemetryDestination(context.Context, string) error
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
	resp.TypeName = req.ProviderTypeName + "_telemetry_destination"
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
			"Expected provider data to implement the telemetry destination durable client contract.",
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

	params, diags := expandCreate(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateTelemetryDestination(ctx, params)
	if err != nil {
		if definiteFailure(err) {
			resp.Diagnostics.AddError("Create Kernel Telemetry Destination", errorDetail(err))
			return
		}
		addUncertainCreateDiagnostic(&resp.Diagnostics, plan.Name.ValueString(), err.Error())
		return
	}
	if created == nil {
		addUncertainCreateDiagnostic(&resp.Diagnostics, plan.Name.ValueString(), "Kernel returned an empty telemetry destination response.")
		return
	}

	state, diags := flatten(ctx, *created, plan.Headers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
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
	remote, err := r.client.GetTelemetryDestination(ctx, id)
	if err != nil {
		if projectscope.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Kernel Telemetry Destination", errorDetail(err))
		return
	}
	if remote == nil {
		resp.Diagnostics.AddError(
			"Read Kernel Telemetry Destination",
			"Kernel returned an empty response while reading telemetry destination "+strconv.Quote(id)+".",
		)
		return
	}

	next, diags := flatten(ctx, *remote, state.Headers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if next.ID.ValueString() != id {
		resp.Diagnostics.AddError(
			"Invalid Kernel Telemetry Destination Response",
			"Kernel returned telemetry destination ID "+strconv.Quote(next.ID.ValueString())+" while reading Terraform state ID "+strconv.Quote(id)+". "+
				"If this destination was imported by name, remove it from Terraform state and import it again using its ID.",
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

	params, diags := expandUpdate(ctx, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	remote, err := r.client.UpdateTelemetryDestination(ctx, id, params)
	if err != nil {
		resp.Diagnostics.AddError("Update Kernel Telemetry Destination", errorDetail(err))
		return
	}
	if remote == nil {
		resp.Diagnostics.AddError(
			"Update Kernel Telemetry Destination",
			"Kernel returned an empty response while updating telemetry destination "+strconv.Quote(id)+".",
		)
		return
	}

	next, diags := flatten(ctx, *remote, plan.Headers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if next.ID.ValueString() != id {
		resp.Diagnostics.AddError(
			"Invalid Kernel Telemetry Destination Response",
			"Kernel returned telemetry destination ID "+strconv.Quote(next.ID.ValueString())+" while updating "+strconv.Quote(id)+".",
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

	err := r.client.DeleteTelemetryDestination(ctx, state.ID.ValueString())
	if err != nil && !projectscope.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete Kernel Telemetry Destination", errorDetail(err))
	}
}

func (r *destinationResource) ImportState(ctx context.Context, req tfresource.ImportStateRequest, resp *tfresource.ImportStateResponse) {
	tfresource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *destinationResource) configured(diags *diag.Diagnostics) bool {
	if r.client == nil {
		diags.AddError(
			"Missing Kernel Client",
			"The Kernel provider was not configured before using the telemetry destination resource.",
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
		return "Telemetry destinations are organization-scoped, so the provider's API key must not be bound to a project. Underlying error: " + err.Error()
	}
	return err.Error()
}

func addUncertainCreateDiagnostic(diags *diag.Diagnostics, name, reason string) {
	diags.AddError(
		"Kernel Telemetry Destination Creation Outcome Uncertain",
		"Kernel may have created telemetry destination "+strconv.Quote(name)+", but Terraform did not receive a confirmation. "+
			"Check Kernel for the destination. If it exists, import its ID before applying again; otherwise retry the apply. "+
			"Reason: "+reason,
	)
}
