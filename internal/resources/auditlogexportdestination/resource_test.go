package auditlogexportdestination

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
	create func(context.Context, kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error)
	get    func(context.Context, string) (*kernel.AuditLogExportDestination, error)
	update func(context.Context, string, kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error)
	delete func(context.Context, string) error
}

func (f fakeDestinationClient) CreateAuditLogExportDestination(ctx context.Context, params kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
	if f.create == nil {
		return nil, errors.New("unexpected create")
	}
	return f.create(ctx, params)
}

func (f fakeDestinationClient) GetAuditLogExportDestination(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, id)
}

func (f fakeDestinationClient) UpdateAuditLogExportDestination(ctx context.Context, id string, params kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
	if f.update == nil {
		return nil, errors.New("unexpected update")
	}
	return f.update(ctx, id, params)
}

func (f fakeDestinationClient) DeleteAuditLogExportDestination(ctx context.Context, id string) error {
	if f.delete == nil {
		return errors.New("unexpected delete")
	}
	return f.delete(ctx, id)
}

func destinationResponse(t *testing.T, mutate func(*kernel.AuditLogExportDestination)) (*kernel.AuditLogExportDestination, error) {
	t.Helper()
	destination := destinationForTest(t, remoteDestination)
	if mutate != nil {
		mutate(&destination)
	}
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
		URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/audit-logs/export/destinations"},
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

	m := model(t)
	m.ID = types.StringUnknown()
	m.Status = types.StringUnknown()
	m.ExternalID = types.StringUnknown()
	m.KernelRoleARN = types.StringUnknown()
	return m
}

func TestCreatePersistsState(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		create: func(context.Context, kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
			return destinationResponse(t, nil)
		},
	})
	resp := &tfresource.CreateResponse{State: newState(t, nil)}

	r.Create(context.Background(), tfresource.CreateRequest{Plan: newPlan(t, plannedModel(t))}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := stateModel(t, resp.State), model(t); got != want {
		t.Fatalf("state = %+v, want %+v", got, want)
	}
}

func TestCreateActiveDestinationActivatesAfterCreate(t *testing.T) {
	t.Parallel()

	var gotID, gotBody string
	r := newResourceWithClient(fakeDestinationClient{
		create: func(context.Context, kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
			return destinationResponse(t, nil)
		},
		update: func(ctx context.Context, id string, params kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			gotID = id
			gotBody = marshal(t, params)
			return destinationResponse(t, func(d *kernel.AuditLogExportDestination) {
				d.Status = kernel.AuditLogExportDestinationStatusActive
			})
		},
	})
	plan := plannedModel(t)
	plan.Status = types.StringValue("active")
	resp := &tfresource.CreateResponse{State: newState(t, nil)}

	r.Create(context.Background(), tfresource.CreateRequest{Plan: newPlan(t, plan)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotID != "dest_123" || gotBody != `{"status":"active"}` {
		t.Fatalf("update %q with %s, want dest_123 with status active", gotID, gotBody)
	}
	if got := stateModel(t, resp.State).Status.ValueString(); got != "active" {
		t.Fatalf("status = %q, want active", got)
	}
}

func TestCreateKeepsStateWhenActivationFails(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response *kernel.AuditLogExportDestination
		err      error
	}{
		"transport error":    {err: errors.New("connection reset")},
		"empty response":     {},
		"malformed response": {response: &kernel.AuditLogExportDestination{ID: "dest_123"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newResourceWithClient(fakeDestinationClient{
				create: func(context.Context, kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
					return destinationResponse(t, nil)
				},
				update: func(context.Context, string, kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
					return test.response, test.err
				},
			})
			plan := plannedModel(t)
			plan.Status = types.StringValue("active")
			resp := &tfresource.CreateResponse{State: newState(t, nil)}

			r.Create(context.Background(), tfresource.CreateRequest{Plan: newPlan(t, plan)}, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected status error")
			}
			if got, want := stateModel(t, resp.State), model(t); got != want {
				t.Fatalf("state = %+v, want created destination %+v kept", got, want)
			}
		})
	}
}

func TestCreateFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		create      func() (*kernel.AuditLogExportDestination, error)
		wantSummary string
		wantDetail  string
	}{
		"definite API rejection": {
			create: func() (*kernel.AuditLogExportDestination, error) {
				return nil, apiError(t, http.StatusBadRequest, `{"code":"bad_request"}`)
			},
			wantSummary: "Create Kernel Audit Log Export Destination",
		},
		"project-scoped credential or plan": {
			create: func() (*kernel.AuditLogExportDestination, error) {
				return nil, apiError(t, http.StatusForbidden, `{"code":"forbidden"}`)
			},
			wantSummary: "Create Kernel Audit Log Export Destination",
			wantDetail:  "plan that includes audit logs",
		},
		"server error": {
			create: func() (*kernel.AuditLogExportDestination, error) {
				return nil, apiError(t, http.StatusInternalServerError, `{"code":"internal_error"}`)
			},
			wantSummary: "Kernel Audit Log Export Destination Creation Outcome Uncertain",
		},
		"transport error": {
			create: func() (*kernel.AuditLogExportDestination, error) {
				return nil, errors.New("connection reset")
			},
			wantSummary: "Kernel Audit Log Export Destination Creation Outcome Uncertain",
			wantDetail:  "connection reset",
		},
		"empty response": {
			create: func() (*kernel.AuditLogExportDestination, error) {
				return nil, nil
			},
			wantSummary: "Kernel Audit Log Export Destination Creation Outcome Uncertain",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newResourceWithClient(fakeDestinationClient{
				create: func(context.Context, kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
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
		get: func(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
			gotID = id
			return destinationResponse(t, func(d *kernel.AuditLogExportDestination) {
				d.Status = kernel.AuditLogExportDestinationStatusActive
			})
		},
	})
	prior := model(t)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got := stateModel(t, resp.State).Prefix; !got.Equal(prior.Prefix) {
		t.Fatalf("prefix = %v, want %v", got, prior.Prefix)
	}
	if gotID != "dest_123" {
		t.Fatalf("get id = %q, want dest_123", gotID)
	}
	if got := stateModel(t, resp.State).Status.ValueString(); got != "active" {
		t.Fatalf("status = %q, want active", got)
	}
}

func TestReadRemovesConfirmedMissingDestination(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		get: func(context.Context, string) (*kernel.AuditLogExportDestination, error) {
			return nil, apiError(t, http.StatusNotFound, `{"code":"not_found"}`)
		},
	})
	prior := model(t)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing destination was not removed from state")
	}
}

func TestReadRejectsCanonicalIDMismatch(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		get: func(context.Context, string) (*kernel.AuditLogExportDestination, error) {
			return destinationResponse(t, func(d *kernel.AuditLogExportDestination) { d.ID = "dest_456" })
		},
	})
	prior := model(t)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected ID mismatch error")
	}
	if got := stateModel(t, resp.State).ID.ValueString(); got != "dest_123" {
		t.Fatalf("state id = %q, want preserved dest_123", got)
	}
}

func TestUpdateSendsPatch(t *testing.T) {
	t.Parallel()

	var gotID, gotBody string
	r := newResourceWithClient(fakeDestinationClient{
		update: func(ctx context.Context, id string, params kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			gotID = id
			gotBody = marshal(t, params)
			return destinationResponse(t, func(d *kernel.AuditLogExportDestination) { d.Prefix = "audit" })
		},
	})
	prior := model(t)
	plan := model(t)
	plan.Prefix = types.StringValue("audit/")
	resp := &tfresource.UpdateResponse{State: newState(t, &prior)}

	r.Update(context.Background(), tfresource.UpdateRequest{Plan: newPlan(t, plan), State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotID != "dest_123" || gotBody != `{"prefix":"audit/"}` {
		t.Fatalf("update %q with %s, want dest_123 with prefix", gotID, gotBody)
	}
	if got := stateModel(t, resp.State); got != plan {
		t.Fatalf("state = %+v, want %+v", got, plan)
	}
}

func TestUpdateRejectsIDMismatch(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		update: func(context.Context, string, kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			return destinationResponse(t, func(d *kernel.AuditLogExportDestination) { d.ID = "dest_456" })
		},
	})
	prior := model(t)
	plan := model(t)
	plan.Bucket = types.StringValue("other-bucket")
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
	prior := model(t)
	resp := &tfresource.DeleteResponse{State: newState(t, &prior)}

	r.Delete(context.Background(), tfresource.DeleteRequest{State: newState(t, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

func TestDeleteReportsFailure(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeDestinationClient{
		delete: func(context.Context, string) error {
			return apiError(t, http.StatusForbidden, `{"code":"forbidden"}`)
		},
	})
	prior := model(t)
	resp := &tfresource.DeleteResponse{State: newState(t, &prior)}

	r.Delete(context.Background(), tfresource.DeleteRequest{State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected delete error")
	}
}

func TestImportStateSetsID(t *testing.T) {
	t.Parallel()

	resp := &tfresource.ImportStateResponse{State: newState(t, nil)}

	NewResource().(*destinationResource).ImportState(context.Background(), tfresource.ImportStateRequest{ID: "dest_123"}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got := stateModel(t, resp.State).ID.ValueString(); got != "dest_123" {
		t.Fatalf("state id = %q, want dest_123", got)
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
	prior := model(t)
	resp := &tfresource.ReadResponse{State: newState(t, &prior)}

	r.Read(context.Background(), tfresource.ReadRequest{State: newState(t, &prior)}, resp)

	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "Missing Kernel Client" {
		t.Fatalf("diagnostics = %v, want Missing Kernel Client", resp.Diagnostics)
	}
}
