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

func TestListProfilePageRejectsRepeatedNextOffset(t *testing.T) {
	t.Parallel()

	clients := New(Config{
		APIKey:    "test-api-key",
		BaseURL:   "https://api.example",
		ProjectID: "default_project",
	}, WithHTTPClient(pagedListHTTPClient(t, nil, "/profiles", "project_123", "Target", []lookupPage{
		{offset: "100", body: profileListPage(profileJSON("profile-b", "Other")), next: "100"},
	})))

	_, err := clients.ListProfilePage(context.Background(), "project_123", "Target", 100)
	if err == nil {
		t.Fatal("expected repeated next offset error")
	}
	for _, want := range []string{"non-advancing", "current offset 100", "next offset 100"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}

func TestListProfilePageRejectsHasMoreWithoutNextOffset(t *testing.T) {
	t.Parallel()

	clients := New(Config{
		APIKey:    "test-api-key",
		BaseURL:   "https://api.example",
		ProjectID: "default_project",
	}, WithHTTPClient(pagedListHTTPClient(t, nil, "/profiles", "project_123", "Target", []lookupPage{
		{body: profileListPage(profileJSON("profile-b", "Other")), hasMore: "true"},
	})))

	_, err := clients.ListProfilePage(context.Background(), "project_123", "Target", 0)
	if err == nil {
		t.Fatal("expected has-more without next offset error")
	}
	for _, want := range []string{"profile pagination", "more results", "without a next offset"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}

func TestListProfilePageStopsOnExplicitHasMoreFalse(t *testing.T) {
	t.Parallel()

	// An explicit X-Has-More: false is the authoritative end-of-results
	// signal; a stale X-Next-Offset alongside it must not keep the scan
	// paging past the end the server declared.
	clients := New(Config{
		APIKey:    "test-api-key",
		BaseURL:   "https://api.example",
		ProjectID: "default_project",
	}, WithHTTPClient(pagedListHTTPClient(t, nil, "/profiles", "project_123", "Target", []lookupPage{
		{body: profileListPage(profileJSON("profile-target", "Target")), next: "100", hasMore: "false"},
	})))

	page, err := clients.ListProfilePage(context.Background(), "project_123", "Target", 0)
	if err != nil {
		t.Fatalf("ListProfilePage returned error: %v", err)
	}
	if page.HasNextPage {
		t.Fatal("HasNextPage = true, want false when X-Has-More is explicitly false")
	}
}

func TestListProfilePageReadsItemsAndNextOffset(t *testing.T) {
	t.Parallel()

	var requests []capturedRequest
	clients := New(Config{
		APIKey:    "test-api-key",
		BaseURL:   "https://api.example",
		ProjectID: "default_project",
	}, WithHTTPClient(pagedListHTTPClient(t, &requests, "/profiles", "project_123", "Target", []lookupPage{
		{body: profileListPage(profileJSON("profile-other", "Other")), next: "100"},
		{offset: "100", body: profileListPage(profileJSON("profile-target", "Target"))},
	})))

	page, err := clients.ListProfilePage(context.Background(), "project_123", "Target", 0)
	if err != nil {
		t.Fatalf("ListProfilePage returned error: %v", err)
	}
	if got, want := len(page.Items), 1; got != want {
		t.Fatalf("items length = %d, want %d", got, want)
	}
	if page.Items[0].ID != "profile-other" {
		t.Fatalf("profile id = %q, want profile-other", page.Items[0].ID)
	}
	if !page.HasNextPage {
		t.Fatal("HasNextPage = false, want true")
	}
	if page.NextOffset != 100 {
		t.Fatalf("NextOffset = %d, want 100", page.NextOffset)
	}

	page, err = clients.ListProfilePage(context.Background(), "project_123", "Target", 100)
	if err != nil {
		t.Fatalf("ListProfilePage returned error: %v", err)
	}
	if page.HasNextPage {
		t.Fatal("HasNextPage = true, want false")
	}
	if page.Items[0].ID != "profile-target" {
		t.Fatalf("profile id = %q, want profile-target", page.Items[0].ID)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
}

func TestListProxyPageReadsItemsAndNextOffset(t *testing.T) {
	t.Parallel()

	var requests []capturedRequest
	clients := New(Config{
		APIKey:    "test-api-key",
		BaseURL:   "https://api.example",
		ProjectID: "default_project",
	}, WithHTTPClient(pagedListHTTPClient(t, &requests, "/proxies", "project_123", "", []lookupPage{
		{body: proxyListPage(proxyJSON("proxy-other", "Other")), next: "100"},
		{offset: "100", body: proxyListPage(proxyJSON("proxy-target", "Target"))},
	})))

	page, err := clients.ListProxyPage(context.Background(), "project_123", 0)
	if err != nil {
		t.Fatalf("ListProxyPage returned error: %v", err)
	}
	if got, want := len(page.Items), 1; got != want {
		t.Fatalf("items length = %d, want %d", got, want)
	}
	if page.Items[0].ID != "proxy-other" {
		t.Fatalf("proxy id = %q, want proxy-other", page.Items[0].ID)
	}
	if !page.HasNextPage {
		t.Fatal("HasNextPage = false, want true")
	}
	if page.NextOffset != 100 {
		t.Fatalf("NextOffset = %d, want 100", page.NextOffset)
	}

	page, err = clients.ListProxyPage(context.Background(), "project_123", 100)
	if err != nil {
		t.Fatalf("ListProxyPage returned error: %v", err)
	}
	if page.HasNextPage {
		t.Fatal("HasNextPage = true, want false")
	}
	if page.Items[0].ID != "proxy-target" {
		t.Fatalf("proxy id = %q, want proxy-target", page.Items[0].ID)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
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

type lookupPage struct {
	offset  string
	next    string
	hasMore string
	body    string
}

func recordingHTTPClient(requests *[]capturedRequest, responseBody func(*http.Request) string) *http.Client {
	return recordingHTTPClientWithStatus(requests, http.StatusOK, responseBody)
}

func recordingHTTPClientWithStatus(requests *[]capturedRequest, status int, responseBody func(*http.Request) string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captureRequest(requests, req)

			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(responseBody(req))),
			}, nil
		}),
	}
}

func recordingHTTPClientWithHeaders(requests *[]capturedRequest, response func(*http.Request) (string, http.Header)) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captureRequest(requests, req)

			body, header := response(req)
			if header == nil {
				header = http.Header{}
			}
			if header.Get("Content-Type") == "" {
				header.Set("Content-Type", "application/json")
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
}

func captureRequest(requests *[]capturedRequest, req *http.Request) {
	if requests == nil {
		return
	}

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

func lookupHeaders(page lookupPage) http.Header {
	if page.next == "" && page.hasMore == "" {
		return nil
	}
	header := http.Header{}
	if page.next != "" {
		header.Set("X-Next-Offset", page.next)
	}
	if page.hasMore != "" {
		header.Set("X-Has-More", page.hasMore)
	}
	return header
}

func pagedListHTTPClient(t *testing.T, requests *[]capturedRequest, path, projectID, wantQuery string, pages []lookupPage) *http.Client {
	t.Helper()

	byOffset := make(map[string]lookupPage, len(pages))
	for _, page := range pages {
		byOffset[page.offset] = page
	}

	return recordingHTTPClientWithHeaders(requests, func(req *http.Request) (string, http.Header) {
		if req.URL.Path != path {
			t.Fatalf("path = %s, want %s", req.URL.Path, path)
		}
		if got := req.URL.Query().Get("query"); got != wantQuery {
			t.Fatalf("query = %q, want %q", got, wantQuery)
		}
		if got := req.Header.Get("X-Kernel-Project-Id"); got != projectID {
			t.Fatalf("project header = %q, want %q", got, projectID)
		}

		offset := req.URL.Query().Get("offset")
		page, ok := byOffset[offset]
		if !ok {
			t.Fatalf("unexpected offset: %q", offset)
		}

		return page.body, lookupHeaders(page)
	})
}

func profileListPage(profiles ...string) string {
	return "[" + strings.Join(profiles, ",") + "]"
}

func profileJSON(id, name string) string {
	return `{"id":"` + id + `","name":"` + name + `","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
}

func proxyListPage(proxies ...string) string {
	return "[" + strings.Join(proxies, ",") + "]"
}

func proxyJSON(id, name string) string {
	return `{"id":"` + id + `","name":"` + name + `","type":"custom","protocol":"https","status":"available","last_checked":"2026-01-01T00:00:00Z"}`
}
