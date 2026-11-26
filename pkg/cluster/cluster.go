// Package cluster provides a short import alias for pkg/clustering.
package cluster

import (
	"github.com/sensepost/gowitness/pkg/clustering"
	"github.com/sensepost/gowitness/pkg/models"
)

const DefaultHammingThreshold = clustering.DefaultHammingThreshold

type (
	Options = clustering.Options
	Report  = clustering.Report
	Cluster = clustering.Cluster
	Member  = clustering.Member
)

// DefaultOptions returns clustering defaults.
func DefaultOptions() Options { return clustering.DefaultOptions() }

// Analyze clusters and annotates pointer results.
func Analyze(results []*Result, optionValues ...Options) *Report {
	return clustering.Analyze(results, optionValues...)
}

// AnalyzeValues clusters value results.
func AnalyzeValues(results []Result, optionValues ...Options) *Report {
	return clustering.AnalyzeValues(results, optionValues...)
}

// ClusterResults returns combined favicon+title clusters.
func ClusterResults(results []*Result, optionValues ...Options) []Cluster {
	return clustering.ClusterResults(results, optionValues...)
}

// ClusterByScreenshot returns perceptual screenshot groups.
func ClusterByScreenshot(results []*Result, optionValues ...Options) []Cluster {
	return clustering.ClusterByScreenshot(results, optionValues...)
}

// ClusterByFavicon returns favicon groups.
func ClusterByFavicon(results []*Result, optionValues ...Options) []Cluster {
	return clustering.ClusterByFavicon(results, optionValues...)
}

// ClusterByTitle returns normalized title groups.
func ClusterByTitle(results []*Result, optionValues ...Options) []Cluster {
	return clustering.ClusterByTitle(results, optionValues...)
}

// ClusterByFaviconAndTitle returns combined groups.
func ClusterByFaviconAndTitle(results []*Result, optionValues ...Options) []Cluster {
	return clustering.ClusterByFaviconAndTitle(results, optionValues...)
}

// Deduplicate returns canonical results.
func Deduplicate(results []*Result, optionValues ...Options) []*Result {
	return clustering.Deduplicate(results, optionValues...)
}

// MergedResults is an alias for Deduplicate.
func MergedResults(results []*Result, optionValues ...Options) []*Result {
	return clustering.MergedResults(results, optionValues...)
}

// DeduplicateScreenshots is an alias for Deduplicate.
func DeduplicateScreenshots(results []*Result, optionValues ...Options) []*Result {
	return clustering.DeduplicateScreenshots(results, optionValues...)
}

// NormalizeTitle exposes title normalisation.
func NormalizeTitle(title string) string { return clustering.NormalizeTitle(title) }

// FaviconKey exposes stable favicon grouping key generation.
func FaviconKey(result *Result) string { return clustering.FaviconKey(result) }

// EnrichResult fills offline favicon/title fields.
func EnrichResult(result *Result) { clustering.EnrichResult(result) }

// Result is re-exported so callers can use this package's short name.
type Result = models.Result

// Clusterer is a small stateful wrapper for callers that prefer constructors.
type Clusterer struct {
	Options Options
}

// NewClusterer creates a Clusterer. A single optional threshold is accepted.
func NewClusterer(threshold ...int) *Clusterer {
	options := DefaultOptions()
	if len(threshold) > 0 && threshold[0] > 0 {
		options.HammingThreshold = threshold[0]
	}
	return &Clusterer{Options: options}
}

// Cluster analyses results with the clusterer's options.
func (c *Clusterer) Cluster(results []*Result) *Report {
	return Analyze(results, c.Options)
}

// Deduplicate analyses results and returns canonical rows.
func (c *Clusterer) Deduplicate(results []*Result) []*Result {
	return Deduplicate(results, c.Options)
}
