package kernelclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestClientsSendProjectHeaderOnlyWhenExplicitlyScoped(t *testing.T) {
	t.Parallel()

	var requests []capturedRequest
	clients := New(Config{
		APIKey:    "test-api-key",
		BaseURL:   "https://api.example",
		ProjectID: "default_project",
	}, WithHTTPClient(recordingHTTPClient(&requests, func(req *http.Request) string {
		switch req.URL.Path {
		case "/org/projects/project_123":
			return `{"id":"project_123","name":"Project","status":"active","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
		case "/profiles/profile_123", "/profiles/profile_456":
			return `{"id":"profile_123","name":"Profile","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","last_used_at":"2026-01-01T00:00:00Z"}`
		case "/proxies/proxy_123":
			return `{"id":"proxy_123","type":"datacenter","status":"active"}`
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return `{}`
		}
	})))

	if got, want := clients.DefaultProjectID(), "default_project"; got != want {
		t.Fatalf("DefaultProjectID() = %q, want %q", got, want)
	}

	if _, err := clients.GetProject(context.Background(), "project_123"); err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}
	if _, err := clients.GetProfile(context.Background(), "project_123", "profile_123"); err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if _, err := clients.GetProfile(context.Background(), "", "profile_456"); err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if _, err := clients.GetProxy(context.Background(), "project_123", "proxy_123"); err != nil {
		t.Fatalf("GetProxy returned error: %v", err)
	}

	if got, want := requests[0].ProjectID, ""; got != want {
		t.Fatalf("org-scoped request header = %q, want %q", got, want)
	}
	if got, want := requests[0].Authorization, "Bearer test-api-key"; got != want {
		t.Fatalf("project authorization header = %q, want %q", got, want)
	}
	if got, want := requests[1].ProjectID, "project_123"; got != want {
		t.Fatalf("scoped profile request header = %q, want %q", got, want)
	}
	if got, want := requests[1].Host, "api.example"; got != want {
		t.Fatalf("profile host = %q, want %q", got, want)
	}
	if !requests[1].HasDeadline {
		t.Fatal("project-scoped request should have a finite timeout deadline")
	}
	if got, want := requests[2].ProjectID, ""; got != want {
		t.Fatalf("unscoped profile request header = %q, want %q", got, want)
	}
	if got, want := requests[3].ProjectID, "project_123"; got != want {
		t.Fatalf("scoped proxy request header = %q, want %q", got, want)
	}
}

func TestClientsDoNotReadSDKEnvironmentDefaults(t *testing.T) {
	t.Setenv("KERNEL_BASE_URL", "https://env.example")
	t.Setenv("KERNEL_API_KEY", "env-api-key")
	t.Setenv("KERNEL_CUSTOM_HEADERS", "X-Kernel-Project-Id: env-project")

	var requests []capturedRequest
	clients := New(Config{
		APIKey: "config-api-key",
	}, WithHTTPClient(recordingHTTPClient(&requests, func(req *http.Request) string {
		if req.URL.Path != "/org/projects/project_123" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		return `{"id":"project_123","name":"Project","status":"active","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	})))

	if _, err := clients.GetProject(context.Background(), "project_123"); err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}

	if got, want := requests[0].Host, "api.onkernel.com"; got != want {
		t.Fatalf("host = %q, want %q", got, want)
	}
	if got, want := requests[0].Authorization, "Bearer config-api-key"; got != want {
		t.Fatalf("authorization header = %q, want %q", got, want)
	}
	if got, want := requests[0].ProjectID, ""; got != want {
		t.Fatalf("project header = %q, want %q", got, want)
	}
}

func TestBrowserPoolMutationsDisableSDKRetries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		method string
		path   string
		call   func(context.Context, Clients) error
	}{
		"create": {
			method: http.MethodPost,
			path:   "/browser_pools",
			call: func(ctx context.Context, clients Clients) error {
				_, err := clients.CreateBrowserPool(ctx, "project_123", kernel.BrowserPoolNewParams{Size: 1})
				return err
			},
		},
		"update": {
			method: http.MethodPatch,
			path:   "/browser_pools/pool_123",
			call: func(ctx context.Context, clients Clients) error {
				_, err := clients.UpdateBrowserPool(ctx, "project_123", "pool_123", kernel.BrowserPoolUpdateParams{
					Size: kernel.Int(2),
				})
				return err
			},
		},
		"delete": {
			method: http.MethodDelete,
			path:   "/browser_pools/pool_123",
			call: func(ctx context.Context, clients Clients) error {
				return clients.DeleteBrowserPool(ctx, "project_123", "pool_123")
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requests []capturedRequest
			clients := New(Config{
				APIKey:  "test-api-key",
				BaseURL: "https://api.example",
			}, WithHTTPClient(recordingHTTPClientWithStatus(&requests, http.StatusInternalServerError, func(req *http.Request) string {
				if req.Method != test.method {
					t.Fatalf("method = %s, want %s", req.Method, test.method)
				}
				if req.URL.Path != test.path {
					t.Fatalf("path = %s, want %s", req.URL.Path, test.path)
				}
				return `{"error":{"message":"retryable failure"}}`
			})))

			if err := test.call(context.Background(), clients); err == nil {
				t.Fatal("expected retryable API error")
			}

			if got, want := len(requests), 1; got != want {
				t.Fatalf("request count = %d, want %d", got, want)
			}
			if got, want := requests[0].RetryCount, "0"; got != want {
				t.Fatalf("retry count header = %q, want %q", got, want)
			}
			if got, want := requests[0].ProjectID, "project_123"; got != want {
				t.Fatalf("project header = %q, want %q", got, want)
			}
		})
	}
}

func TestDeleteBrowserPoolDoesNotForceRuntimeCleanup(t *testing.T) {
	t.Parallel()

	var requestBody map[string]bool
	clients := New(Config{
		APIKey:  "test-api-key",
		BaseURL: "https://api.example",
	}, WithHTTPClient(recordingHTTPClient(nil, func(req *http.Request) string {
		if req.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", req.Method)
		}
		if req.URL.Path != "/browser_pools/pool_123" {
			t.Fatalf("path = %s, want /browser_pools/pool_123", req.URL.Path)
		}
		if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return `{}`
	})))

	if err := clients.DeleteBrowserPool(context.Background(), "", "pool_123"); err != nil {
		t.Fatalf("DeleteBrowserPool returned error: %v", err)
	}

	force, ok := requestBody["force"]
	if !ok {
		t.Fatal("force field is missing")
	}
	if force {
		t.Fatal("force = true, want false")
	}
}

func TestClientsDoNotExposeRuntimeBrowserPoolMethods(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(Clients{})
	for _, name := range []string{"Acquire", "Release", "Flush"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Fatalf("Clients exposes runtime method %s", name)
		}
	}
}

type capturedRequest struct {
	Method        string
	Path          string
	Host          string
	Authorization string
	ProjectID     string
	RetryCount    string
	HasDeadline   bool
}

func recordingHTTPClient(requests *[]capturedRequest, responseBody func(*http.Request) string) *http.Client {
	return recordingHTTPClientWithStatus(requests, http.StatusOK, responseBody)
}

func recordingHTTPClientWithStatus(requests *[]capturedRequest, status int, responseBody func(*http.Request) string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if requests != nil {
				_, hasDeadline := req.Context().Deadline()
				*requests = append(*requests, capturedRequest{
					Method:        req.Method,
					Path:          req.URL.Path,
					Host:          req.URL.Host,
					Authorization: req.Header.Get("Authorization"),
					ProjectID:     req.Header.Get("X-Kernel-Project-Id"),
					RetryCount:    req.Header.Get("X-Stainless-Retry-Count"),
					HasDeadline:   hasDeadline,
				})
			}

			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(responseBody(req))),
			}, nil
		}),
	}
}
