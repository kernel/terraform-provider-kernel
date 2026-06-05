package profile

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ profileClient = kernelclient.Clients{}

type fakeProfileClient struct {
	defaultProjectID string
	get              func(context.Context, string, string) (*kernel.Profile, error)
	list             func(context.Context, string, string, int64) (kernelclient.ProfilePage, error)
}

func (f fakeProfileClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeProfileClient) GetProfile(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, projectID, idOrName)
}

func (f fakeProfileClient) ListProfilePage(ctx context.Context, projectID, query string, offset int64) (kernelclient.ProfilePage, error) {
	if f.list == nil {
		return kernelclient.ProfilePage{}, errors.New("unexpected list")
	}
	return f.list(ctx, projectID, query, offset)
}

func TestDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()

	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_profile" {
		t.Fatalf("TypeName = %q, want kernel_profile", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "name", "project_id", "created_at"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s attribute", name)
		}
	}
	if _, ok := schema.Schema.Attributes["updated_at"]; ok {
		t.Fatal("schema should not include runtime updated_at")
	}
	if _, ok := schema.Schema.Attributes["last_used_at"]; ok {
		t.Fatal("schema should not include runtime last_used_at")
	}
}

func TestReadSetsTerraformState(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{
		list: listProfilePages(t, "Target", map[int64]kernelclient.ProfilePage{
			0: profilePage(profileForTest("profile-target", "Target")),
		}),
	})

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	req := datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    profileConfigValue(tftypes.NewValue(tftypes.String, nil), tftypes.NewValue(tftypes.String, "Target")),
		},
	}
	resp := datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}

	ds.Read(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state profileModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected state diagnostics: %v", resp.Diagnostics)
	}
	if state.ID.ValueString() != "profile-target" {
		t.Fatalf("state id = %q, want profile-target", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Target" {
		t.Fatalf("state name = %q, want Target", state.Name.ValueString())
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

		ds := newDataSourceWithClient(fakeProfileClient{defaultProjectID: "proj_default"})
		_, diags := ds.read(context.Background(), profileModel{
			ID:        types.StringValue("profile-1"),
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
			ds := newDataSourceWithClient(fakeProfileClient{
				defaultProjectID: test.defaultProjectID,
				get: func(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
					gotProjectID = projectID
					profile := profileForTest("profile-1", "Profile")
					return &profile, nil
				},
			})

			state, diags := ds.read(context.Background(), profileModel{
				ID:        types.StringValue("profile-1"),
				ProjectID: test.configProjectID,
			})
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if gotProjectID != test.wantProjectID {
				t.Fatalf("GetProfile project = %q, want %q", gotProjectID, test.wantProjectID)
			}
			if !state.ProjectID.Equal(test.configProjectID) {
				t.Fatalf("state project_id = %v, want configured value %v echoed", state.ProjectID, test.configProjectID)
			}
		})
	}
}

func TestReadProfileByID(t *testing.T) {
	t.Parallel()

	var gotID string
	ds := newDataSourceWithClient(fakeProfileClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
			gotID = idOrName
			profile := profileForTest("profile-1", "Profile")
			return &profile, nil
		},
	})

	state, diags := ds.read(context.Background(), profileModel{ID: types.StringValue("profile-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotID != "profile-1" {
		t.Fatalf("GetProfile id = %q, want profile-1", gotID)
	}
	if state.Name.ValueString() != "Profile" {
		t.Fatalf("state name = %q, want Profile", state.Name.ValueString())
	}
}

func TestReadProfileByIDAllowsNullableName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
			profile := profileWithoutNameForTest("profile-1")
			return &profile, nil
		},
	})

	state, diags := ds.read(context.Background(), profileModel{ID: types.StringValue("profile-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Name.IsNull() {
		t.Fatalf("state name = %q, want null", state.Name.ValueString())
	}
}

func TestReadProfileByIDRejectsNameMatch(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
			profile := profileForTest("profile-actual", idOrName)
			return &profile, nil
		},
	})

	_, diags := ds.read(context.Background(), profileModel{ID: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics when profile ID lookup returns a different canonical ID")
	}
}

func TestReadLooksUpExactProfileName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{
		list: listProfilePages(t, "Target", map[int64]kernelclient.ProfilePage{
			0:   profilePageWithNext(100, profileForTest("profile-other", "Other")),
			100: profilePage(profileForTest("profile-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), profileModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "profile-target" {
		t.Fatalf("state id = %q, want profile-target", state.ID.ValueString())
	}
}

func TestReadRejectsMissingExactProfileName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{
		list: listProfilePages(t, "Target", map[int64]kernelclient.ProfilePage{
			0: profilePage(profileForTest("profile-other", "Other")),
		}),
	})

	_, diags := ds.read(context.Background(), profileModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing profile name")
	}
}

func TestReadRejectsAmbiguousProfileName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{
		list: listProfilePages(t, "Target", map[int64]kernelclient.ProfilePage{
			0:   profilePageWithNext(100, profileForTest("profile-a", "Target")),
			100: profilePage(profileForTest("profile-b", "Target")),
		}),
	})

	_, diags := ds.read(context.Background(), profileModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for ambiguous profile name")
	}
}

func TestReadDeduplicatesRepeatedProfileLookupRows(t *testing.T) {
	t.Parallel()

	// Offset pagination can repeat a row across pages when the list shifts
	// mid-scan; the same profile id twice is one match, not an ambiguity.
	ds := newDataSourceWithClient(fakeProfileClient{
		list: listProfilePages(t, "Target", map[int64]kernelclient.ProfilePage{
			0:   profilePageWithNext(100, profileForTest("profile-target", "Target")),
			100: profilePage(profileForTest("profile-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), profileModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "profile-target" {
		t.Fatalf("state id = %q, want profile-target", state.ID.ValueString())
	}
}

func TestReadRejectsInvalidProfileLookupMatch(t *testing.T) {
	t.Parallel()

	// A row that claims the requested name but has inconsistent raw JSON is a
	// broken API response for the profile we would return — fail loud.
	invalid := profileForTest("profile-invalid", "Target")
	invalid.JSON.Name = respjson.NewInvalidField(`"Target"`)

	ds := newDataSourceWithClient(fakeProfileClient{
		list: listProfilePages(t, "Target", map[int64]kernelclient.ProfilePage{
			0: profilePage(invalid),
		}),
	})

	_, diags := ds.read(context.Background(), profileModel{Name: types.StringValue("Target")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid profile lookup match")
	}
}

func TestReadSkipsMalformedUnrelatedProfileLookupRows(t *testing.T) {
	t.Parallel()

	// The server name query is fuzzy, so unrelated rows share the page with
	// the exact match. A malformed name on an unrelated row must be skipped,
	// not abort the lookup for the valid match.
	invalid := profileForTest("profile-invalid", "Target Staging")
	invalid.Name = "123"
	invalid.JSON.Name = respjson.NewInvalidField("123")

	ds := newDataSourceWithClient(fakeProfileClient{
		list: listProfilePages(t, "Target", map[int64]kernelclient.ProfilePage{
			0: profilePage(invalid, profileForTest("profile-target", "Target")),
		}),
	})

	state, diags := ds.read(context.Background(), profileModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "profile-target" {
		t.Fatalf("state id = %q, want profile-target", state.ID.ValueString())
	}
}

func TestReadRejectsInvalidProfileResponseField(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
			profile := profileForTest("profile-1", "Profile")
			profile.CreatedAt = time.Time{}
			profile.JSON.CreatedAt = respjson.NewInvalidField("123")
			return &profile, nil
		},
	})

	_, diags := ds.read(context.Background(), profileModel{ID: types.StringValue("profile-1")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid profile response field")
	}
}

func TestReadRejectsMissingProfileSelector(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeProfileClient{})

	_, diags := ds.read(context.Background(), profileModel{})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing profile selector")
	}
}

func TestReadRejectsEmptyProfileSelectors(t *testing.T) {
	t.Parallel()

	tests := map[string]profileModel{
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
			ds := newDataSourceWithClient(fakeProfileClient{
				get: func(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
					called = true
					return nil, errors.New("should not read profile")
				},
			})

			_, diags := ds.read(context.Background(), config)
			if !diags.HasError() {
				t.Fatal("expected diagnostics for empty selector")
			}
			if called {
				t.Fatal("GetProfile was called for empty selector")
			}
		})
	}
}

func profileForTest(id, name string) kernel.Profile {
	return profileFromJSON(`{"id":"` + id + `","name":"` + name + `","created_at":"2026-06-05T12:00:00Z","updated_at":"2026-06-05T12:00:00Z","last_used_at":"2026-06-05T12:00:00Z"}`)
}

func profileWithoutNameForTest(id string) kernel.Profile {
	return profileFromJSON(`{"id":"` + id + `","name":null,"created_at":"2026-06-05T12:00:00Z","updated_at":"2026-06-05T12:00:00Z","last_used_at":"2026-06-05T12:00:00Z"}`)
}

func profileFromJSON(body string) kernel.Profile {
	var profile kernel.Profile
	if err := json.Unmarshal([]byte(body), &profile); err != nil {
		panic(err)
	}
	return profile
}

func profileConfigValue(id, name tftypes.Value) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"id":         tftypes.String,
				"name":       tftypes.String,
				"project_id": tftypes.String,
				"created_at": tftypes.String,
			},
		},
		map[string]tftypes.Value{
			"id":         id,
			"name":       name,
			"project_id": tftypes.NewValue(tftypes.String, nil),
			"created_at": tftypes.NewValue(tftypes.String, nil),
		},
	)
}

func listProfilePages(t *testing.T, wantQuery string, pages map[int64]kernelclient.ProfilePage) func(context.Context, string, string, int64) (kernelclient.ProfilePage, error) {
	t.Helper()

	return func(ctx context.Context, projectID, query string, offset int64) (kernelclient.ProfilePage, error) {
		if query != wantQuery {
			t.Fatalf("ListProfilePage query = %q, want %q", query, wantQuery)
		}

		page, ok := pages[offset]
		if !ok {
			t.Fatalf("unexpected profile page offset: %d", offset)
		}
		return page, nil
	}
}

func profilePage(profiles ...kernel.Profile) kernelclient.ProfilePage {
	return kernelclient.ProfilePage{Items: profiles}
}

func profilePageWithNext(nextOffset int64, profiles ...kernel.Profile) kernelclient.ProfilePage {
	return kernelclient.ProfilePage{
		Items:       profiles,
		NextOffset:  nextOffset,
		HasNextPage: true,
	}
}
