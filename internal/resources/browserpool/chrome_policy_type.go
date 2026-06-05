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
	_ basetypes.StringTypable                    = ChromePolicyType{}
	_ basetypes.StringValuableWithSemanticEquals = ChromePolicyValue{}
)

type ChromePolicyType struct {
	basetypes.StringType
}

func (t ChromePolicyType) Equal(other attr.Type) bool {
	_, ok := other.(ChromePolicyType)
	return ok
}

func (t ChromePolicyType) String() string {
	return "browserpool.ChromePolicyType"
}

func (t ChromePolicyType) ValueFromString(ctx context.Context, value basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return ChromePolicyValue{StringValue: value}, nil
	}

	normalized, diags := NormalizeChromePolicyJSON(value.ValueString())
	if diags.HasError() {
		return ChromePolicyValue{StringValue: value}, diags
	}

	return ChromePolicyValue{StringValue: basetypes.NewStringValue(normalized)}, nil
}

func (t ChromePolicyType) ValueFromTerraform(ctx context.Context, value tftypes.Value) (attr.Value, error) {
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

func (t ChromePolicyType) ValueType(ctx context.Context) attr.Value {
	return ChromePolicyValue{}
}

type ChromePolicyValue struct {
	basetypes.StringValue
}

func (v ChromePolicyValue) Equal(other attr.Value) bool {
	otherValue, ok := other.(ChromePolicyValue)
	if !ok {
		return false
	}

	return v.StringValue.Equal(otherValue.StringValue)
}

func (v ChromePolicyValue) StringSemanticEquals(ctx context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	otherValue, diags := other.ToStringValue(ctx)
	if diags.HasError() {
		return false, diags
	}

	thisNormalized, thisDiags := NormalizeChromePolicyJSON(v.ValueString())
	diags.Append(thisDiags...)
	otherNormalized, otherDiags := NormalizeChromePolicyJSON(otherValue.ValueString())
	diags.Append(otherDiags...)
	if diags.HasError() {
		return false, diags
	}

	return thisNormalized == otherNormalized, diags
}

func (v ChromePolicyValue) Type(ctx context.Context) attr.Type {
	return ChromePolicyType{}
}

func ChromePolicyNull() ChromePolicyValue {
	return ChromePolicyValue{StringValue: basetypes.NewStringNull()}
}
