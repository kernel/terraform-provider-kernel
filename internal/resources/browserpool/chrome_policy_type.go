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

// chromePolicyType stores the chrome_policy string verbatim and overrides only
// semantic equality, so JSON objects that differ only in key order or
// whitespace do not produce a spurious plan diff. It deliberately never fails
// value conversion on malformed input: a bad chrome_policy is a user error
// that chromePolicyJSONValidator reports cleanly, not a provider bug. (An
// error returned from ValueFromTerraform surfaces as the framework's "please
// report the following to the provider developer" diagnostic and pre-empts
// attribute validation entirely.)
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

func (t chromePolicyType) ValueFromString(_ context.Context, value basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return chromePolicyValue{StringValue: value}, nil
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

	return chromePolicyValue{StringValue: stringValue}, nil
}

func (t chromePolicyType) ValueType(context.Context) attr.Value {
	return chromePolicyValue{}
}

type chromePolicyValue struct {
	basetypes.StringValue
}

// Equal reports literal string equality (used for framework bookkeeping);
// semantic JSON equality lives in StringSemanticEquals.
func (v chromePolicyValue) Equal(other attr.Value) bool {
	otherValue, ok := other.(chromePolicyValue)
	if !ok {
		return false
	}

	return v.StringValue.Equal(otherValue.StringValue)
}

// StringSemanticEquals treats two chrome_policy strings as equal when they
// encode the same JSON object regardless of key order or whitespace. A value
// that cannot be normalized is treated as not-equal and never raises a
// diagnostic: malformed JSON is reported by chromePolicyJSONValidator, and
// returning not-equal here keeps the proposed value so both the diff and that
// validation error surface.
func (v chromePolicyValue) StringSemanticEquals(ctx context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	otherValue, diags := other.ToStringValue(ctx)
	if diags.HasError() {
		return false, diags
	}

	thisNormalized, thisDiags := normalizeChromePolicyJSON(v.ValueString())
	otherNormalized, otherDiags := normalizeChromePolicyJSON(otherValue.ValueString())
	if thisDiags.HasError() || otherDiags.HasError() {
		return false, nil
	}

	return thisNormalized == otherNormalized, nil
}

func (v chromePolicyValue) Type(context.Context) attr.Type {
	return chromePolicyType{}
}
