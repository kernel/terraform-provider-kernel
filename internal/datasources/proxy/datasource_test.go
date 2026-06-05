package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ proxyClient = kernelclient.Clients{}

type fakeProxyClient struct {
	defaultProjectID string
	get              func(context.Context, string, string) (*kernel.ProxyGetResponse, error)
	list             func(context.Context, string, int64) (kernelclient.ProxyPage, error)
}

func (f fakeProxyClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeProxyClient) GetProxy(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, projectID, id)
}

func (f fakeProxyClient) ListProxyPage(ctx context.Context, projectID string, offset int64) (kernelclient.ProxyPage, error) {
	if f.list == nil {
		return kernelclient.ProxyPage{}, errors.New("unexpected list")
	}
	return f.list(ctx, projectID, offset)
}

func TestDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()

	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_proxy" {
		t.Fatalf("TypeName = %q, want kernel_proxy", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "name", "type", "protocol"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s attribute", name)
		}
	}
	for _, name := range []string{"status", "last_checked", "ip_address", "config"} {
		if _, ok := schema.Schema.Attributes[name]; ok {
			t.Fatalf("schema should not include %s", name)
		}
	}
}

func TestReadSetsTerraformState(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0: proxyPage(listedProxyForTest("proxy-target", "Target")),
		}),
	})

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	req := datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    proxyConfigValue(tftypes.NewValue(tftypes.String, nil), tftypes.NewValue(tftypes.String, "Target")),
		},
	}
	resp := datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}

	ds.Read(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state proxyModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected state diagnostics: %v", resp.Diagnostics)
	}
	if state.ID.ValueString() != "proxy-target" {
		t.Fatalf("state id = %q, want proxy-target", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Target" {
		t.Fatalf("state name = %q, want Target", state.Name.ValueString())
	}
	if state.Type.ValueString() != "custom" {
		t.Fatalf("state type = %q, want custom", state.Type.ValueString())
	}
	if state.Protocol.ValueString() != "https" {
		t.Fatalf("state protocol = %q, want https", state.Protocol.ValueString())
	}
}

func TestReadResolvesProjectScope(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configProjectID  types.String
		defaultProjectID string
		wantProjectID    string
	}{
		"explicit attribute wins over provider default": {
			configProjectID:  types.StringValue("proj_attr"),
			defaultProjectID: "proj_default",
			wantProjectID:    "proj_attr",
		},
		"unset attribute inherits provider default": {
			configProjectID:  types.StringNull(),
			defaultProjectID: "proj_default",
			wantProjectID:    "proj_default",
		},
		"nothing set stays unscoped": {
			configProjectID:  types.StringNull(),
			defaultProjectID: "",
			wantProjectID:    "",
		},
	}

	t.Run("unknown attribute errors instead of reading the default", func(t *testing.T) {
		t.Parallel()

		ds := newDataSourceWithClient(fakeProxyClient{defaultProjectID: "proj_default"})
		_, diags := ds.read(context.Background(), proxyModel{
			ID:        types.StringValue("proxy-1"),
			ProjectID: types.StringUnknown(),
		})
		if !diags.HasError() {
			t.Fatal("expected diagnostics for unknown project_id")
		}
	})

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotProjectID string
			ds := newDataSourceWithClient(fakeProxyClient{
				defaultProjectID: test.defaultProjectID,
				get: func(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
					gotProjectID = projectID
					proxy := proxyForTest("proxy-1", "Proxy")
					return &proxy, nil
				},
			})

			state, diags := ds.read(context.Background(), proxyModel{
				ID:        types.StringValue("proxy-1"),
				ProjectID: test.configProjectID,
			})
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if gotProjectID != test.wantProjectID {
				t.Fatalf("GetProxy project = %q, want %q", gotProjectID, test.wantProjectID)
			}
			if !state.ProjectID.Equal(test.configProjectID) {
				t.Fatalf("state project_id = %v, want configured value %v echoed", state.ProjectID, test.configProjectID)
			}
		})
	}
}

func TestReadProxyByID(t *testing.T) {
	t.Parallel()

	var gotID string
	ds := newDataSourceWithClient(fakeProxyClient{
		get: func(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
			gotID = id
			proxy := proxyForTest("proxy-1", "Proxy")
			return &proxy, nil
		},
	})

	state, diags := ds.read(context.Background(), proxyModel{ID: types.StringValue("proxy-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotID != "proxy-1" {
		t.Fatalf("GetProxy id = %q, want proxy-1", gotID)
	}
	if state.Name.ValueString() != "Proxy" {
		t.Fatalf("state name = %q, want Proxy", state.Name.ValueString())
	}
}

func TestReadProxyByIDAllowsNullableName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		get: func(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
			proxy := proxyWithoutNameForTest("proxy-1")
			return &proxy, nil
		},
	})

	state, diags := ds.read(context.Background(), proxyModel{ID: types.StringValue("proxy-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Name.IsNull() {
		t.Fatalf("state name = %q, want null", state.Name.ValueString())
	}
}

func TestReadProxyByIDRejectsMismatchedID(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		get: func(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
			proxy := proxyForTest("proxy-actual", "Proxy")
			return &proxy, nil
		},
	})

	_, diags := ds.read(context.Background(), proxyModel{ID: types.StringValue("proxy-requested")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics when proxy ID lookup returns a different canonical ID")
	}
}

func TestReadLooksUpExactProxyName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0:   proxyPageWithNext(100, listedProxyForTest("proxy-other", "Other")),
			100: proxyPage(listedProxyForTest("proxy-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), proxyModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "proxy-target" {
		t.Fatalf("state id = %q, want proxy-target", state.ID.ValueString())
	}
}

func TestReadSkipsNamelessProxyLookupCandidates(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0:   proxyPageWithNext(100, listedProxyWithoutNameForTest("proxy-nameless")),
			100: proxyPage(listedProxyForTest("proxy-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), proxyModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "proxy-target" {
		t.Fatalf("state id = %q, want proxy-target", state.ID.ValueString())
	}
}

func TestReadRejectsMissingExactProxyName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0: proxyPage(listedProxyForTest("proxy-other", "Other")),
		}),
	})

	_, diags := ds.read(context.Background(), proxyModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing proxy name")
	}
}

func TestReadRejectsAmbiguousProxyName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0:   proxyPageWithNext(100, listedProxyForTest("proxy-a", "Target")),
			100: proxyPage(listedProxyForTest("proxy-b", "Target")),
		}),
	})

	_, diags := ds.read(context.Background(), proxyModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for ambiguous proxy name")
	}
}

func TestReadDeduplicatesRepeatedProxyLookupRows(t *testing.T) {
	t.Parallel()

	// Offset pagination can repeat a row across pages when the list shifts
	// mid-scan; the same proxy id twice is one match, not an ambiguity.
	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0:   proxyPageWithNext(100, listedProxyForTest("proxy-target", "Target")),
			100: proxyPage(listedProxyForTest("proxy-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), proxyModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "proxy-target" {
		t.Fatalf("state id = %q, want proxy-target", state.ID.ValueString())
	}
}

func TestReadRejectsInvalidProxyLookupMatch(t *testing.T) {
	t.Parallel()

	// A row that claims the requested name but has inconsistent raw JSON is a
	// broken API response for the proxy we would return — fail loud.
	invalid := listedProxyForTest("proxy-invalid", "Target")
	invalid.JSON.Name = respjson.NewInvalidField(`"Target"`)

	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0: proxyPage(invalid),
		}),
	})

	_, diags := ds.read(context.Background(), proxyModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid proxy lookup match")
	}
}

func TestReadSkipsMalformedUnrelatedProxyLookupRows(t *testing.T) {
	t.Parallel()

	// The proxy list is unfiltered, so every proxy in the project shares the
	// scan with the exact match. A malformed name on an unrelated row must be
	// skipped, not abort the lookup for the valid match.
	invalid := listedProxyForTest("proxy-invalid", "Other")
	invalid.Name = "123"
	invalid.JSON.Name = respjson.NewInvalidField("123")

	ds := newDataSourceWithClient(fakeProxyClient{
		list: listProxyPages(t, map[int64]kernelclient.ProxyPage{
			0: proxyPage(invalid, listedProxyForTest("proxy-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), proxyModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "proxy-target" {
		t.Fatalf("state id = %q, want proxy-target", state.ID.ValueString())
	}
}

func TestReadRejectsInvalidProxyResponseField(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{
		get: func(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
			proxy := proxyForTest("proxy-1", "Proxy")
			proxy.Protocol = ""
			proxy.JSON.Protocol = respjson.NewInvalidField("123")
			return &proxy, nil
		},
	})

	_, diags := ds.read(context.Background(), proxyModel{ID: types.StringValue("proxy-1")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid proxy response field")
	}
}

func TestReadRejectsMissingProxySelector(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProxyClient{})

	_, diags := ds.read(context.Background(), proxyModel{})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing proxy selector")
	}
}

func TestReadRejectsEmptyProxySelectors(t *testing.T) {
	t.Parallel()

	tests := map[string]proxyModel{
		"empty id": {
			ID: types.StringValue(""),
		},
		"empty name": {
			Name: types.StringValue(""),
		},
	}

	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			called := false
			ds := newDataSourceWithClient(fakeProxyClient{
				get: func(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
					called = true
					return nil, errors.New("should not read proxy")
				},
			})

			_, diags := ds.read(context.Background(), config)
			if !diags.HasError() {
				t.Fatal("expected diagnostics for empty selector")
			}
			if called {
				t.Fatal("GetProxy was called for empty selector")
			}
		})
	}
}

func proxyForTest(id, name string) kernel.ProxyGetResponse {
	var proxy kernel.ProxyGetResponse
	if err := json.Unmarshal([]byte(proxyJSONForTest(id, name)), &proxy); err != nil {
		panic(err)
	}
	return proxy
}

func proxyWithoutNameForTest(id string) kernel.ProxyGetResponse {
	var proxy kernel.ProxyGetResponse
	if err := json.Unmarshal([]byte(proxyJSONWithoutNameForTest(id)), &proxy); err != nil {
		panic(err)
	}
	return proxy
}

func listedProxyForTest(id, name string) kernel.ProxyListResponse {
	var proxy kernel.ProxyListResponse
	if err := json.Unmarshal([]byte(proxyJSONForTest(id, name)), &proxy); err != nil {
		panic(err)
	}
	return proxy
}

func listedProxyWithoutNameForTest(id string) kernel.ProxyListResponse {
	var proxy kernel.ProxyListResponse
	if err := json.Unmarshal([]byte(proxyJSONWithoutNameForTest(id)), &proxy); err != nil {
		panic(err)
	}
	return proxy
}

func proxyJSONForTest(id, name string) string {
	return `{"id":"` + id + `","name":"` + name + `","type":"custom","protocol":"https","status":"available","last_checked":"2026-06-05T12:00:00Z","ip_address":"203.0.113.10"}`
}

func proxyJSONWithoutNameForTest(id string) string {
	return `{"id":"` + id + `","name":null,"type":"custom","protocol":"https","status":"available","last_checked":"2026-06-05T12:00:00Z","ip_address":"203.0.113.10"}`
}

func proxyConfigValue(id, name tftypes.Value) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"id":         tftypes.String,
				"name":       tftypes.String,
				"project_id": tftypes.String,
				"type":       tftypes.String,
				"protocol":   tftypes.String,
			},
		},
		map[string]tftypes.Value{
			"id":         id,
			"name":       name,
			"project_id": tftypes.NewValue(tftypes.String, nil),
			"type":       tftypes.NewValue(tftypes.String, nil),
			"protocol":   tftypes.NewValue(tftypes.String, nil),
		},
	)
}

func listProxyPages(t *testing.T, pages map[int64]kernelclient.ProxyPage) func(context.Context, string, int64) (kernelclient.ProxyPage, error) {
	t.Helper()

	return func(ctx context.Context, projectID string, offset int64) (kernelclient.ProxyPage, error) {
		page, ok := pages[offset]
		if !ok {
			t.Fatalf("unexpected proxy page offset: %d", offset)
		}
		return page, nil
	}
}

func proxyPage(proxies ...kernel.ProxyListResponse) kernelclient.ProxyPage {
	return kernelclient.ProxyPage{Items: proxies}
}

func proxyPageWithNext(nextOffset int64, proxies ...kernel.ProxyListResponse) kernelclient.ProxyPage {
	return kernelclient.ProxyPage{
		Items:       proxies,
		NextOffset:  nextOffset,
		HasNextPage: true,
	}
}
