package datasources

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type IDNameSelector struct {
	HasID   bool
	HasName bool
}

func ResolveIDNameSelector(kind, typeName string, id, name types.String) (IDNameSelector, diag.Diagnostics) {
	var diags diag.Diagnostics

	if id.IsUnknown() || name.IsUnknown() {
		diags.AddError(
			"Unknown "+kind+" Selector",
			kind+" id and name must be known before reading the data source.",
		)
		return IDNameSelector{}, diags
	}
	if !id.IsNull() && id.ValueString() == "" {
		diags.AddError(
			"Empty "+kind+" ID",
			kind+" id must be omitted or a non-empty string.",
		)
	}
	if !name.IsNull() && name.ValueString() == "" {
		diags.AddError(
			"Empty "+kind+" Name",
			kind+" name must be omitted or a non-empty string.",
		)
	}
	if diags.HasError() {
		return IDNameSelector{}, diags
	}

	selector := IDNameSelector{
		HasID:   !id.IsNull(),
		HasName: !name.IsNull(),
	}
	if selector.HasID && selector.HasName {
		diags.AddError(
			"Conflicting "+kind+" Selectors",
			"Configure only one of id or name for "+typeName+".",
		)
		return IDNameSelector{}, diags
	}

	return selector, diags
}

func FieldPresent(raw string) bool {
	return raw != "" && strings.TrimSpace(raw) != "null"
}

func ValidResponseString(raw string, valid bool, value string) bool {
	if !FieldPresent(raw) || !valid || value == "" {
		return false
	}

	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded == value
}

func ValidResponseTime(raw string, valid bool, value time.Time) bool {
	if !FieldPresent(raw) || !valid || value.IsZero() {
		return false
	}

	var decoded time.Time
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded.Equal(value)
}

func AddInvalidResponseField(diags *diag.Diagnostics, kind, field string) {
	diags.AddError(
		"Invalid Kernel "+kind+" Response",
		"Kernel returned a "+strings.ToLower(kind)+" with missing or invalid field "+field+".",
	)
}
