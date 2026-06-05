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

type ProjectPage struct {
	Items       []kernel.Project
	NextOffset  int64
	HasNextPage bool
}

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
		extensions:       kernel.NewExtensionService(requestOpts...),
		browserPools:     kernel.NewBrowserPoolService(requestOpts...),
	}
}

func (c Clients) DefaultProjectID() string {
	return c.defaultProjectID
}

// Projects are org-scoped, so their methods take no project.

func (c Clients) GetProject(ctx context.Context, id string) (*kernel.Project, error) {
	return c.projects.Get(ctx, id)
}

func (c Clients) ListProjectPage(ctx context.Context, query string, offset int64) (ProjectPage, error) {
	var raw *http.Response
	params := kernel.ProjectListParams{
		Query: kernel.String(query),
		Limit: kernel.Int(nameLookupLimit),
	}
	if offset > 0 {
		params.Offset = kernel.Int(offset)
	}

	page, err := c.projects.List(ctx, params, option.WithResponseInto(&raw))
	if err != nil {
		return ProjectPage{}, err
	}
	if page == nil {
		return ProjectPage{}, fmt.Errorf("Kernel returned an empty project list response")
	}

	next, ok, err := projectLookupNextOffset(raw, offset)
	if err != nil {
		return ProjectPage{}, err
	}
	return ProjectPage{
		Items:       page.Items,
		NextOffset:  next,
		HasNextPage: ok,
	}, nil
}

// The remaining methods are project-scoped and take the resolved project
// for each call.

func (c Clients) GetProxy(ctx context.Context, projectID, id string) (*kernel.ProxyGetResponse, error) {
	return c.proxies.Get(ctx, id, scope(projectID)...)
}

func (c Clients) GetProfile(ctx context.Context, projectID, idOrName string) (*kernel.Profile, error) {
	return c.profiles.Get(ctx, idOrName, scope(projectID)...)
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

func projectLookupNextOffset(raw *http.Response, current int64) (int64, bool, error) {
	if raw == nil {
		return 0, false, fmt.Errorf("Kernel returned an empty pagination response")
	}

	value := raw.Header.Get("X-Next-Offset")
	if value == "" {
		return 0, false, nil
	}

	next, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid Kernel pagination next offset %q: %w", value, err)
	}
	if next <= 0 {
		return 0, false, nil
	}
	if next <= current {
		return 0, false, fmt.Errorf(
			"non-advancing Kernel project pagination: current offset %d, next offset %d",
			current,
			next,
		)
	}

	return next, true, nil
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
