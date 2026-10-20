package telemetrydestination

import "github.com/hashicorp/terraform-plugin-framework/types"

type destinationModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Endpoint    types.String `tfsdk:"endpoint"`
	Description types.String `tfsdk:"description"`
	Headers     types.Map    `tfsdk:"headers"`
}
