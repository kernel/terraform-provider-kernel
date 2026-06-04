package kernelclient

import (
	"context"
	"net/http"
	"time"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
)

const projectIDHeader = "X-Kernel-Project-Id"

const DefaultRequestTimeout = 2 * time.Minute

type Config struct {
	APIKey         string
	BaseURL        string
	ProjectID      string
	RequestTimeout time.Duration
}

type Clients struct {
	org     durableClient
	project durableClient
}

type durableClient struct {
	projects     kernel.ProjectService
	profiles     kernel.ProfileService
	proxies      kernel.ProxyService
	extensions   kernel.ExtensionService
	browserPools kernel.BrowserPoolService
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

	orgOptions := requestOptions(config, clientOpts)
	projectOptions := append([]option.RequestOption{}, orgOptions...)
	if config.ProjectID != "" {
		projectOptions = append(projectOptions, option.WithHeader(projectIDHeader, config.ProjectID))
	}

	return Clients{
		org:     newDurableClient(orgOptions),
		project: newDurableClient(projectOptions),
	}
}

func (c Clients) GetProject(ctx context.Context, id string) (*kernel.Project, error) {
	return c.org.projects.Get(ctx, id)
}

func (c Clients) ListProjects(ctx context.Context, params kernel.ProjectListParams) (*pagination.OffsetPagination[kernel.Project], error) {
	return c.org.projects.List(ctx, params)
}

func (c Clients) GetProfile(ctx context.Context, idOrName string) (*kernel.Profile, error) {
	return c.project.profiles.Get(ctx, idOrName)
}

func (c Clients) ListProfiles(ctx context.Context, params kernel.ProfileListParams) (*pagination.OffsetPagination[kernel.Profile], error) {
	return c.project.profiles.List(ctx, params)
}

func (c Clients) GetProxy(ctx context.Context, id string) (*kernel.ProxyGetResponse, error) {
	return c.project.proxies.Get(ctx, id)
}

func (c Clients) ListProxies(ctx context.Context, params kernel.ProxyListParams) (*pagination.OffsetPagination[kernel.ProxyListResponse], error) {
	return c.project.proxies.List(ctx, params)
}

func (c Clients) ListExtensions(ctx context.Context, params kernel.ExtensionListParams) (*pagination.OffsetPagination[kernel.ExtensionListResponse], error) {
	return c.project.extensions.List(ctx, params)
}

func (c Clients) CreateBrowserPool(ctx context.Context, params kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error) {
	return c.project.browserPools.New(ctx, params, noMutationRetries())
}

func (c Clients) GetBrowserPool(ctx context.Context, id string) (*kernel.BrowserPool, error) {
	return c.project.browserPools.Get(ctx, id)
}

func (c Clients) ListBrowserPools(ctx context.Context, params kernel.BrowserPoolListParams) (*pagination.OffsetPagination[kernel.BrowserPool], error) {
	return c.project.browserPools.List(ctx, params)
}

func (c Clients) UpdateBrowserPool(ctx context.Context, id string, params kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error) {
	return c.project.browserPools.Update(ctx, id, params, noMutationRetries())
}

func (c Clients) DeleteBrowserPool(ctx context.Context, id string) error {
	return c.project.browserPools.Delete(ctx, id, kernel.BrowserPoolDeleteParams{Force: kernel.Bool(false)}, noMutationRetries())
}

func noMutationRetries() option.RequestOption {
	return option.WithMaxRetries(0)
}

func newDurableClient(opts []option.RequestOption) durableClient {
	return durableClient{
		projects:     kernel.NewProjectService(opts...),
		profiles:     kernel.NewProfileService(opts...),
		proxies:      kernel.NewProxyService(opts...),
		extensions:   kernel.NewExtensionService(opts...),
		browserPools: kernel.NewBrowserPoolService(opts...),
	}
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
