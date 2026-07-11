package kernelclient

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
)

const DefaultRequestTimeout = 2 * time.Minute

const nameLookupLimit int64 = 100

type Page[T any] struct {
	Items       []T
	NextOffset  int64
	HasNextPage bool
}

type ProfilePage = Page[kernel.Profile]
type ProxyPage = Page[kernel.ProxyListResponse]
type AppPage = Page[kernel.AppListResponse]

// Config configures the shared Kernel API clients. ProjectID is a default
// only; the client never applies it implicitly.
type Config struct {
	APIKey         string
	BaseURL        string
	ProjectID      string
	RequestTimeout time.Duration
}

type Clients struct {
	defaultProjectID string
	projects         kernel.ProjectService
	profiles         kernel.ProfileService
	proxies          kernel.ProxyService
	apps             kernel.AppService
	extensions       kernel.ExtensionService
	browserPools     kernel.BrowserPoolService
}

type Option func(*clientOptions)

type clientOptions struct {
	httpClient option.HTTPClient
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(opts *clientOptions) {
		opts.httpClient = httpClient
	}
}

func New(config Config, opts ...Option) Clients {
	clientOpts := clientOptions{}
	for _, opt := range opts {
		opt(&clientOpts)
	}

	requestOpts := requestOptions(config, clientOpts)

	return Clients{
		defaultProjectID: config.ProjectID,
		projects:         kernel.NewProjectService(requestOpts...),
		profiles:         kernel.NewProfileService(requestOpts...),
		proxies:          kernel.NewProxyService(requestOpts...),
		apps:             kernel.NewAppService(requestOpts...),
		extensions:       kernel.NewExtensionService(requestOpts...),
		browserPools:     kernel.NewBrowserPoolService(requestOpts...),
	}
}

func (c Clients) DefaultProjectID() string {
	return c.defaultProjectID
}

// Projects are org-scoped, so their methods take no project.

func (c Clients) CreateProject(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
	return c.projects.New(ctx, params, noMutationRetries())
}

// GetProject resolves a project by ID or by name; the API treats the path
// parameter as id-or-name (names are unique within an organization).
func (c Clients) GetProject(ctx context.Context, idOrName string) (*kernel.Project, error) {
	return c.projects.Get(ctx, idOrName)
}

func (c Clients) UpdateProject(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
	return c.projects.Update(ctx, id, params, noMutationRetries())
}

func (c Clients) DeleteProject(ctx context.Context, id string) error {
	return c.projects.Delete(ctx, id, noMutationRetries())
}

// The remaining methods are project-scoped and take the resolved project
// for each call.

func (c Clients) GetProxy(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
	return c.proxies.Get(ctx, id, scope(projectID)...)
}

func (c Clients) CreateProxy(ctx context.Context, projectID string, params kernel.ProxyNewParams) (*kernel.ProxyNewResponse, error) {
	return c.proxies.New(ctx, params, scope(projectID, noMutationRetries())...)
}

func (c Clients) DeleteProxy(ctx context.Context, projectID, id string) error {
	return c.proxies.Delete(ctx, id, scope(projectID, noMutationRetries())...)
}

func (c Clients) ListProxyPage(ctx context.Context, projectID string, offset int64) (ProxyPage, error) {
	var raw *http.Response
	params := kernel.ProxyListParams{
		Limit: kernel.Int(nameLookupLimit),
	}
	if offset > 0 {
		params.Offset = kernel.Int(offset)
	}

	page, err := c.proxies.List(ctx, params, scope(projectID, option.WithResponseInto(&raw))...)
	if err != nil {
		return ProxyPage{}, err
	}
	if page == nil {
		return ProxyPage{}, fmt.Errorf("Kernel returned an empty proxy list response")
	}

	next, ok, err := lookupNextOffset(raw, offset, "proxy")
	if err != nil {
		return ProxyPage{}, err
	}
	return ProxyPage{
		Items:       page.Items,
		NextOffset:  next,
		HasNextPage: ok,
	}, nil
}

func (c Clients) ListAppPage(ctx context.Context, projectID, appName, version string, offset int64) (AppPage, error) {
	var raw *http.Response
	params := kernel.AppListParams{
		AppName: kernel.String(appName),
		Version: kernel.String(version),
		Limit:   kernel.Int(nameLookupLimit),
	}
	if offset > 0 {
		params.Offset = kernel.Int(offset)
	}

	page, err := c.apps.List(ctx, params, scope(projectID, option.WithResponseInto(&raw))...)
	if err != nil {
		return AppPage{}, err
	}
	if page == nil {
		return AppPage{}, fmt.Errorf("Kernel returned an empty app list response")
	}

	next, ok, err := lookupNextOffset(raw, offset, "app")
	if err != nil {
		return AppPage{}, err
	}
	return AppPage{
		Items:       page.Items,
		NextOffset:  next,
		HasNextPage: ok,
	}, nil
}

func (c Clients) GetProfile(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
	return c.profiles.Get(ctx, idOrName, scope(projectID)...)
}

func (c Clients) CreateProfile(ctx context.Context, projectID string, params kernel.ProfileNewParams) (*kernel.Profile, error) {
	return c.profiles.New(ctx, params, scope(projectID, noMutationRetries())...)
}

func (c Clients) DeleteProfile(ctx context.Context, projectID, idOrName string) error {
	return c.profiles.Delete(ctx, idOrName, scope(projectID, noMutationRetries())...)
}

func (c Clients) ListProfilePage(ctx context.Context, projectID, query string, offset int64) (ProfilePage, error) {
	var raw *http.Response
	params := kernel.ProfileListParams{
		Query: kernel.String(query),
		Limit: kernel.Int(nameLookupLimit),
	}
	if offset > 0 {
		params.Offset = kernel.Int(offset)
	}

	page, err := c.profiles.List(ctx, params, scope(projectID, option.WithResponseInto(&raw))...)
	if err != nil {
		return ProfilePage{}, err
	}
	if page == nil {
		return ProfilePage{}, fmt.Errorf("Kernel returned an empty profile list response")
	}

	next, ok, err := lookupNextOffset(raw, offset, "profile")
	if err != nil {
		return ProfilePage{}, err
	}
	return ProfilePage{
		Items:       page.Items,
		NextOffset:  next,
		HasNextPage: ok,
	}, nil
}

// GetExtension resolves an extension by ID or by name within the project; the
// API treats the path parameter as id-or-name and returns metadata only.
func (c Clients) GetExtension(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
	return c.extensions.Get(ctx, idOrName, scope(projectID)...)
}

func (c Clients) UploadExtension(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
	return c.extensions.Upload(ctx, params, scope(projectID, noMutationRetries())...)
}

func (c Clients) DeleteExtension(ctx context.Context, projectID, id string) error {
	return c.extensions.Delete(ctx, id, scope(projectID, noMutationRetries())...)
}

func (c Clients) CreateBrowserPool(ctx context.Context, projectID string, params kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error) {
	return c.browserPools.New(ctx, params, scope(projectID, noMutationRetries())...)
}

func (c Clients) GetBrowserPool(ctx context.Context, projectID, id string) (*kernel.BrowserPool, error) {
	return c.browserPools.Get(ctx, id, scope(projectID)...)
}

func (c Clients) UpdateBrowserPool(ctx context.Context, projectID, id string, params kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error) {
	return c.browserPools.Update(ctx, id, params, scope(projectID, noMutationRetries())...)
}

func (c Clients) DeleteBrowserPool(ctx context.Context, projectID, id string) error {
	return c.browserPools.Delete(ctx, id, kernel.BrowserPoolDeleteParams{Force: kernel.Bool(false)}, scope(projectID, noMutationRetries())...)
}

// scope builds the request options for a project-scoped call. An empty
// projectID sends no project scope so the API key's binding (or the org
// default) decides the project.
func scope(projectID string, extra ...option.RequestOption) []option.RequestOption {
	opts := make([]option.RequestOption, 0, len(extra)+1)
	if projectID != "" {
		opts = append(opts, option.WithProjectID(projectID))
	}
	return append(opts, extra...)
}

func noMutationRetries() option.RequestOption {
	return option.WithMaxRetries(0)
}

func lookupNextOffset(raw *http.Response, current int64, kind string) (int64, bool, error) {
	if raw == nil {
		return 0, false, fmt.Errorf("Kernel returned an empty pagination response")
	}

	hasMore, hasMoreSet, err := lookupHasMore(raw, kind)
	if err != nil {
		return 0, false, err
	}
	// An explicit X-Has-More is the authoritative continuation signal: a
	// stale X-Next-Offset alongside has-more false must not keep a scan
	// paging past the end the server declared.
	if hasMoreSet && !hasMore {
		return 0, false, nil
	}

	value := raw.Header.Get("X-Next-Offset")
	if value == "" {
		if hasMore {
			return 0, false, fmt.Errorf("Kernel %s pagination reported more results without a next offset", kind)
		}
		return 0, false, nil
	}

	next, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid Kernel pagination next offset %q: %w", value, err)
	}
	if next <= 0 {
		if hasMore {
			return 0, false, fmt.Errorf("Kernel %s pagination reported more results with non-positive next offset %d", kind, next)
		}
		return 0, false, nil
	}
	if next <= current {
		return 0, false, fmt.Errorf(
			"non-advancing Kernel %s pagination: current offset %d, next offset %d",
			kind,
			current,
			next,
		)
	}

	return next, true, nil
}

// lookupHasMore reports the X-Has-More value and whether the header was
// present at all; callers treat an explicit value as authoritative and fall
// back to offset semantics when the header is absent.
func lookupHasMore(raw *http.Response, kind string) (bool, bool, error) {
	value := raw.Header.Get("X-Has-More")
	if value == "" {
		return false, false, nil
	}

	hasMore, err := strconv.ParseBool(value)
	if err != nil {
		return false, false, fmt.Errorf("invalid Kernel %s pagination has-more %q: %w", kind, value, err)
	}
	return hasMore, true, nil
}

func requestOptions(config Config, clientOpts clientOptions) []option.RequestOption {
	requestTimeout := config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = DefaultRequestTimeout
	}

	opts := []option.RequestOption{
		option.WithEnvironmentProduction(),
		option.WithAPIKey(config.APIKey),
		option.WithRequestTimeout(requestTimeout),
	}

	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}
	if clientOpts.httpClient != nil {
		opts = append(opts, option.WithHTTPClient(clientOpts.httpClient))
	}

	return opts
}
