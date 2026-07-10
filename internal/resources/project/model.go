package project

import "github.com/hashicorp/terraform-plugin-framework/types"

type projectModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}
