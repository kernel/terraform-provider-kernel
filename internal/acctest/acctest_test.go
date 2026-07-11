package acctest

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"
)

type fakeBrowserPoolCleaner struct {
	defaultProjectID string
	delete           func(context.Context, string, string) error
}

type fakeProjectCleaner struct {
	delete func(context.Context, string) error
}

func (f fakeProjectCleaner) DeleteProject(ctx context.Context, id string) error {
	return f.delete(ctx, id)
}

func (f fakeBrowserPoolCleaner) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeBrowserPoolCleaner) DeleteBrowserPool(ctx context.Context, projectID, id string) error {
	if f.delete == nil {
		return nil
	}
	return f.delete(ctx, projectID, id)
}

type testRecorder struct {
	testing.TB
	failed   bool
	cleanups []func()
}

func (r *testRecorder) Helper() {}

func (r *testRecorder) Fatalf(format string, args ...any) {
	r.failed = true
}

func (r *testRecorder) Errorf(format string, args ...any) {
	r.failed = true
}

func (r *testRecorder) Cleanup(fn func()) {
	r.cleanups = append(r.cleanups, fn)
}

// notFoundAPIError builds a 404 the way the SDK does: Error() formats the
// request and response, so a bare struct without them panics.
func notFoundAPIError() *kernel.Error {
	return &kernel.Error{
		StatusCode: http.StatusNotFound,
		Request: &http.Request{
			Method: http.MethodDelete,
			URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/browser_pools/pool-1"},
		},
		Response: &http.Response{StatusCode: http.StatusNotFound},
	}
}

func TestProviderConfigUsesEnvironment(t *testing.T) {
	t.Parallel()

	if got, want := ProviderConfig(), "provider \"kernel\" {}\n"; got != want {
		t.Fatalf("ProviderConfig() = %q, want %q", got, want)
	}
}

func TestAcceptanceEnabledRequiresExplicitOptIn(t *testing.T) {
	t.Setenv(EnvAcceptance, "")
	if AcceptanceEnabled() {
		t.Fatal("AcceptanceEnabled() = true, want false without opt-in")
	}

	t.Setenv(EnvAcceptance, "true")
	if AcceptanceEnabled() {
		t.Fatal("AcceptanceEnabled() = true, want false for non-1 value")
	}

	t.Setenv(EnvAcceptance, "1")
	if !AcceptanceEnabled() {
		t.Fatal("AcceptanceEnabled() = false, want true for explicit opt-in")
	}
}

func TestCleanupBrowserPoolRequiresAcceptanceOptIn(t *testing.T) {
	t.Setenv(EnvAcceptance, "")
	t.Setenv(EnvAPIKey, "test-key")

	called := false
	recorder := &testRecorder{TB: t}
	cleanupBrowserPool(recorder, fakeBrowserPoolCleaner{
		delete: func(ctx context.Context, projectID, id string) error {
			called = true
			return nil
		},
	}, "", "pool-1")

	if !recorder.failed {
		t.Fatal("cleanupBrowserPool did not fail without acceptance opt-in")
	}
	if len(recorder.cleanups) != 0 {
		t.Fatalf("cleanupBrowserPool registered %d cleanups without acceptance opt-in", len(recorder.cleanups))
	}
	if called {
		t.Fatal("cleanupBrowserPool called delete without acceptance opt-in")
	}
}

func TestCleanupBrowserPoolRequiresAPIKey(t *testing.T) {
	t.Setenv(EnvAcceptance, "1")
	t.Setenv(EnvAPIKey, "")

	called := false
	recorder := &testRecorder{TB: t}
	cleanupBrowserPool(recorder, fakeBrowserPoolCleaner{
		delete: func(ctx context.Context, projectID, id string) error {
			called = true
			return nil
		},
	}, "", "pool-1")

	if !recorder.failed {
		t.Fatal("cleanupBrowserPool did not fail without API key")
	}
	if len(recorder.cleanups) != 0 {
		t.Fatalf("cleanupBrowserPool registered %d cleanups without API key", len(recorder.cleanups))
	}
	if called {
		t.Fatal("cleanupBrowserPool called delete without API key")
	}
}

func TestCleanupBrowserPoolIgnoresEmptyID(t *testing.T) {
	t.Setenv(EnvAcceptance, "")
	t.Setenv(EnvAPIKey, "")

	called := false
	recorder := &testRecorder{TB: t}
	cleanupBrowserPool(recorder, fakeBrowserPoolCleaner{
		delete: func(ctx context.Context, projectID, id string) error {
			called = true
			return nil
		},
	}, "", "")

	if recorder.failed {
		t.Fatal("cleanupBrowserPool failed for empty id")
	}
	if len(recorder.cleanups) != 0 {
		t.Fatalf("cleanupBrowserPool registered %d cleanups for empty id", len(recorder.cleanups))
	}
	if called {
		t.Fatal("cleanupBrowserPool called delete for empty id")
	}
}

func TestCleanupBrowserPoolRegistersDelete(t *testing.T) {
	t.Setenv(EnvAcceptance, "1")
	t.Setenv(EnvAPIKey, "test-key")

	tests := map[string]struct {
		projectID     string
		wantProjectID string
	}{
		"empty project falls back to the env default": {
			projectID:     "",
			wantProjectID: "proj_env",
		},
		"explicit project wins over the env default": {
			projectID:     "proj_x",
			wantProjectID: "proj_x",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var gotID, gotProjectID string
			recorder := &testRecorder{TB: t}
			cleanupBrowserPool(recorder, fakeBrowserPoolCleaner{
				defaultProjectID: "proj_env",
				delete: func(ctx context.Context, projectID, id string) error {
					gotProjectID = projectID
					gotID = id
					return notFoundAPIError()
				},
			}, test.projectID, "pool-1")

			if recorder.failed {
				t.Fatal("cleanupBrowserPool failed before cleanup ran")
			}
			if len(recorder.cleanups) != 1 {
				t.Fatalf("cleanupBrowserPool registered %d cleanups, want 1", len(recorder.cleanups))
			}

			recorder.cleanups[0]()
			if recorder.failed {
				t.Fatal("cleanupBrowserPool failed on 404 cleanup")
			}
			if gotID != "pool-1" {
				t.Fatalf("cleanup id = %q, want pool-1", gotID)
			}
			if gotProjectID != test.wantProjectID {
				t.Fatalf("cleanup project = %q, want %q", gotProjectID, test.wantProjectID)
			}
		})
	}
}

func TestCleanupBrowserPoolSurfacesRealDeleteErrors(t *testing.T) {
	t.Setenv(EnvAcceptance, "1")
	t.Setenv(EnvAPIKey, "test-key")

	// A 404 means the pool is already gone and is swallowed (covered by
	// TestCleanupBrowserPoolRegistersDelete); any other failure must surface.
	tests := map[string]error{
		"api error other than 404": &kernel.Error{
			StatusCode: http.StatusInternalServerError,
			Request: &http.Request{
				Method: http.MethodDelete,
				URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/browser_pools/pool-1"},
			},
			Response: &http.Response{StatusCode: http.StatusInternalServerError},
		},
		"transport error": errors.New("connection reset"),
	}

	for name, deleteErr := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := &testRecorder{TB: t}
			cleanupBrowserPool(recorder, fakeBrowserPoolCleaner{
				delete: func(ctx context.Context, projectID, id string) error {
					return deleteErr
				},
			}, "proj_x", "pool-1")

			if len(recorder.cleanups) != 1 {
				t.Fatalf("cleanupBrowserPool registered %d cleanups, want 1", len(recorder.cleanups))
			}
			recorder.cleanups[0]()
			if !recorder.failed {
				t.Fatal("cleanup swallowed a non-404 delete error")
			}
		})
	}
}

func TestCleanupProject(t *testing.T) {
	tests := map[string]struct {
		acceptance   string
		apiKey       string
		id           string
		deleteErr    error
		wantCleanups int
		wantDelete   bool
		wantFailure  bool
	}{
		"empty ID is ignored": {},
		"acceptance disabled": {
			apiKey:      "test-key",
			id:          "project_123",
			wantFailure: true,
		},
		"API key missing": {
			acceptance:  "1",
			id:          "project_123",
			wantFailure: true,
		},
		"not found is already clean": {
			acceptance:   "1",
			apiKey:       "test-key",
			id:           "project_123",
			deleteErr:    notFoundAPIError(),
			wantCleanups: 1,
			wantDelete:   true,
		},
		"delete error is reported": {
			acceptance:   "1",
			apiKey:       "test-key",
			id:           "project_123",
			deleteErr:    errors.New("connection reset"),
			wantCleanups: 1,
			wantDelete:   true,
			wantFailure:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvAcceptance, test.acceptance)
			t.Setenv(EnvAPIKey, test.apiKey)

			var gotID string
			deleteCalled := false
			deadlineSet := false
			recorder := &testRecorder{TB: t}
			cleanupProject(recorder, fakeProjectCleaner{
				delete: func(ctx context.Context, id string) error {
					deleteCalled = true
					_, deadlineSet = ctx.Deadline()
					gotID = id
					return test.deleteErr
				},
			}, test.id)

			if got, want := len(recorder.cleanups), test.wantCleanups; got != want {
				t.Fatalf("cleanupProject registered %d cleanups, want %d", got, want)
			}
			if test.wantCleanups == 1 {
				recorder.cleanups[0]()
			}
			if recorder.failed != test.wantFailure {
				t.Fatalf("cleanupProject failure = %t, want %t", recorder.failed, test.wantFailure)
			}
			if deleteCalled != test.wantDelete {
				t.Fatalf("cleanupProject called delete = %t, want %t", deleteCalled, test.wantDelete)
			}
			if deleteCalled && !deadlineSet {
				t.Fatal("cleanupProject called delete without a context deadline")
			}
			if test.wantDelete && gotID != test.id {
				t.Fatalf("cleanup id = %q, want %q", gotID, test.id)
			}
		})
	}
}

func TestUniqueNameIsKernelScopedAndSafe(t *testing.T) {
	t.Parallel()

	name := UniqueName(t, "Browser Pool: Create_Read")
	if len(name) > 63 {
		t.Fatalf("UniqueName length = %d, want <= 63: %s", len(name), name)
	}

	matched, err := regexp.MatchString(`^kernel-tf-browser-pool-create-read-[a-z0-9]+-[a-f0-9]{8}$`, name)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatalf("UniqueName() = %q, want safe Kernel test name", name)
	}
}

func TestUniqueNameTrimsLongPrefixes(t *testing.T) {
	t.Parallel()

	name := UniqueName(t, strings.Repeat("long-", 30))
	if len(name) > 63 {
		t.Fatalf("UniqueName length = %d, want <= 63: %s", len(name), name)
	}
	if !strings.HasPrefix(name, "kernel-tf-long") {
		t.Fatalf("UniqueName() = %q, want trimmed prefix preserved", name)
	}
}

func TestProtoV6ProviderFactoriesRegistersKernel(t *testing.T) {
	t.Parallel()

	factories := ProtoV6ProviderFactories()
	factory, ok := factories["kernel"]
	if !ok {
		t.Fatalf("missing kernel provider factory: %v", factories)
	}

	server, err := factory()
	if err != nil {
		t.Fatalf("provider factory returned error: %v", err)
	}
	if server == nil {
		t.Fatal("provider factory returned nil server")
	}
}
