// Package clustering performs offline screenshot deduplication and
// favicon/title clustering. It is intentionally independent from the browser
// scan path so analysing thousands of historical results cannot slow down a
// single target probe.
package clustering

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sensepost/gowitness/internal/islazy"
	"github.com/sensepost/gowitness/pkg/favicon"
	"github.com/sensepost/gowitness/pkg/models"
)

const DefaultHammingThreshold = 10

// Options controls deterministic report analysis.
type Options struct {
	// HammingThreshold is inclusive and uses the existing pHash bit distance.
	HammingThreshold int `json:"hamming_threshold"`

	// IncludeSingletons keeps unique groups in cluster arrays. Model group IDs
	// are still assigned to singleton groups.
	IncludeSingletons bool `json:"include_singletons"`

	// UseExistingScreenshotGroups trusts PerceptionHashGroupId instead of
	// recomputing perceptual groups.
	UseExistingScreenshotGroups bool `json:"use_existing_screenshot_groups"`
}

// DefaultOptions returns the conservative defaults used by the CLI.
func DefaultOptions() Options {
	return Options{HammingThreshold: DefaultHammingThreshold}
}

// Member is a stable, lightweight reference to a result in a cluster.
type Member struct {
	Index          int    `json:"index"`
	ID             uint   `json:"id,omitempty"`
	URL            string `json:"url"`
	FinalURL       string `json:"final_url,omitempty"`
	Title          string `json:"title,omitempty"`
	Favicon        string `json:"favicon,omitempty"`
	FaviconHash    string `json:"favicon_hash,omitempty"`
	Filename       string `json:"file_name,omitempty"`
	PerceptionHash string `json:"perception_hash,omitempty"`
}

// Cluster is a group produced by one clustering strategy.
type Cluster struct {
	ID               uint     `json:"id"`
	ClusterID        uint     `json:"cluster_id"`
	Type             string   `json:"type"`
	Kind             string   `json:"kind"`
	Key              string   `json:"key"`
	Label            string   `json:"label,omitempty"`
	Title            string   `json:"title,omitempty"`
	Favicon          string   `json:"favicon,omitempty"`
	Size             int      `json:"size"`
	Count            int      `json:"count"`
	ResultIDs        []uint   `json:"result_ids"`
	IDs              []uint   `json:"ids"`
	Indexes          []int    `json:"indexes"`
	URLs             []string `json:"urls"`
	Members          []Member `json:"members"`
	RepresentativeID uint     `json:"representative_id,omitempty"`
	CanonicalID      uint     `json:"canonical_id,omitempty"`
	Representative   string   `json:"representative_url,omitempty"`
	CanonicalURL     string   `json:"canonical_url,omitempty"`
}

// Report contains all supported clustering views and merge statistics.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Options     Options   `json:"options"`

	TotalResults     int `json:"total_results"`
	ValidScreenshots int `json:"valid_screenshots"`
	CanonicalResults int `json:"canonical_results"`
	DuplicateResults int `json:"duplicate_results"`
	DuplicatesMerged int `json:"duplicates_merged"`
	DuplicateGroups  int `json:"duplicate_groups"`
	ScreenshotGroups int `json:"screenshot_groups_total"`
	FaviconGroups    int `json:"favicon_groups_total"`
	TitleGroups      int `json:"title_groups_total"`
	CombinedGroups   int `json:"combined_groups_total"`

	ScreenshotClusters []Cluster `json:"screenshot_clusters"`
	FaviconClusters    []Cluster `json:"favicon_clusters"`
	TitleClusters      []Cluster `json:"title_clusters"`
	CombinedClusters   []Cluster `json:"combined_clusters"`

	// Clusters contains the combined favicon+title clusters. It is also a
	// convenient generic view for callers that only need one cluster list.
	Clusters []Cluster `json:"clusters"`

	Canonical []*models.Result `json:"-"`
}

// Analyze enriches and clusters results. The input slice is annotated with
// stable group IDs and duplicate markers, and the returned report retains a
// pointer to the canonical results.
func Analyze(results []*models.Result, optionValues ...Options) *Report {
	options := DefaultOptions()
	if len(optionValues) > 0 {
		options = optionValues[0]
		if options.HammingThreshold < 0 {
			options.HammingThreshold = DefaultHammingThreshold
		}
	}

	report := &Report{
		GeneratedAt:        time.Now().UTC(),
		Options:            options,
		TotalResults:       len(results),
		ScreenshotClusters: []Cluster{},
		FaviconClusters:    []Cluster{},
		TitleClusters:      []Cluster{},
		CombinedClusters:   []Cluster{},
		Clusters:           []Cluster{},
		Canonical:          []*models.Result{},
	}

	for _, result := range results {
		if result == nil {
			continue
		}
		result.SyncClusterAliases()
		EnrichResult(result)

		// Merged JSONL reports intentionally retain merge provenance on
		// canonical rows. Preserve it when this analysis only sees those
		// canonical rows; full reports recompute and replace it below.
		priorDuplicateCount := result.DuplicateCount
		priorDuplicateURLs := append([]string(nil), result.DuplicateURLs...)
		resetMergeState(result)
		if priorDuplicateCount > 0 {
			result.DuplicateCount = priorDuplicateCount
			result.DuplicateURLs = priorDuplicateURLs
		}
	}

	screenshotClusters := assignScreenshotGroups(results, options)
	faviconClusters, faviconGroupCount := assignKeyGroups(results, "favicon", faviconKey, func(r *models.Result, id uint) {
		r.FaviconClusterID = id
		r.FaviconClusterId = id
	})
	titleClusters, titleGroupCount := assignKeyGroups(results, "title", func(r *models.Result) string {
		return r.TitleNormalized
	}, func(r *models.Result, id uint) {
		r.TitleClusterID = id
		r.TitleClusterId = id
	})
	combinedClusters, combinedGroupCount := assignKeyGroups(results, "favicon_title", combinedKey, func(r *models.Result, id uint) {
		r.ClusterID = id
		r.ClusterId = id
	})

	report.ValidScreenshots = countValidScreenshots(results)
	report.ScreenshotGroups = len(screenshotClusters)
	report.FaviconGroups = faviconGroupCount
	report.TitleGroups = titleGroupCount
	report.CombinedGroups = combinedGroupCount

	report.ScreenshotClusters = finalizeClusters(screenshotClusters, results, options.IncludeSingletons)
	report.FaviconClusters = finalizeClusters(faviconClusters, results, options.IncludeSingletons)
	report.TitleClusters = finalizeClusters(titleClusters, results, options.IncludeSingletons)
	report.CombinedClusters = finalizeClusters(combinedClusters, results, options.IncludeSingletons)
	report.Clusters = report.CombinedClusters

	markScreenshotDuplicates(screenshotClusters, results)

	representedMerged := 0
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.IsDuplicate {
			report.DuplicateResults++
		} else {
			report.Canonical = append(report.Canonical, result)
			representedMerged += result.DuplicateCount
		}
	}

	report.CanonicalResults = len(report.Canonical)
	report.DuplicatesMerged = representedMerged
	report.DuplicateGroups = len(report.ScreenshotClusters)

	return report
}

// AnalyzeValues is a convenience wrapper for callers with value slices.
func AnalyzeValues(results []models.Result, optionValues ...Options) *Report {
	pointers := make([]*models.Result, len(results))
	for i := range results {
		pointers[i] = &results[i]
	}
	return Analyze(pointers, optionValues...)
}

// ClusterResults is a concise compatibility wrapper returning combined
// favicon+title clusters.
func ClusterResults(results []*models.Result, optionValues ...Options) []Cluster {
	return Analyze(results, optionValues...).CombinedClusters
}

// Deduplicate analyses results and returns only canonical rows.
func Deduplicate(results []*models.Result, optionValues ...Options) []*models.Result {
	return Analyze(results, optionValues...).Canonical
}

// MergedResults is an alias for Deduplicate.
func MergedResults(results []*models.Result, optionValues ...Options) []*models.Result {
	return Deduplicate(results, optionValues...)
}

// NormalizeTitle canonicalises titles for exact human-meaningful grouping.
func NormalizeTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	title = strings.ToLower(title)
	var normalized strings.Builder
	lastSpace := false
	for _, r := range title {
		if isTitleTokenRune(r) {
			normalized.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace && normalized.Len() > 0 {
			normalized.WriteByte(' ')
			lastSpace = true
		}
	}

	return strings.TrimSpace(normalized.String())
}

// FaviconKey returns the stable favicon identity used for grouping. Content
// hashes are preferred over normalised URLs and raw values.
func FaviconKey(result *models.Result) string {
	return faviconKey(result)
}

// EnrichResult fills cheap, offline-safe favicon and title fields.
func EnrichResult(result *models.Result) {
	if result == nil {
		return
	}

	result.SyncClusterAliases()
	if strings.TrimSpace(result.TitleNormalized) == "" {
		result.TitleNormalized = NormalizeTitle(result.Title)
	}

	if strings.TrimSpace(result.FaviconHash) == "" {
		for _, networkLog := range result.Network {
			if len(networkLog.Content) == 0 {
				continue
			}
			urlLower := strings.ToLower(networkLog.URL)
			mimeLower := strings.ToLower(networkLog.MIMEType)
			if strings.Contains(urlLower, "favicon") || strings.Contains(mimeLower, "icon") {
				result.FaviconHash = favicon.HashBytes(networkLog.Content)
				if strings.TrimSpace(result.Favicon) == "" {
					result.Favicon = networkLog.URL
				}
				if strings.TrimSpace(result.FaviconURL) == "" {
					result.FaviconURL = networkLog.URL
				}
				break
			}
		}
	}

	baseURL := firstNonEmpty(result.FinalURL, result.URL)
	if strings.TrimSpace(result.FaviconURL) == "" {
		if looksLikeURL(result.Favicon) {
			result.FaviconURL = result.Favicon
		} else if result.HTML != "" {
			result.FaviconURL = favicon.ExtractURL(result.HTML, baseURL)
		} else if baseURL != "" {
			result.FaviconURL = favicon.ExtractURL("", baseURL)
		}
	}

	if strings.TrimSpace(result.Favicon) == "" && strings.TrimSpace(result.FaviconURL) != "" {
		result.Favicon = result.FaviconURL
	}
}

type clusterBuilder struct {
	cluster Cluster
	indexes []int
	hash    []byte
}

func assignKeyGroups(
	results []*models.Result,
	kind string,
	keyFunc func(*models.Result) string,
	assign func(*models.Result, uint),
) ([]*clusterBuilder, int) {
	byKey := make(map[string]*clusterBuilder)
	builders := make([]*clusterBuilder, 0)
	nextID := uint(1)

	for index, result := range results {
		if result == nil {
			continue
		}

		key := strings.TrimSpace(keyFunc(result))
		if key == "" {
			assign(result, 0)
			continue
		}

		builder, ok := byKey[key]
		if !ok {
			builder = &clusterBuilder{cluster: Cluster{
				ID:        nextID,
				ClusterID: nextID,
				Type:      kind,
				Kind:      kind,
				Key:       key,
				ResultIDs: []uint{},
				IDs:       []uint{},
				Indexes:   []int{},
				URLs:      []string{},
				Members:   []Member{},
			}}
			byKey[key] = builder
			builders = append(builders, builder)
			nextID++
		}

		builder.indexes = append(builder.indexes, index)
		builder.cluster.Indexes = append(builder.cluster.Indexes, index)
		if result.ID != 0 {
			builder.cluster.ResultIDs = append(builder.cluster.ResultIDs, result.ID)
			builder.cluster.IDs = append(builder.cluster.IDs, result.ID)
		}
		builder.cluster.URLs = append(builder.cluster.URLs, memberURL(result, index))
		builder.cluster.Members = append(builder.cluster.Members, newMember(index, result))
		assign(result, builder.cluster.ID)
	}

	for _, builder := range builders {
		builder.cluster.Size = len(builder.indexes)
		builder.cluster.Count = builder.cluster.Size
		representative := chooseCanonicalIndex(builder.indexes, results)
		if representative >= 0 {
			representativeResult := results[representative]
			builder.cluster.RepresentativeID = representativeResult.ID
			builder.cluster.CanonicalID = representativeResult.ID
			builder.cluster.Representative = memberURL(representativeResult, representative)
			builder.cluster.CanonicalURL = builder.cluster.Representative
			builder.cluster.Label = clusterLabel(kind, builder.cluster.Key, representativeResult)
			builder.cluster.Title = representativeResult.Title
			builder.cluster.Favicon = firstNonEmpty(representativeResult.FaviconHash, representativeResult.Favicon)
		}
	}

	return builders, len(byKey)
}

func assignScreenshotGroups(results []*models.Result, options Options) []*clusterBuilder {
	builders := make([]*clusterBuilder, 0)
	byExistingID := make(map[uint]*clusterBuilder)
	nextID := uint(1)

	for index, result := range results {
		if result == nil {
			continue
		}

		var builder *clusterBuilder

		if options.UseExistingScreenshotGroups && result.PerceptionHashGroupId != 0 {
			if existing, ok := byExistingID[result.PerceptionHashGroupId]; ok {
				builder = existing
			} else {
				builder = newScreenshotBuilder(nextID, result.PerceptionHash, result, index)
				byExistingID[result.PerceptionHashGroupId] = builder
				builders = append(builders, builder)
				nextID++
			}
		} else if parsed, err := islazy.ParsePerceptionHash(result.PerceptionHash); err == nil {
			for _, candidate := range builders {
				if len(candidate.hash) == 0 {
					continue
				}
				if distance, distanceErr := islazy.HammingDistance(parsed, candidate.hash); distanceErr == nil && distance <= options.HammingThreshold {
					builder = candidate
					break
				}
			}

			if builder == nil {
				builder = newScreenshotBuilder(nextID, result.PerceptionHash, result, index)
				builder.hash = parsed
				builders = append(builders, builder)
				nextID++
			}
		} else if strings.TrimSpace(result.ScreenshotHash) != "" {
			key := "sha256:" + strings.ToLower(strings.TrimSpace(result.ScreenshotHash))
			for _, candidate := range builders {
				if candidate.cluster.Key == key {
					builder = candidate
					break
				}
			}
			if builder == nil {
				builder = newScreenshotBuilder(nextID, key, result, index)
				builders = append(builders, builder)
				nextID++
			}
		} else {
			result.PerceptionHashGroupId = 0
			result.ScreenshotClusterID = 0
			result.ScreenshotClusterId = 0
			continue
		}

		addMemberToBuilder(builder, index, result)
		result.PerceptionHashGroupId = builder.cluster.ID
		result.ScreenshotClusterID = builder.cluster.ID
		result.ScreenshotClusterId = builder.cluster.ID
	}

	for _, builder := range builders {
		builder.cluster.Size = len(builder.indexes)
		builder.cluster.Count = builder.cluster.Size
		canonicalIndex := chooseCanonicalIndex(builder.indexes, results)
		if canonicalIndex >= 0 {
			canonical := results[canonicalIndex]
			builder.cluster.RepresentativeID = canonical.ID
			builder.cluster.CanonicalID = canonical.ID
			builder.cluster.Representative = memberURL(canonical, canonicalIndex)
			builder.cluster.CanonicalURL = builder.cluster.Representative
			builder.cluster.Label = firstNonEmpty(canonical.Title, canonical.URL, canonical.Filename)
			builder.cluster.Title = canonical.Title
		}
	}

	return builders
}

func newScreenshotBuilder(id uint, key string, result *models.Result, index int) *clusterBuilder {
	builder := &clusterBuilder{cluster: Cluster{
		ID:        id,
		ClusterID: id,
		Type:      "screenshot",
		Kind:      "screenshot",
		Key:       key,
		ResultIDs: []uint{},
		IDs:       []uint{},
		Indexes:   []int{},
		URLs:      []string{},
		Members:   []Member{},
	}}
	if parsed, err := islazy.ParsePerceptionHash(key); err == nil {
		builder.hash = parsed
	}
	return builder
}

func addMemberToBuilder(builder *clusterBuilder, index int, result *models.Result) {
	builder.indexes = append(builder.indexes, index)
	builder.cluster.Indexes = append(builder.cluster.Indexes, index)
	if result.ID != 0 {
		builder.cluster.ResultIDs = append(builder.cluster.ResultIDs, result.ID)
		builder.cluster.IDs = append(builder.cluster.IDs, result.ID)
	}
	builder.cluster.URLs = append(builder.cluster.URLs, memberURL(result, index))
	builder.cluster.Members = append(builder.cluster.Members, newMember(index, result))
}

func finalizeClusters(builders []*clusterBuilder, results []*models.Result, includeSingletons bool) []Cluster {
	clusters := make([]Cluster, 0, len(builders))
	for _, builder := range builders {
		if !includeSingletons && len(builder.indexes) < 2 {
			continue
		}
		clusters = append(clusters, builder.cluster)
	}

	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].Size != clusters[j].Size {
			return clusters[i].Size > clusters[j].Size
		}
		return clusters[i].ID < clusters[j].ID
	})

	return clusters
}

func markScreenshotDuplicates(builders []*clusterBuilder, results []*models.Result) {
	for _, builder := range builders {
		if len(builder.indexes) < 2 {
			continue
		}

		canonicalIndex := chooseCanonicalIndex(builder.indexes, results)
		if canonicalIndex < 0 {
			continue
		}
		canonical := results[canonicalIndex]
		duplicateURLs := make([]string, 0, len(builder.indexes)-1)

		for _, index := range builder.indexes {
			result := results[index]
			if result == nil || index == canonicalIndex {
				continue
			}

			result.IsDuplicate = true
			result.DuplicateCount = 0
			result.DuplicateURLs = nil
			result.DuplicateOfID = canonical.ID
			result.DuplicateOfId = canonical.ID
			result.CanonicalURL = memberURL(canonical, canonicalIndex)
			duplicateURLs = append(duplicateURLs, result.URL)
			if result.URL == "" {
				duplicateURLs[len(duplicateURLs)-1] = memberURL(result, index)
			}
		}

		canonical.DuplicateCount = len(builder.indexes) - 1
		canonical.DuplicateURLs = duplicateURLs
	}
}

func chooseCanonicalIndex(indexes []int, results []*models.Result) int {
	best := -1
	bestScore := 0

	for _, index := range indexes {
		result := results[index]
		if result == nil {
			continue
		}

		score := 0
		if !result.Failed {
			score += 100
		}
		if result.PerceptionHash != "" {
			score += 20
		}
		if result.Screenshot != "" {
			score += 10
		}
		if result.Filename != "" {
			score += 5
		}
		if result.Title != "" {
			score++
		}
		// On equal quality, retain the earliest input/DB row.
		if best == -1 || score > bestScore || (score == bestScore && index < best) {
			best = index
			bestScore = score
		}
	}

	return best
}

func resetMergeState(result *models.Result) {
	result.IsDuplicate = false
	result.DuplicateCount = 0
	result.DuplicateURLs = nil
	result.DuplicateOfID = 0
	result.DuplicateOfId = 0
	result.CanonicalURL = ""
}

func newMember(index int, result *models.Result) Member {
	return Member{
		Index:          index,
		ID:             result.ID,
		URL:            result.URL,
		FinalURL:       result.FinalURL,
		Title:          result.Title,
		Favicon:        result.Favicon,
		FaviconHash:    result.FaviconHash,
		Filename:       result.Filename,
		PerceptionHash: result.PerceptionHash,
	}
}

func memberURL(result *models.Result, index int) string {
	if result == nil {
		return ""
	}
	if result.URL != "" {
		return result.URL
	}
	if result.FinalURL != "" {
		return result.FinalURL
	}
	if result.ID != 0 {
		return "id:" + strconv.FormatUint(uint64(result.ID), 10)
	}
	return "index:" + strconv.Itoa(index)
}

func faviconKey(result *models.Result) string {
	if result.FaviconHash != "" {
		return "content:" + strings.ToLower(strings.TrimSpace(result.FaviconHash))
	}

	candidate := firstNonEmpty(result.Favicon, result.FaviconURL)
	if candidate == "" {
		return ""
	}

	if looksLikeURL(candidate) {
		return "url:" + favicon.NormalizeURL(candidate)
	}

	return "value:" + strings.ToLower(strings.TrimSpace(candidate))
}

func combinedKey(result *models.Result) string {
	faviconPart := faviconKey(result)
	titlePart := strings.TrimSpace(result.TitleNormalized)
	if faviconPart == "" || titlePart == "" {
		return ""
	}
	return faviconPart + "|title:" + titlePart
}

func clusterLabel(kind, key string, result *models.Result) string {
	switch kind {
	case "title":
		return result.Title
	case "favicon":
		return firstNonEmpty(result.Favicon, result.FaviconURL, result.FaviconHash)
	default:
		return firstNonEmpty(result.Title, key)
	}
}

func countValidScreenshots(results []*models.Result) int {
	count := 0
	for _, result := range results {
		if result == nil {
			continue
		}
		if _, err := islazy.ParsePerceptionHash(result.PerceptionHash); err == nil {
			count++
			continue
		}
		if result.ScreenshotHash != "" || result.Screenshot != "" || result.Filename != "" {
			count++
		}
	}
	return count
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func looksLikeURL(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "//") ||
		strings.HasPrefix(value, "data:")
}

func isTitleTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}
