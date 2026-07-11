package extension

import "github.com/hashicorp/terraform-plugin-framework/types"

type extensionModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	ProjectID    types.String `tfsdk:"project_id"`
	SourcePath   types.String `tfsdk:"source_path"`
	SourceSHA256 types.String `tfsdk:"source_sha256"`
}
