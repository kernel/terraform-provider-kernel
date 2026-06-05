package browserpool

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func normalizeChromePolicyJSON(input string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	var policy map[string]any

	if err := decodeChromePolicy(input, &policy); err != nil {
		diags.AddError(
			"Invalid Chrome Policy JSON",
			"chrome_policy must be valid JSON object syntax: "+err.Error(),
		)
		return "", diags
	}

	if policy == nil {
		diags.AddError(
			"Invalid Chrome Policy JSON",
			"chrome_policy must be a JSON object.",
		)
		return "", diags
	}

	normalized, err := json.Marshal(policy)
	if err != nil {
		diags.AddError(
			"Invalid Chrome Policy JSON",
			"chrome_policy could not be normalized: "+err.Error(),
		)
		return "", diags
	}

	return string(normalized), diags
}

func decodeChromePolicy(input string, policy *map[string]any) error {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()

	if err := decoder.Decode(policy); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("chrome_policy must contain exactly one JSON object")
		}
		return err
	}

	return nil
}
