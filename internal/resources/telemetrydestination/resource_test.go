package telemetrydestination

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

type fakeDestinationClient struct {
	create func(context.Context, kernel.TelemetryDestinationNewParams) (*kernel.OtlpDestination, error)
	get    func(context.Context, string) (*kernel.OtlpDestination, error)
	update func(context.Context, string, kernel.TelemetryDestinationUpdateParams) (*kernel.OtlpDestination, error)
	delete func(context.Context, string) error
}

func (f fakeDestinationClient) CreateTelemetryDestination(ctx context.Context, params kernel.TelemetryDestinationNewParams) (*kernel.OtlpDestination, error) {
	if f.create == nil {
		return nil, errors.New("unexpected create")
	}
	return f.create(ctx, params)
}

func (f fakeDestinationClient) GetTelemetryDestination(ctx context.Context, id string) (*kernel.OtlpDestination, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, id)
}

func (f fakeDestinationClient) UpdateTelemetryDestination(ctx context.Context, id string, params kernel.TelemetryDestinationUpdateParams) (*kernel.OtlpDestination, error) {
	if f.update == nil {
		return nil, errors.New("unexpected update")
	}
	return f.update(ctx, id, params)
}

func (f fakeDestinationClient) DeleteTelemetryDestination(ctx context.Context, id string) error {
	if f.delete == nil {
		return errors.New("unexpected delete")
	}
	return f.delete(ctx, id)
}

const remoteDestination = `{"id":"dest_123","name":"honeycomb","endpoint":"https://api.honeycomb.io","headers":{"Authorization":""}}`

func destinationResponse(t *testing.T, raw string) (*kernel.OtlpDestination, error) {
	t.Helper()
	destination := destinationForTest(t, raw)
	return &destination, nil
}

func apiError(t *testing.T, status int, raw string) *kernel.Error {
	t.Helper()

	var apiError kernel.Error
	if err := json.Unmarshal([]byte(raw), &apiError); err != nil {
		t.Fatalf("unmarshal API error: %v", err)
	}
	apiError.StatusCode = status
	apiError.Request = &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/telemetry/destinations"},
	}
	apiError.Response = &http.Response{StatusCode: status, Status: http.StatusText(status)}
	return &apiError
}

func newState(t *testing.T, value *destinationModel) tfsdk.State {
	t.Helper()

	state := tfsdk.State{Schema: destinationSchema()}
	if value == nil {
		state.RemoveResource(context.Background())
		return state
	}
	if diags := state.Set(context.Background(), value); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return state
}

func newPlan(t *testing.T, value destinationModel) tfsdk.Plan {
	t.Helper()

	plan := tfsdk.Plan{Schema: destinationSchema()}
	if diags := plan.Set(context.Background(), value); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return plan
}

func stateModel(t *testing.T, state tfsdk.State) destinationModel {
	t.Helper()

	var m destinationModel
	if diags := state.Get(context.Background(), &m); diags.HasError() {
		t.Fatalf("get state: %v", diags)
	}
	return m
}

func plannedModel(t *testing.T) destinationModel {
	t.Helper()

	m := model(t, nil, map[string]string{"authorization": "key"})
	m.ID = types.StringUnknown()
	return m
}

func TestCreatePersistsStateWithConfiguredHeaderValues(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		create: func(context.Context, kernel.TelemetryDestinationNewParams) (*kernel.OtlpDestination, error) {
			return destinationResponse(t, remoteDestination)
		},
	})
	resp := &tfresource.CreateResponse{State: newState(t, nil)}

	r.Create(context.Background(), tfresource.CreateRequest{Plan: newPlan(t, plannedModel(t))}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	got := stateModel(t, resp.State)
	if got.ID.ValueString() != "dest_123" {
		t.Fatalf("state id = %q, want dest_123", got.ID.ValueString())
	}
	if want := headersValue(t, map[string]string{"authorization": "key"}); !got.Headers.Equal(want) {
		t.Fatalf("headers = %v, want %v", got.Headers, want)
	}
}

func TestCreateFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		create      func() (*kernel.OtlpDestination, error)
		wantSummary string
		wantDetail  string
	}{
		"definite API rejection": {
			create: func() (*kernel.OtlpDestination, error) {
				return nil, apiError(t, http.StatusConflict, `{"code":"conflict"}`)
			},
			wantSummary: "Create Kernel Telemetry Destination",
		},
		"project-scoped credential": {
			create: func() (*kernel.OtlpDestination, error) {
				return nil, apiError(t, http.StatusForbidden, `{"code":"forbidden"}`)
			},
			wantSummary: "Create Kernel Telemetry Destination",
			wantDetail:  "must not be bound to a project",
		},
		"server error": {
			create: func() (*kernel.OtlpDestination, error) {
				return nil, apiError(t, http.StatusInternalServerError, `{"code":"internal_error"}`)
			},
			wantSummary: "Kernel Telemetry Destination Creation Outcome Uncertain",
		},
		"transport error": {
			create: func() (*kernel.OtlpDestination, error) {
				return nil, errors.New("connection reset")
			},
			wantSummary: "Kernel Telemetry Destination Creation Outcome Uncertain",
			wantDetail:  "connection reset",
		},
		"empty response": {
			create: func() (*kernel.OtlpDestination, error) {
				return nil, nil
			},
			wantSummary: "Kernel Telemetry Destination Creation Outcome Uncertain",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newResourceWithClient(fakeDestinationClient{
				create: func(context.Context, kernel.TelemetryDestinationNewParams) (*kernel.OtlpDestination, error) {
					return test.create()
				},
			})
			resp := &tfresource.CreateResponse{State: newState(t, nil)}

			r.Create(context.Background(), tfresource.CreateRequest{Plan: newPlan(t, plannedModel(t))}, resp)

			if got := len(resp.Diagnostics); got != 1 {
				t.Fatalf("diagnostics = %v, want one error", resp.Diagnostics)
			}
			if got := resp.Diagnostics[0].Summary(); got != test.wantSummary {
				t.Fatalf("summary = %q, want %q", got, test.wantSummary)
			}
			if got := resp.Diagnostics[0].Detail(); !strings.Contains(got, test.wantDetail) {
				t.Fatalf("detail = %q, want it to contain %q", got, test.wantDetail)
			}
			if !resp.State.Raw.IsNull() {
				t.Fatal("failed create persisted state")
			}
		})
	}
}

func TestReadRefreshesState(t *testing.T) {
	t.Parallel()

	var gotID string
	r := newResourceWithClient(fakeDestinationClient{
		get: func(ctx context.Context, id string) (*kernel.OtlpDestination, error) {
			gotID = id
			return destinationResponse(t, `{"id":"dest_123","name":"renamed","endpoint":"https://api.honeycomb.io","headers":{"Authorization":""}}`)
		},
	})
	prior := model(t, nil, map[string]string{"authorization": "key"})
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotID != "dest_123" {
		t.Fatalf("get id = %q, want dest_123", gotID)
	}
	got := stateModel(t, resp.State)
	if got.Name.ValueString() != "renamed" {
		t.Fatalf("name = %q, want renamed", got.Name.ValueString())
	}
	if !got.Headers.Equal(prior.Headers) {
		t.Fatalf("headers = %v, want %v", got.Headers, prior.Headers)
	}
}

func TestReadRemovesConfirmedMissingDestination(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		get: func(context.Context, string) (*kernel.OtlpDestination, error) {
			return nil, apiError(t, http.StatusNotFound, `{"code":"not_found"}`)
		},
	})
	prior := model(t, nil, nil)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing destination was not removed from state")
	}
}

func TestReadKeepsStateOnUncodedNotFound(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		get: func(context.Context, string) (*kernel.OtlpDestination, error) {
			return nil, apiError(t, http.StatusNotFound, `{"code":"project_not_found"}`)
		},
	})
	prior := model(t, nil, nil)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected read error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("uncoded 404 removed the destination from state")
	}
}

func TestReadRejectsCanonicalIDMismatch(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		get: func(context.Context, string) (*kernel.OtlpDestination, error) {
			return destinationResponse(t, `{"id":"dest_456","name":"honeycomb","endpoint":"https://api.honeycomb.io","headers":{}}`)
		},
	})
	prior := model(t, nil, nil)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected ID mismatch error")
	}
	if got := stateModel(t, resp.State).ID.ValueString(); got != "dest_123" {
		t.Fatalf("state id = %q, want preserved dest_123", got)
	}
}

func TestUpdateSendsPatchAndPersistsPlannedHeaders(t *testing.T) {
	t.Parallel()

	var gotID, gotBody string
	r := newResourceWithClient(fakeDestinationClient{
		update: func(ctx context.Context, id string, params kernel.TelemetryDestinationUpdateParams) (*kernel.OtlpDestination, error) {
			gotID = id
			gotBody = marshal(t, params)
			return destinationResponse(t, `{"id":"dest_123","name":"honeycomb","endpoint":"https://api.honeycomb.io","headers":{"X-New":""}}`)
		},
	})
	prior := model(t, nil, map[string]string{"Authorization": "key"})
	plan := model(t, nil, map[string]string{"X-New": "value"})
	resp := &tfresource.UpdateResponse{State: newState(t, &prior)}

	r.Update(context.Background(), tfresource.UpdateRequest{Plan: newPlan(t, plan), State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotID != "dest_123" {
		t.Fatalf("update id = %q, want dest_123", gotID)
	}
	if want := `{"headers":{"Authorization":null,"X-New":"value"}}`; gotBody != want {
		t.Fatalf("update body = %s, want %s", gotBody, want)
	}
	if got := stateModel(t, resp.State); !got.Headers.Equal(plan.Headers) {
		t.Fatalf("headers = %v, want %v", got.Headers, plan.Headers)
	}
}

func TestUpdateRejectsIDMismatch(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		update: func(context.Context, string, kernel.TelemetryDestinationUpdateParams) (*kernel.OtlpDestination, error) {
			return destinationResponse(t, `{"id":"dest_456","name":"renamed","endpoint":"https://api.honeycomb.io","headers":{}}`)
		},
	})
	prior := model(t, nil, nil)
	plan := model(t, nil, nil)
	plan.Name = types.StringValue("renamed")
	resp := &tfresource.UpdateResponse{State: newState(t, &prior)}

	r.Update(context.Background(), tfresource.UpdateRequest{Plan: newPlan(t, plan), State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected ID mismatch error")
	}
}

func TestDeleteToleratesMissingDestination(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		delete: func(context.Context, string) error {
			return apiError(t, http.StatusNotFound, `{"code":"not_found"}`)
		},
	})
	prior := model(t, nil, nil)
	resp := &tfresource.DeleteResponse{State: newState(t, &prior)}

	r.Delete(context.Background(), tfresource.DeleteRequest{State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

func TestDeleteReportsConflict(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		delete: func(context.Context, string) error {
			return apiError(t, http.StatusConflict, `{"code":"conflict","message":"in use by 1 browser session(s)"}`)
		},
	})
	prior := model(t, nil, nil)
	resp := &tfresource.DeleteResponse{State: newState(t, &prior)}

	r.Delete(context.Background(), tfresource.DeleteRequest{State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected delete conflict error")
	}
}

func TestImportStateSetsID(t *testing.T) {
	t.Parallel()

	resp := &tfresource.ImportStateResponse{State: newState(t, nil)}

	NewResource().(*destinationResource).ImportState(context.Background(), tfresource.ImportStateRequest{ID: "dest_123"}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	got := stateModel(t, resp.State)
	if got.ID.ValueString() != "dest_123" {
		t.Fatalf("state id = %q, want dest_123", got.ID.ValueString())
	}
	if !got.Headers.IsNull() {
		t.Fatalf("headers = %v, want null before refresh", got.Headers)
	}
}

func TestConfigure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		data       any
		wantError  bool
		wantClient bool
	}{
		"durable client":    {data: fakeDestinationClient{}, wantClient: true},
		"unexpected data":   {data: "not a client", wantError: true},
		"nil provider data": {},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := &destinationResource{}
			var resp tfresource.ConfigureResponse
			r.Configure(context.Background(), tfresource.ConfigureRequest{ProviderData: test.data}, &resp)

			if got := resp.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("has error = %t, want %t: %v", got, test.wantError, resp.Diagnostics)
			}
			if got := r.client != nil; got != test.wantClient {
				t.Fatalf("client configured = %t, want %t", got, test.wantClient)
			}
		})
	}
}

func TestOperationsRequireConfiguredClient(t *testing.T) {
	t.Parallel()

	r := &destinationResource{}
	prior := model(t, nil, nil)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "Missing Kernel Client" {
		t.Fatalf("diagnostics = %v, want Missing Kernel Client", resp.Diagnostics)
	}
}
