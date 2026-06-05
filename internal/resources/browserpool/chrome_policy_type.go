package browserpool

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ basetypes.StringTypable                    = chromePolicyType{}
	_ basetypes.StringValuableWithSemanticEquals = chromePolicyValue{}
)

type chromePolicyType struct {
	basetypes.StringType
}

func (t chromePolicyType) Equal(other attr.Type) bool {
	_, ok := other.(chromePolicyType)
	return ok
}

func (t chromePolicyType) String() string {
	return "browserpool.chromePolicyType"
}

func (t chromePolicyType) ValueFromString(ctx context.Context, value basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return chromePolicyValue{StringValue: value}, nil
	}

	normalized, diags := normalizeChromePolicyJSON(value.ValueString())
	if diags.HasError() {
		return chromePolicyValue{StringValue: value}, diags
	}

	return chromePolicyValue{StringValue: basetypes.NewStringValue(normalized)}, nil
}

func (t chromePolicyType) ValueFromTerraform(ctx context.Context, value tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, value)
	if err != nil {
		return nil, err
	}

	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected chrome_policy value type %T", attrValue)
	}

	chromePolicy, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("invalid chrome_policy: %v", diags)
	}

	return chromePolicy, nil
}

func (t chromePolicyType) ValueType(ctx context.Context) attr.Value {
	return chromePolicyValue{}
}

type chromePolicyValue struct {
	basetypes.StringValue
}

func (v chromePolicyValue) Equal(other attr.Value) bool {
	otherValue, ok := other.(chromePolicyValue)
	if !ok {
		return false
	}

	return v.StringValue.Equal(otherValue.StringValue)
}

func (v chromePolicyValue) StringSemanticEquals(ctx context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	otherValue, diags := other.ToStringValue(ctx)
	if diags.HasError() {
		return false, diags
	}

	thisNormalized, thisDiags := normalizeChromePolicyJSON(v.ValueString())
	diags.Append(thisDiags...)
	otherNormalized, otherDiags := normalizeChromePolicyJSON(otherValue.ValueString())
	diags.Append(otherDiags...)
	if diags.HasError() {
		return false, diags
	}

	return thisNormalized == otherNormalized, diags
}

func (v chromePolicyValue) Type(ctx context.Context) attr.Type {
	return chromePolicyType{}
}
