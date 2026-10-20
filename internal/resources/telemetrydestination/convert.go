package telemetrydestination

import (
	"context"
	"net/textproto"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func expandCreate(ctx context.Context, plan destinationModel) (kernel.TelemetryDestinationNewParams, diag.Diagnostics) {
	params := kernel.TelemetryDestinationNewParams{
		Name:     plan.Name.ValueString(),
		Endpoint: plan.Endpoint.ValueString(),
	}
	if !plan.Description.IsNull() {
		params.Description = kernel.String(plan.Description.ValueString())
	}
	headers, diags := headerMap(ctx, plan.Headers)
	params.Headers = headers
	return params, diags
}

// expandUpdate returns only the fields that differ between plan and state.
// Headers patch key by key on the API, so names dropped from configuration
// are sent as explicit nulls to delete them.
func expandUpdate(ctx context.Context, plan, state destinationModel) (kernel.TelemetryDestinationUpdateParams, diag.Diagnostics) {
	var params kernel.TelemetryDestinationUpdateParams
	if !plan.Name.Equal(state.Name) {
		params.Name = kernel.String(plan.Name.ValueString())
	}
	if !plan.Endpoint.Equal(state.Endpoint) {
		params.Endpoint = kernel.String(plan.Endpoint.ValueString())
	}
	if !plan.Description.Equal(state.Description) {
		// The API stores an empty description as none.
		params.Description = kernel.String(plan.Description.ValueString())
	}

	planHeaders, diags := headerMap(ctx, plan.Headers)
	stateHeaders, stateDiags := headerMap(ctx, state.Headers)
	diags.Append(stateDiags...)
	if diags.HasError() {
		return params, diags
	}

	patch := map[string]*string{}
	planNames := make(map[string]struct{}, len(planHeaders))
	for name, value := range planHeaders {
		planNames[textproto.CanonicalMIMEHeaderKey(name)] = struct{}{}
		if prior, ok := stateHeaders[name]; !ok || prior != value {
			patch[name] = &value
		}
	}
	for name := range stateHeaders {
		if _, ok := planNames[textproto.CanonicalMIMEHeaderKey(name)]; !ok {
			patch[name] = nil
		}
	}
	if len(patch) > 0 {
		params.SetExtraFields(map[string]any{"headers": patch})
	}
	return params, diags
}

// flatten builds state from a destination response. Kernel redacts header
// values, so values come from prior (plan or state) and only the set of
// header names is taken from the response; a name Kernel holds that prior
// lacks is recorded with an empty value so Terraform plans to correct it.
func flatten(ctx context.Context, remote kernel.OtlpDestination, prior types.Map) (destinationModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	for field, value := range map[string]string{"id": remote.ID, "name": remote.Name, "endpoint": remote.Endpoint} {
		if value == "" {
			diags.AddError(
				"Invalid Kernel Telemetry Destination Response",
				"Kernel returned a telemetry destination with missing field "+field+".",
			)
		}
	}
	priorHeaders, priorDiags := headerMap(ctx, prior)
	diags.Append(priorDiags...)
	if diags.HasError() {
		return destinationModel{}, diags
	}

	model := destinationModel{
		ID:          types.StringValue(remote.ID),
		Name:        types.StringValue(remote.Name),
		Endpoint:    types.StringValue(remote.Endpoint),
		Description: types.StringNull(),
		Headers:     types.MapNull(types.StringType),
	}
	if remote.Description != "" {
		model.Description = types.StringValue(remote.Description)
	}

	priorNames := make(map[string]string, len(priorHeaders))
	for name := range priorHeaders {
		priorNames[textproto.CanonicalMIMEHeaderKey(name)] = name
	}
	headers := make(map[string]string, len(remote.Headers))
	for name := range remote.Headers {
		if priorName, ok := priorNames[textproto.CanonicalMIMEHeaderKey(name)]; ok {
			headers[priorName] = priorHeaders[priorName]
		} else {
			headers[name] = ""
		}
	}
	if len(headers) > 0 || !prior.IsNull() {
		value, mapDiags := types.MapValueFrom(ctx, types.StringType, headers)
		diags.Append(mapDiags...)
		model.Headers = value
	}
	return model, diags
}

func headerMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var headers map[string]string
	diags := value.ElementsAs(ctx, &headers, false)
	return headers, diags
}
