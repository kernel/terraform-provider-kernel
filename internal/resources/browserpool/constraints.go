package browserpool

const (
	maxBrowserPoolNameLength int   = 255
	maxBrowserPoolExtensions int   = 20
	minBrowserPoolSize       int64 = 1
	minTimeoutSeconds        int64 = 10
	maxTimeoutSeconds        int64 = 259200
	minFillRatePerMinute     int64 = 0
	minViewportDimension     int64 = 1
	minViewportRefreshRate   int64 = 1
)
