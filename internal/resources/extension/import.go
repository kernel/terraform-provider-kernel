package extension

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

type extensionImporter interface {
	DefaultProjectID() string
}

func importExtensionResource(ctx context.Context, client extensionImporter, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if client == nil {
		resp.Diagnostics.AddError(
			"Missing Kernel Client",
			"The Kernel provider was not configured before importing an extension resource.",
		)
		return
	}

	projectID, extensionID, ok := parseExtensionImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid Kernel Extension Import ID",
			"Cannot import "+strconv.Quote(req.ID)+": import an extension as \"<extension-id>\" or \"<project-id>/<extension-id>\". "+
				"The bare form uses the provider project_id when configured, otherwise the API key's project binding. Use the qualified form for a different project.",
		)
		return
	}
	if projectID == "" {
		projectID = client.DefaultProjectID()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, extensionModel{
		ID:           types.StringValue(extensionID),
		Name:         types.StringUnknown(),
		ProjectID:    projectscope.StateValue(projectID),
		SourcePath:   types.StringNull(),
		SourceSHA256: types.StringUnknown(),
	})...)
}

func parseExtensionImportID(id string) (projectID, extensionID string, ok bool) {
	before, after, found := strings.Cut(id, "/")
	if !found {
		return "", id, id != ""
	}
	if strings.Contains(after, "/") {
		return "", "", false
	}
	if before == "" || after == "" {
		return "", "", false
	}
	return before, after, true
}
