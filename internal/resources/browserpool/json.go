package browserpool

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// normalizeChromePolicyJSON parses a chrome_policy string as a single JSON
// object and re-serializes it to a canonical form (Go marshals map keys
// sorted, with no insignificant whitespace) for semantic comparison. It does
// not HTML-escape <, >, or & so a normalized value matches the bytes a user
// wrote rather than <-style escapes.
func normalizeChromePolicyJSON(input string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	policy, err := decodeChromePolicy(input)
	if err != nil {
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

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(policy); err != nil {
		diags.AddError(
			"Invalid Chrome Policy JSON",
			"chrome_policy could not be normalized: "+err.Error(),
		)
		return "", diags
	}

	// Encode appends a trailing newline; drop it so equal objects compare equal.
	return strings.TrimRight(buf.String(), "\n"), diags
}

// decodeChromePolicy decodes input as exactly one JSON object. A JSON null (or
// any non-object) leaves a nil map, which callers reject as "not an object";
// a syntactically invalid or trailing-garbage input returns an error.
// Duplicate keys within the object follow encoding/json's last-value-wins
// behavior — detecting them would need a token-level scan and is out of scope.
func decodeChromePolicy(input string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()

	var policy map[string]any
	if err := decoder.Decode(&policy); err != nil {
		return nil, err
	}

	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("chrome_policy must contain exactly one JSON object")
		}
		return nil, err
	}

	return policy, nil
}
