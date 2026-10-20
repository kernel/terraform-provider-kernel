package telemetrydestination

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ destinationClient = kernelclient.Clients{}

func headersValue(t *testing.T, headers map[string]string) types.Map {
	t.Helper()
	if headers == nil {
		return types.MapNull(types.StringType)
	}
	elements := make(map[string]attr.Value, len(headers))
	for name, value := range headers {
		elements[name] = types.StringValue(value)
	}
	return types.MapValueMust(types.StringType, elements)
}

func destinationForTest(t *testing.T, raw string) kernel.OtlpDestination {
	t.Helper()
	var destination kernel.OtlpDestination
	if err := json.Unmarshal([]byte(raw), &destination); err != nil {
		t.Fatalf("unmarshal destination: %v", err)
	}
	return destination
}

func model(t *testing.T, description *string, headers map[string]string) destinationModel {
	t.Helper()
	m := destinationModel{
		ID:          types.StringValue("dest_123"),
		Name:        types.StringValue("honeycomb"),
		Endpoint:    types.StringValue("https://api.honeycomb.io"),
		Description: types.StringNull(),
		Headers:     headersValue(t, headers),
	}
	if description != nil {
		m.Description = types.StringValue(*description)
	}
	return m
}

func marshal(t *testing.T, v json.Marshaler) string {
	t.Helper()
	body, err := v.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(body)
}

func TestExpandCreateSendsConfiguredFields(t *testing.T) {
	t.Parallel()

	description := "prod"
	params, diags := expandCreate(context.Background(), model(t, &description, map[string]string{"x-honeycomb-team": "key"}))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	got := marshal(t, params)
	want := `{"endpoint":"https://api.honeycomb.io","name":"honeycomb","description":"prod","headers":{"x-honeycomb-team":"key"}}`
	if got != want {
		t.Fatalf("create body = %s, want %s", got, want)
	}
}

func TestExpandCreateOmitsUnsetOptionalFields(t *testing.T) {
	t.Parallel()

	params, diags := expandCreate(context.Background(), model(t, nil, nil))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got, want := marshal(t, params), `{"endpoint":"https://api.honeycomb.io","name":"honeycomb"}`; got != want {
		t.Fatalf("create body = %s, want %s", got, want)
	}
}

func TestExpandUpdateSendsOnlyChanges(t *testing.T) {
	t.Parallel()

	description := "prod"
	state := model(t, &description, map[string]string{"Authorization": "old", "X-Keep": "same", "X-Drop": "gone"})
	plan := model(t, nil, map[string]string{"Authorization": "new", "X-Keep": "same", "X-Add": "added"})
	plan.Endpoint = types.StringValue("https://otlp.example")

	params, diags := expandUpdate(context.Background(), plan, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	got := marshal(t, params)
	want := `{"description":"","endpoint":"https://otlp.example","headers":{"Authorization":"new","X-Add":"added","X-Drop":null}}`
	if got != want {
		t.Fatalf("update body = %s, want %s", got, want)
	}
}

func TestExpandUpdateRespellingHeaderDoesNotDeleteIt(t *testing.T) {
	t.Parallel()

	state := model(t, nil, map[string]string{"authorization": "key"})
	plan := model(t, nil, map[string]string{"Authorization": "key"})

	params, diags := expandUpdate(context.Background(), plan, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got, want := marshal(t, params), `{"headers":{"Authorization":"key"}}`; got != want {
		t.Fatalf("update body = %s, want %s", got, want)
	}
}

func TestExpandUpdateRemovingAllHeadersDeletesEach(t *testing.T) {
	t.Parallel()

	state := model(t, nil, map[string]string{"Authorization": "key"})
	plan := model(t, nil, nil)

	params, diags := expandUpdate(context.Background(), plan, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got, want := marshal(t, params), `{"headers":{"Authorization":null}}`; got != want {
		t.Fatalf("update body = %s, want %s", got, want)
	}
}

func TestFlattenKeepsPriorHeaderValuesForRedactedResponse(t *testing.T) {
	t.Parallel()

	remote := destinationForTest(t, `{"id":"dest_123","name":"honeycomb","endpoint":"https://api.honeycomb.io","description":"prod","headers":{"X-Honeycomb-Team":""}}`)
	prior := headersValue(t, map[string]string{"x-honeycomb-team": "key"})

	got, diags := flatten(context.Background(), remote, prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.Headers.Equal(prior) {
		t.Fatalf("headers = %v, want %v", got.Headers, prior)
	}
	if got, want := got.Description.ValueString(), "prod"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestFlattenSurfacesHeaderNameDrift(t *testing.T) {
	t.Parallel()

	remote := destinationForTest(t, `{"id":"dest_123","name":"honeycomb","endpoint":"https://api.honeycomb.io","headers":{"X-Added-Outside":""}}`)
	prior := headersValue(t, map[string]string{"Authorization": "key"})

	got, diags := flatten(context.Background(), remote, prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	want := headersValue(t, map[string]string{"X-Added-Outside": ""})
	if !got.Headers.Equal(want) {
		t.Fatalf("headers = %v, want %v", got.Headers, want)
	}
	if !got.Description.IsNull() {
		t.Fatalf("description = %v, want null", got.Description)
	}
}

func TestFlattenKeepsNullHeadersWhenNoneStored(t *testing.T) {
	t.Parallel()

	remote := destinationForTest(t, `{"id":"dest_123","name":"honeycomb","endpoint":"https://api.honeycomb.io","headers":{}}`)

	got, diags := flatten(context.Background(), remote, types.MapNull(types.StringType))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.Headers.IsNull() {
		t.Fatalf("headers = %v, want null", got.Headers)
	}
}

func TestFlattenRejectsMissingFields(t *testing.T) {
	t.Parallel()

	remote := destinationForTest(t, `{"id":"dest_123","name":"honeycomb","headers":{}}`)

	if _, diags := flatten(context.Background(), remote, types.MapNull(types.StringType)); !diags.HasError() {
		t.Fatal("expected diagnostics for missing endpoint")
	}
}

func TestSchemaMarksHeadersSensitive(t *testing.T) {
	t.Parallel()

	headers, ok := destinationSchema().Attributes["headers"]
	if !ok {
		t.Fatal("missing headers attribute")
	}
	if !headers.IsSensitive() {
		t.Fatal("headers should be sensitive")
	}
}

func TestFlattenImportRecordsHeaderNamesWithEmptyValues(t *testing.T) {
	t.Parallel()

	remote := destinationForTest(t, `{"id":"dest_123","name":"honeycomb","endpoint":"https://api.honeycomb.io","headers":{"Authorization":"","X-Dataset":""}}`)

	got, diags := flatten(context.Background(), remote, types.MapNull(types.StringType))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	want := headersValue(t, map[string]string{"Authorization": "", "X-Dataset": ""})
	if !got.Headers.Equal(want) {
		t.Fatalf("headers = %v, want %v", got.Headers, want)
	}
}
