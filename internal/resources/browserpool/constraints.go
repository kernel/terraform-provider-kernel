package browserpool

const (
	maxBrowserPoolNameLength int   = 255
	maxBrowserPoolExtensions int   = 20
	maxStartURLBytes         int   = 2048
	maxChromePolicyBytes     int   = 5 * 1024 * 1024
	minBrowserPoolSize       int64 = 1
	minTimeoutSeconds        int64 = 10
	maxTimeoutSeconds        int64 = 259200
	minFillRatePerMinute     int64 = 0
	maxFillRatePerMinute     int64 = 50
	minViewportDimension     int64 = 1
	minViewportRefreshRate   int64 = 1
)
