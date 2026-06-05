// Package projectscope owns the policy for which Kernel project a single
// operation targets. An explicit attribute value wins, then the provider-level
// default. Empty means unscoped: no project header is sent and the API key's
// binding decides the project.
//
// Resolve is for operations driven by configuration (resource Create, data
// source Read). Operations on existing resources (Read, Update, Delete) must
// use the project recorded in state instead, so a later change to the
// provider default never re-points them at a different project.
package projectscope

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

// Resolve returns the project for a config-driven operation: the attribute
// value when set, otherwise the provider-level default. Unknown must fall
// back to the default, not error: an Optional+Computed resource attribute
// left unset arrives at apply time as unknown.
func Resolve(attribute types.String, defaultProjectID string) string {
	if !attribute.IsNull() && !attribute.IsUnknown() {
		return attribute.ValueString()
	}
	return defaultProjectID
}

// ResolveDataSource resolves the project for a data source read. Terraform
// normally defers data source reads until the configuration is wholly known,
// but the framework documents that Config may still carry unknown values; an
// unknown project must error rather than silently read the provider default.
func ResolveDataSource(diags *diag.Diagnostics, attribute types.String, defaultProjectID string) string {
	if attribute.IsUnknown() {
		diags.AddAttributeError(
			path.Root("project_id"),
			"Unknown Kernel Project ID",
			"project_id is not known during this read. Terraform defers data source reads until the value is known; if this error appears, re-run the operation or report it as a provider bug.",
		)
		return ""
	}
	return Resolve(attribute, defaultProjectID)
}

// StateValue converts a resolved project into its state representation:
// null when unscoped, so reads stay with the API key's binding.
func StateValue(projectID string) types.String {
	if projectID == "" {
		return types.StringNull()
	}
	return types.StringValue(projectID)
}

// AddError records err for a project-scoped operation. On calls that
// explicitly targeted a project, a 403 becomes a project-access diagnostic
// and a project_not_found 404 becomes a project diagnostic, both on
// project_id; every other error keeps its raw text under the operation name.
func AddError(diags *diag.Diagnostics, operation, projectID string, err error) {
	var apiError *kernel.Error
	if projectID != "" && errors.As(err, &apiError) {
		switch {
		case apiError.StatusCode == http.StatusForbidden:
			diags.AddAttributeError(
				path.Root("project_id"),
				"Kernel Project Access Denied",
				operation+" failed: the configured API key cannot access project "+projectID+". "+
					"Use an API key with access to this project, or change which project the operation targets. "+
					"Underlying error: "+err.Error(),
			)
			return
		case apiError.StatusCode == http.StatusNotFound && errorCode(apiError) == "project_not_found":
			diags.AddAttributeError(
				path.Root("project_id"),
				"Kernel Project Not Found",
				operation+" failed: project "+projectID+" was not found or is inactive. "+
					"Underlying error: "+err.Error(),
			)
			return
		}
	}
	diags.AddError(operation, err.Error())
}

// IsNotFound reports whether err is the API's coded resource-not-found
// response. A 404 without code "not_found" — such as the middleware's
// project_not_found for a missing or suspended project — does not mean the
// resource is gone and must not remove it from state.
func IsNotFound(err error) bool {
	var apiError *kernel.Error
	return errors.As(err, &apiError) &&
		apiError.StatusCode == http.StatusNotFound &&
		errorCode(apiError) == "not_found"
}

func errorCode(apiError *kernel.Error) string {
	var body struct {
		Code  string `json:"code"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(apiError.RawJSON()), &body) != nil {
		return ""
	}
	if body.Code != "" {
		return body.Code
	}
	return body.Error.Code
}
