package clustering

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sensepost/gowitness/pkg/models"
)

func TestNormalizeTitle(t *testing.T) {
	tests := map[string]string{
		"  Admin   Login  ":    "admin login",
		"Example.COM — Login!": "example com login",
		"路由器 管理界面":             "路由器 管理界面",
		"":                     "",
	}

	for input, expected := range tests {
		if actual := NormalizeTitle(input); actual != expected {
			t.Fatalf("NormalizeTitle(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestFaviconAndTitleClustersMatchManualGroups(t *testing.T) {
	results := []*models.Result{
		{ID: 1, URL: "https://a.example/", Title: "Admin Login", FaviconHash: "icon-a"},
		{ID: 2, URL: "https://b.example/", Title: "  admin\tlogin ", FaviconHash: "icon-a"},
		{ID: 3, URL: "https://c.example/", Title: "Dashboard", FaviconHash: "icon-a"},
		{ID: 4, URL: "https://d.example/", Title: "Admin Login", FaviconHash: "icon-b"},
	}

	report := Analyze(results)

	if len(report.FaviconClusters) != 1 {
		t.Fatalf("expected one multi-member favicon cluster, got %+v", report.FaviconClusters)
	}
	if report.FaviconClusters[0].Size != 3 {
		t.Fatalf("expected favicon cluster size 3, got %d", report.FaviconClusters[0].Size)
	}
	if len(report.TitleClusters) != 1 || report.TitleClusters[0].Size != 3 {
		t.Fatalf("expected one title cluster with three members, got %+v", report.TitleClusters)
	}
	if len(report.CombinedClusters) != 1 || report.CombinedClusters[0].Size != 2 {
		t.Fatalf("expected one combined favicon+title cluster with two members, got %+v", report.CombinedClusters)
	}
	if results[0].FaviconClusterID == 0 || results[0].TitleClusterID == 0 || results[0].ClusterID == 0 {
		t.Fatalf("cluster IDs were not assigned: %+v", results[0])
	}
}

func TestScreenshotDuplicatesAreMerged(t *testing.T) {
	results := []*models.Result{
		{ID: 1, URL: "https://a.example/", PerceptionHash: "p:0000000000000000"},
		{ID: 2, URL: "https://b.example/", PerceptionHash: "p:0000000000000000"},
		{ID: 3, URL: "https://c.example/", PerceptionHash: "p:ffffffffffffffff"},
	}

	report := Analyze(results, Options{HammingThreshold: 0})

	if report.TotalResults != 3 {
		t.Fatalf("unexpected total: %d", report.TotalResults)
	}
	if report.CanonicalResults != 2 {
		t.Fatalf("expected two canonical results, got %d", report.CanonicalResults)
	}
	if report.DuplicatesMerged != 1 {
		t.Fatalf("expected one merged duplicate, got %d", report.DuplicatesMerged)
	}
	if !results[1].IsDuplicate || results[1].DuplicateOfID != 1 {
		t.Fatalf("second result was not marked as duplicate of first: %+v", results[1])
	}
	if results[0].DuplicateCount != 1 || len(results[0].DuplicateURLs) != 1 {
		t.Fatalf("canonical merge metadata incorrect: %+v", results[0])
	}
	if len(report.ScreenshotClusters) != 1 || report.ScreenshotClusters[0].Size != 2 {
		t.Fatalf("unexpected screenshot clusters: %+v", report.ScreenshotClusters)
	}

	remerged := Analyze(report.Canonical, Options{HammingThreshold: 0})
	if remerged.DuplicatesMerged != 1 {
		t.Fatalf("merged provenance was not preserved: %+v", remerged)
	}
	if report.Canonical[0].DuplicateCount != 1 || len(report.Canonical[0].DuplicateURLs) != 1 {
		t.Fatalf("canonical merge provenance changed after reanalysis: %+v", report.Canonical[0])
	}
}

func TestNearPerceptualHashesUseThreshold(t *testing.T) {
	results := []*models.Result{
		{ID: 1, URL: "https://a.example/", PerceptionHash: "p:0000000000000000"},
		{ID: 2, URL: "https://b.example/", PerceptionHash: "p:8000000000000000"},
		{ID: 3, URL: "https://c.example/", PerceptionHash: "p:ffffffffffffffff"},
	}

	report := Analyze(results)

	if len(report.ScreenshotClusters) != 1 || report.ScreenshotClusters[0].Size != 2 {
		t.Fatalf("expected near-hash pair only, got %+v", report.ScreenshotClusters)
	}
	if report.CanonicalResults != 2 {
		t.Fatalf("expected near duplicates merged into two canonical rows, got %d", report.CanonicalResults)
	}
}

func TestThousandTargetsMergeRepeatedScreenshots(t *testing.T) {
	const groups = 250
	const repeats = 4
	results := make([]*models.Result, 0, groups*repeats)

	for group := range groups {
		hash := fmt.Sprintf("p:%016x", uint64(group+1)<<32)
		for repeat := range repeats {
			id := len(results) + 1
			results = append(results, &models.Result{
				ID:             uint(id),
				URL:            fmt.Sprintf("https://%d-%d.example/", group, repeat),
				Title:          fmt.Sprintf("Group %d", group),
				FaviconHash:    fmt.Sprintf("favicon-%d", group%25),
				PerceptionHash: hash,
			})
		}
	}

	report := Analyze(results, Options{HammingThreshold: 0})

	if report.TotalResults != 1000 {
		t.Fatalf("expected 1000 results, got %d", report.TotalResults)
	}
	if report.CanonicalResults != groups {
		t.Fatalf("expected %d canonical results, got %d", groups, report.CanonicalResults)
	}
	if report.DuplicatesMerged != groups*(repeats-1) {
		t.Fatalf("expected %d merged duplicates, got %d", groups*(repeats-1), report.DuplicatesMerged)
	}
	if len(report.ScreenshotClusters) != groups {
		t.Fatalf("expected %d duplicate screenshot groups, got %d", groups, len(report.ScreenshotClusters))
	}
	if len(report.FaviconClusters) != 25 {
		t.Fatalf("expected 25 favicon clusters, got %d", len(report.FaviconClusters))
	}
}

func BenchmarkAnalyzeThousand(b *testing.B) {
	results := make([]*models.Result, 1000)
	for i := range results {
		results[i] = &models.Result{
			ID:             uint(i + 1),
			URL:            fmt.Sprintf("https://%d.example/", i),
			Title:          fmt.Sprintf("Title %d", i%100),
			FaviconHash:    fmt.Sprintf("favicon-%d", i%40),
			PerceptionHash: fmt.Sprintf("p:%016x", uint64(i%250+1)<<32),
		}
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = Analyze(results, Options{HammingThreshold: 0})
	}
}

func BenchmarkEnrichResult(b *testing.B) {
	document := `<html><head><link rel="icon" href="/static/favicon.ico"></head><body>` +
		strings.Repeat("content ", 20_000) + `</body></html>`

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result := &models.Result{
			URL:   "https://target.example/path",
			Title: "  Target Console  ",
			HTML:  document,
		}
		EnrichResult(result)
	}
}
