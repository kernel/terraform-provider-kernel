package extension

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var (
	_ resource.Resource                = (*extensionResource)(nil)
	_ resource.ResourceWithConfigure   = (*extensionResource)(nil)
	_ resource.ResourceWithImportState = (*extensionResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*extensionResource)(nil)
)

type extensionClient interface {
	extensionUploader
	extensionReader
	extensionDeleter
	extensionImporter
}

type extensionResource struct {
	client extensionClient
}

func NewResource() resource.Resource {
	return &extensionResource{}
}

func (r *extensionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_extension"
}

func (r *extensionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = extensionSchema()
}

func (r *extensionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(extensionClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the extension durable client contract.",
		)
		return
	}

	r.client = client
}

func (r *extensionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defaultProjectID := ""
	if r.client != nil {
		defaultProjectID = r.client.DefaultProjectID()
	}
	createExtensionResource(ctx, r.client, defaultProjectID, req, resp)
}

func (r *extensionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	readExtensionResource(ctx, r.client, req, resp)
}

func (r *extensionResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Unexpected Kernel Extension Update",
		"Kernel extensions are immutable. Terraform should replace the extension when durable configuration changes; reaching Update indicates a provider planning error.",
	)
}

func (r *extensionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	deleteExtensionResource(ctx, r.client, req, resp)
}

func (r *extensionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importExtensionResource(ctx, r.client, req, resp)
}

func (r *extensionResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	modifyExtensionPlan(ctx, req, resp)
}
