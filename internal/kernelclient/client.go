package kernelclient

import (
	"context"
	"net/http"
	"time"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
)

const DefaultRequestTimeout = 2 * time.Minute

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
