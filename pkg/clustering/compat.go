package clustering

import "github.com/sensepost/gowitness/pkg/models"

// ClusterByScreenshot returns perceptual screenshot groups.
func ClusterByScreenshot(results []*models.Result, options ...Options) []Cluster {
	return Analyze(results, options...).ScreenshotClusters
}

// ClusterByFavicon returns favicon groups.
func ClusterByFavicon(results []*models.Result, options ...Options) []Cluster {
	return Analyze(results, options...).FaviconClusters
}

// ClusterByTitle returns normalized-title groups.
func ClusterByTitle(results []*models.Result, options ...Options) []Cluster {
	return Analyze(results, options...).TitleClusters
}

// ClusterByFaviconAndTitle returns groups requiring both signals to match.
func ClusterByFaviconAndTitle(results []*models.Result, options ...Options) []Cluster {
	return Analyze(results, options...).CombinedClusters
}

// GroupByScreenshot is an alias for ClusterByScreenshot.
func GroupByScreenshot(results []*models.Result, options ...Options) []Cluster {
	return ClusterByScreenshot(results, options...)
}

// GroupByFavicon is an alias for ClusterByFavicon.
func GroupByFavicon(results []*models.Result, options ...Options) []Cluster {
	return ClusterByFavicon(results, options...)
}

// GroupByTitle is an alias for ClusterByTitle.
func GroupByTitle(results []*models.Result, options ...Options) []Cluster {
	return ClusterByTitle(results, options...)
}

// DeduplicateScreenshots is an alias for Deduplicate.
func DeduplicateScreenshots(results []*models.Result, options ...Options) []*models.Result {
	return Deduplicate(results, options...)
}

// MergeDuplicateScreenshots is an alias for Deduplicate.
func MergeDuplicateScreenshots(results []*models.Result, options ...Options) []*models.Result {
	return Deduplicate(results, options...)
}
