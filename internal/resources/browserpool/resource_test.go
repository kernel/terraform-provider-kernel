package browserpool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ browserPoolClient = kernelclient.Clients{}

// apiErrorForTest builds a kernel.Error the way the SDK does: Error() formats
// the request and response, so a bare struct without them panics.
func apiErrorForTest(t *testing.T, status int, raw string) *kernel.Error {
	t.Helper()

	var err kernel.Error
	if unmarshalErr := json.Unmarshal([]byte(raw), &err); unmarshalErr != nil {
		t.Fatalf("unmarshal API error: %v", unmarshalErr)
	}
	err.StatusCode = status
	err.Request = &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/browser_pools"},
	}
	err.Response = &http.Response{StatusCode: status}
	return &err
}

type fakeBrowserPoolClient struct {
	defaultProjectID string
	create           func(context.Context, string, kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error)
	get              func(context.Context, string, string) (*kernel.BrowserPool, error)
	update           func(context.Context, string, string, kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error)
}

func (f fakeBrowserPoolClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeBrowserPoolClient) CreateBrowserPool(ctx context.Context, projectID string, params kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error) {
	if f.create == nil {
		return nil, errors.New("unexpected create")
	}
	return f.create(ctx, projectID, params)
}

func (f fakeBrowserPoolClient) GetBrowserPool(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, projectID, id)
}

func (f fakeBrowserPoolClient) UpdateBrowserPool(ctx context.Context, projectID, id string, params kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error) {
	if f.update == nil {
		return nil, errors.New("unexpected update")
	}
	return f.update(ctx, projectID, id, params)
}

func TestResourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	r := &browserPoolResource{}

	var metadata tfresource.MetadataResponse
	r.Metadata(context.Background(), tfresource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_browser_pool" {
		t.Fatalf("TypeName = %q, want kernel_browser_pool", metadata.TypeName)
	}

	var schema tfresource.SchemaResponse
	r.Schema(context.Background(), tfresource.SchemaRequest{}, &schema)
	if _, ok := schema.Schema.Attributes["size"]; !ok {
		t.Fatal("browser pool schema missing size attribute")
	}
}

func TestCreateBrowserPoolCreatesAndFlattensState(t *testing.T) {
	t.Parallel()

	plan := browserPoolModel{
		Name:         types.StringValue("pool-a"),
		Size:         types.Int64Value(1),
		ExtensionIDs: stringListForTest(),
		ChromePolicy: chromePolicyValueForTest(`{}`),
	}
	var gotParams kernel.BrowserPoolNewParams
	var gotProjectID string
	r := newResourceWithClient(fakeBrowserPoolClient{
		create: func(ctx context.Context, projectID string, params kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error) {
			gotProjectID = projectID
			gotParams = params
			pool := unmarshalBrowserPool(t, `{
				"id": "pool-1",
				"browser_pool_config": {
					"size": 1,
					"name": "pool-a",
					"timeout_seconds": 90,
					"fill_rate_per_minute": 0
				}
			}`)
			return &pool, nil
		},
	})

	state, diags := r.create(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, gotParams)
	want := map[string]any{
		"name":          "pool-a",
		"size":          float64(1),
		"extensions":    []any{},
		"chrome_policy": map[string]any{},
	}
	if !jsonEqual(body, want) {
		t.Fatalf("create params mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
	if state.ID.ValueString() != "pool-1" {
		t.Fatalf("state id = %q, want pool-1", state.ID.ValueString())
	}
	if gotProjectID != "" {
		t.Fatalf("create project = %q, want unscoped", gotProjectID)
	}
	if !state.ProjectID.IsNull() {
		t.Fatalf("state project_id = %v, want null for an unscoped pool", state.ProjectID)
	}
	assertStringList(t, state.ExtensionIDs, []string{})
	if state.ChromePolicy.ValueString() != `{}` {
		t.Fatalf("chrome_policy = %q, want explicit empty object preserved", state.ChromePolicy.ValueString())
	}
}

func TestCreateBrowserPoolResolvesProjectScope(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		planProjectID    types.String
		defaultProjectID string
		wantProjectID    string
	}{
		"explicit attribute wins over provider default": {
			planProjectID:    types.StringValue("proj_attr"),
			defaultProjectID: "proj_default",
			wantProjectID:    "proj_attr",
		},
		"unset attribute inherits provider default": {
			planProjectID:    types.StringNull(),
			defaultProjectID: "proj_default",
			wantProjectID:    "proj_default",
		},
		"unknown attribute inherits provider default": {
			planProjectID:    types.StringUnknown(),
			defaultProjectID: "proj_default",
			wantProjectID:    "proj_default",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotProjectID string
			r := newResourceWithClient(fakeBrowserPoolClient{
				defaultProjectID: test.defaultProjectID,
				create: func(ctx context.Context, projectID string, params kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error) {
					gotProjectID = projectID
					pool := unmarshalBrowserPool(t, `{
						"id": "pool-1",
						"browser_pool_config": {"size": 1}
					}`)
					return &pool, nil
				},
			})

			state, diags := r.create(context.Background(), browserPoolModel{
				ProjectID:    test.planProjectID,
				Size:         types.Int64Value(1),
				ExtensionIDs: stringListForTest(),
				ChromePolicy: chromePolicyValueForTest(`{}`),
			})
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if gotProjectID != test.wantProjectID {
				t.Fatalf("create project = %q, want %q", gotProjectID, test.wantProjectID)
			}
			if state.ProjectID.ValueString() != test.wantProjectID {
				t.Fatalf("state project_id = %q, want resolved project %q recorded in state", state.ProjectID.ValueString(), test.wantProjectID)
			}
		})
	}
}

func TestCreateBrowserPoolMapsProjectAccessDenied(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeBrowserPoolClient{
		defaultProjectID: "proj_default",
		create: func(ctx context.Context, projectID string, params kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error) {
			return nil, apiErrorForTest(t, http.StatusForbidden, "{}")
		},
	})

	_, diags := r.create(context.Background(), browserPoolModel{
		Size:         types.Int64Value(1),
		ExtensionIDs: stringListForTest(),
		ChromePolicy: chromePolicyValueForTest(`{}`),
	})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for forbidden project")
	}
	found := false
	for _, d := range diags.Errors() {
		if strings.Contains(d.Detail(), "cannot access project proj_default") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected project access diagnostic, got: %v", diags)
	}
}

func TestCreateBrowserPoolReturnsAPIDiagnostics(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeBrowserPoolClient{
		create: func(ctx context.Context, projectID string, params kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error) {
			return nil, errors.New("api failed")
		},
	})

	_, diags := r.create(context.Background(), browserPoolModel{Size: types.Int64Value(1)})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for create error")
	}
}

func TestReadBrowserPoolRefreshesByStateID(t *testing.T) {
	t.Parallel()

	var gotID string
	var gotProjectID string
	r := newResourceWithClient(fakeBrowserPoolClient{
		defaultProjectID: "proj_default",
		get: func(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
			gotProjectID = projectID
			gotID = id
			pool := unmarshalBrowserPool(t, `{
				"id": "pool-1",
				"browser_pool_config": {
					"size": 2,
					"name": "pool-a"
				}
			}`)
			return &pool, nil
		},
	})

	state, removed, diags := r.read(context.Background(), browserPoolModel{
		ID:             types.StringValue("pool-1"),
		ExtensionIDs:   stringListForTest(),
		ChromePolicy:   chromePolicyValueForTest(`{}`),
		TimeoutSeconds: types.Int64Null(),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if removed {
		t.Fatal("removed = true, want false")
	}
	if gotID != "pool-1" {
		t.Fatalf("GetBrowserPool id = %q, want pool-1", gotID)
	}
	if gotProjectID != "" {
		t.Fatalf("read project = %q, want unscoped", gotProjectID)
	}
	if state.Size.ValueInt64() != 2 {
		t.Fatalf("size = %d, want 2", state.Size.ValueInt64())
	}
	assertStringList(t, state.ExtensionIDs, []string{})
	if state.ChromePolicy.ValueString() != `{}` {
		t.Fatalf("chrome_policy = %q, want explicit empty object preserved", state.ChromePolicy.ValueString())
	}
}

func TestReadBrowserPoolUsesStateProject(t *testing.T) {
	t.Parallel()

	var gotProjectID string
	r := newResourceWithClient(fakeBrowserPoolClient{
		defaultProjectID: "proj_default",
		get: func(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
			gotProjectID = projectID
			pool := unmarshalBrowserPool(t, `{
				"id": "pool-1",
				"browser_pool_config": {"size": 1}
			}`)
			return &pool, nil
		},
	})

	state, removed, diags := r.read(context.Background(), browserPoolModel{
		ID:           types.StringValue("pool-1"),
		ProjectID:    types.StringValue("proj_a"),
		ExtensionIDs: stringListForTest(),
		ChromePolicy: chromePolicyValueForTest(`{}`),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if removed {
		t.Fatal("removed = true, want false")
	}
	if gotProjectID != "proj_a" {
		t.Fatalf("read project = %q, want proj_a from state", gotProjectID)
	}
	if state.ProjectID.ValueString() != "proj_a" {
		t.Fatalf("state project_id = %q, want proj_a preserved", state.ProjectID.ValueString())
	}
}

func TestReadBrowserPoolRemovesStateOnNotFound(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeBrowserPoolClient{
		get: func(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
			return nil, apiErrorForTest(t, http.StatusNotFound, `{"code":"not_found","message":"browser pool not found"}`)
		},
	})

	_, removed, diags := r.read(context.Background(), browserPoolModel{ID: types.StringValue("pool-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !removed {
		t.Fatal("removed = false, want true")
	}
}

func TestReadBrowserPoolDoesNotRemoveStateOnProjectNotFound(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeBrowserPoolClient{
		get: func(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
			return nil, apiErrorForTest(t, http.StatusNotFound, `{"code":"project_not_found","message":"Project not found or inactive"}`)
		},
	})

	_, removed, diags := r.read(context.Background(), browserPoolModel{
		ID:        types.StringValue("pool-1"),
		ProjectID: types.StringValue("proj_a"),
	})
	if removed {
		t.Fatal("a project-level 404 must not remove the pool from state")
	}
	if !diags.HasError() {
		t.Fatal("expected diagnostics for project-level 404")
	}
	found := false
	for _, d := range diags.Errors() {
		if strings.Contains(d.Detail(), "project proj_a was not found or is inactive") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected project-not-found diagnostic, got: %v", diags)
	}
}

func TestReadBrowserPoolRejectsMissingStateID(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeBrowserPoolClient{})

	_, _, diags := r.read(context.Background(), browserPoolModel{ID: types.StringNull()})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing id")
	}
}

func TestReadBrowserPoolRejectsEmptyStateID(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeBrowserPoolClient{
		get: func(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
			called = true
			return nil, errors.New("should not call get")
		},
	})

	_, _, diags := r.read(context.Background(), browserPoolModel{ID: types.StringValue("")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for empty id")
	}
	if called {
		t.Fatal("GetBrowserPool was called for empty id")
	}
}

func TestUpdateBrowserPoolPatchesStateIDAndReadsAfterUpdate(t *testing.T) {
	t.Parallel()

	plan := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(2),
		ProfileID:         types.StringNull(),
		ProxyID:           types.StringNull(),
		ExtensionIDs:      types.ListNull(types.StringType),
		ChromePolicy:      chromePolicyNull(),
		Viewport:          types.ObjectNull(viewportAttrTypes()),
		StartURL:          types.StringValue("https://new.example"),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}
	state := browserPoolModel{
		ID:                types.StringValue("pool-1"),
		Name:              types.StringValue("pool-a"),
		ProjectID:         types.StringValue("proj_a"),
		Size:              types.Int64Value(1),
		ProfileID:         types.StringNull(),
		ProxyID:           types.StringNull(),
		ExtensionIDs:      types.ListNull(types.StringType),
		ChromePolicy:      chromePolicyNull(),
		Viewport:          types.ObjectNull(viewportAttrTypes()),
		StartURL:          types.StringValue("https://old.example"),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}

	var calls []string
	var gotID string
	var gotProjectID string
	var gotParams kernel.BrowserPoolUpdateParams
	r := newResourceWithClient(fakeBrowserPoolClient{
		update: func(ctx context.Context, projectID, id string, params kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error) {
			calls = append(calls, "update")
			gotID = id
			gotProjectID = projectID
			gotParams = params
			pool := unmarshalBrowserPool(t, `{
				"id": "pool-1",
				"browser_pool_config": {
					"size": 2,
					"name": "ignored-update-response"
				}
			}`)
			return &pool, nil
		},
		get: func(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
			calls = append(calls, "get")
			if id != "pool-1" {
				t.Fatalf("GetBrowserPool id = %q, want pool-1", id)
			}
			pool := unmarshalBrowserPool(t, `{
				"id": "pool-1",
				"browser_pool_config": {
					"size": 2,
					"name": "pool-a",
					"start_url": "https://new.example",
					"headless": true,
					"kiosk_mode": false,
					"stealth": false,
					"timeout_seconds": 90,
					"fill_rate_per_minute": 10
				}
			}`)
			return &pool, nil
		},
	})

	nextState, diags := r.update(context.Background(), plan, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if gotID != "pool-1" {
		t.Fatalf("UpdateBrowserPool id = %q, want pool-1", gotID)
	}
	if gotProjectID != "proj_a" {
		t.Fatalf("UpdateBrowserPool project = %q, want proj_a from state", gotProjectID)
	}
	body := marshalSDKParams(t, gotParams)
	want := map[string]any{
		"size":      float64(2),
		"start_url": "https://new.example",
	}
	if !jsonEqual(body, want) {
		t.Fatalf("update params mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
	if !reflect.DeepEqual(calls, []string{"update", "get"}) {
		t.Fatalf("calls = %#v, want update then get", calls)
	}
	if nextState.Size.ValueInt64() != 2 {
		t.Fatalf("size = %d, want 2", nextState.Size.ValueInt64())
	}
	if nextState.StartURL.ValueString() != "https://new.example" {
		t.Fatalf("start_url = %q, want https://new.example", nextState.StartURL.ValueString())
	}
}

func TestUpdateBrowserPoolUsesPlanAsReadBaseToAvoidEmptyValueDrift(t *testing.T) {
	t.Parallel()

	plan := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(1),
		ExtensionIDs:      stringListForTest(),
		ChromePolicy:      chromePolicyValueForTest(`{}`),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}
	state := browserPoolModel{
		ID:                types.StringValue("pool-1"),
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(1),
		ExtensionIDs:      stringListForTest("ext-a"),
		ChromePolicy:      chromePolicyValueForTest(`{"HomepageLocation":"https://example.com"}`),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}

	var gotParams kernel.BrowserPoolUpdateParams
	r := newResourceWithClient(fakeBrowserPoolClient{
		update: func(ctx context.Context, projectID, id string, params kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error) {
			gotParams = params
			pool := unmarshalBrowserPool(t, `{
				"id": "pool-1",
				"browser_pool_config": {
					"size": 1,
					"name": "pool-a"
				}
			}`)
			return &pool, nil
		},
		get: func(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
			pool := unmarshalBrowserPool(t, `{
				"id": "pool-1",
				"browser_pool_config": {
					"size": 1,
					"name": "pool-a",
					"headless": true,
					"kiosk_mode": false,
					"stealth": false,
					"timeout_seconds": 90,
					"fill_rate_per_minute": 10
				}
			}`)
			return &pool, nil
		},
	})

	nextState, diags := r.update(context.Background(), plan, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, gotParams)
	want := map[string]any{
		"extensions":    []any{},
		"chrome_policy": map[string]any{},
	}
	if !jsonEqual(body, want) {
		t.Fatalf("update params mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
	assertStringList(t, nextState.ExtensionIDs, []string{})
	if nextState.ChromePolicy.ValueString() != `{}` {
		t.Fatalf("chrome_policy = %q, want explicit empty object preserved", nextState.ChromePolicy.ValueString())
	}
}

func TestUpdateBrowserPoolReturnsAPIDiagnostics(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeBrowserPoolClient{
		update: func(ctx context.Context, projectID, id string, params kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error) {
			return nil, errors.New("api failed")
		},
	})

	_, diags := r.update(
		context.Background(),
		browserPoolModel{Size: types.Int64Value(2)},
		browserPoolModel{ID: types.StringValue("pool-1"), Size: types.Int64Value(1)},
	)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for update error")
	}
}

func TestUpdateBrowserPoolRejectsMissingStateID(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeBrowserPoolClient{
		update: func(ctx context.Context, projectID, id string, params kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error) {
			called = true
			return nil, errors.New("should not call update")
		},
	})

	_, diags := r.update(
		context.Background(),
		browserPoolModel{Size: types.Int64Value(2)},
		browserPoolModel{ID: types.StringNull(), Size: types.Int64Value(1)},
	)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing id")
	}
	if called {
		t.Fatal("UpdateBrowserPool was called for missing id")
	}
}
