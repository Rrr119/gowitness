package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sensepost/gowitness/internal/ascii"
	"github.com/sensepost/gowitness/pkg/clustering"
	"github.com/sensepost/gowitness/pkg/database"
	"github.com/sensepost/gowitness/pkg/imagehash"
	"github.com/sensepost/gowitness/pkg/log"
	"github.com/sensepost/gowitness/pkg/models"
	"github.com/sensepost/gowitness/pkg/writers"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"image"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

var clusterCmdFlags = struct {
	InputFile         string
	JSONFile          string
	DbURI             string
	OutputFile        string
	ClusterFile       string
	MergedJSONL       string
	MergedCSV         string
	ScreenshotPath    string
	Threshold         int
	IncludeSingles    bool
	UseExistingGroups bool
	NoHydrate         bool
	ScreenshotsOnly   bool
	UpdateDB          bool
	MergeOutput       bool
	Quiet             bool
}{
	DbURI:          "sqlite://gowitness.sqlite3",
	ClusterFile:    "gowitness-clusters.json",
	MergedJSONL:    "gowitness-dedup.jsonl",
	ScreenshotPath: "./screenshots",
	Threshold:      clustering.DefaultHammingThreshold,
	UpdateDB:       true,
	MergeOutput:    true,
}

var clusterCmd = &cobra.Command{
	Use:     "cluster",
	Aliases: []string{"clusters", "dedupe", "dedup", "deduplicate"},
	Short:   "Cluster results by screenshot similarity, favicon and title",
	Long: ascii.LogoHelp(ascii.Markdown(`
# report cluster

Analyse an existing JSON Lines or SQLite report offline. Screenshots are
merged using perceptual hash distance; favicon and title views are produced
alongside a combined favicon+title cluster view.

This command never performs target or favicon network requests, so scanning
and single-target latency remain unchanged. The merged JSONL output uses the
normal gowitness Result document shape and can be passed to ` + "`report generate`" + `.
`)),
	Example: ascii.Markdown(`
- gowitness report cluster --json-file gowitness.jsonl
- gowitness report dedupe --json-file gowitness.jsonl --output-file merged.jsonl
- gowitness report cluster --db-uri sqlite://gowitness.sqlite3 --threshold 8`),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if clusterCmdFlags.InputFile == "" && len(args) > 0 {
			clusterCmdFlags.InputFile = args[0]
		}
		if clusterCmdFlags.OutputFile == "" && len(args) > 1 {
			clusterCmdFlags.OutputFile = args[1]
		}

		if clusterCmdFlags.InputFile != "" {
			ext := strings.ToLower(filepath.Ext(clusterCmdFlags.InputFile))
			switch ext {
			case ".jsonl", ".json":
				if clusterCmdFlags.JSONFile == "" {
					clusterCmdFlags.JSONFile = clusterCmdFlags.InputFile
				}
			case ".sqlite", ".sqlite3", ".db":
				clusterCmdFlags.DbURI = "sqlite://" + clusterCmdFlags.InputFile
			default:
				return fmt.Errorf("unsupported --input file type %q", ext)
			}
		}

		if clusterCmdFlags.JSONFile == "" && clusterCmdFlags.InputFile == "" && !clusterCmdFlags.ScreenshotsOnly {
			if !cmd.Flags().Changed("db-uri") {
				if _, err := os.Stat("gowitness.sqlite3"); errors.Is(err, os.ErrNotExist) && directoryHasImages(clusterCmdFlags.ScreenshotPath) {
					clusterCmdFlags.ScreenshotsOnly = true
				}
			}
		}

		if clusterCmdFlags.JSONFile == "" && clusterCmdFlags.DbURI == "" && !clusterCmdFlags.ScreenshotsOnly {
			return errors.New("one of --json-file, --db-uri, --input or --screenshots-only must be set")
		}

		if clusterCmdFlags.OutputFile != "" {
			ext := strings.ToLower(filepath.Ext(clusterCmdFlags.OutputFile))
			if ext == ".jsonl" {
				clusterCmdFlags.MergedJSONL = clusterCmdFlags.OutputFile
			} else if ext == ".csv" {
				clusterCmdFlags.MergedCSV = clusterCmdFlags.OutputFile
			} else if ext == ".json" {
				clusterCmdFlags.ClusterFile = clusterCmdFlags.OutputFile
			} else {
				return fmt.Errorf("unsupported --output-file extension %q (use .json, .jsonl or .csv)", ext)
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		results, conn, err := loadClusterResults()
		if err != nil {
			return fmt.Errorf("could not load results for clustering: %w", err)
		}

		if !clusterCmdFlags.NoHydrate {
			hydrateClusterResults(results)
		}

		report := clustering.Analyze(results, clustering.Options{
			HammingThreshold:            clusterCmdFlags.Threshold,
			IncludeSingletons:           clusterCmdFlags.IncludeSingles,
			UseExistingScreenshotGroups: clusterCmdFlags.UseExistingGroups,
		})

		if err := writeClusterReport(report, clusterCmdFlags.ClusterFile); err != nil {
			return fmt.Errorf("could not write cluster report %q: %w", clusterCmdFlags.ClusterFile, err)
		}

		if clusterCmdFlags.MergeOutput && strings.TrimSpace(clusterCmdFlags.MergedJSONL) != "" {
			if err := writeMergedJSONL(report.Canonical, clusterCmdFlags.MergedJSONL); err != nil {
				return fmt.Errorf("could not write merged JSONL %q: %w", clusterCmdFlags.MergedJSONL, err)
			}
		}

		if strings.TrimSpace(clusterCmdFlags.MergedCSV) != "" {
			if err := writeMergedCSV(report.Canonical, clusterCmdFlags.MergedCSV); err != nil {
				return fmt.Errorf("could not write merged CSV %q: %w", clusterCmdFlags.MergedCSV, err)
			}
		}

		if conn != nil && clusterCmdFlags.UpdateDB {
			if err := persistClusterMetadata(conn, results); err != nil {
				return fmt.Errorf("could not persist cluster metadata to database: %w", err)
			}
		}

		log.Info("clustering complete",
			"total", report.TotalResults,
			"canonical", report.CanonicalResults,
			"duplicates-merged", report.DuplicatesMerged,
			"screenshot-clusters", len(report.ScreenshotClusters),
			"favicon-clusters", len(report.FaviconClusters),
			"title-clusters", len(report.TitleClusters),
			"combined-clusters", len(report.CombinedClusters),
			"report", clusterCmdFlags.ClusterFile,
			"merged-jsonl", clusterCmdFlags.MergedJSONL,
		)
		return nil
	},
}

func init() {
	reportCmd.AddCommand(clusterCmd)
	registerClusterFlags(clusterCmd)
}

func registerClusterFlags(command *cobra.Command) {
	command.Flags().StringVarP(&clusterCmdFlags.InputFile, "input", "i", "", "Input report (.jsonl, .json, .sqlite3)")
	command.Flags().StringVar(&clusterCmdFlags.InputFile, "input-file", "", "Alias for --input")
	command.Flags().StringVar(&clusterCmdFlags.InputFile, "from-file", "", "Input report path (compatibility alias)")
	command.Flags().StringVar(&clusterCmdFlags.InputFile, "source", "", "Input report path (compatibility alias)")
	command.Flags().StringVar(&clusterCmdFlags.JSONFile, "json-file", "", "JSON Lines or JSON array report to read")
	command.Flags().StringVar(&clusterCmdFlags.DbURI, "db-uri", "sqlite://gowitness.sqlite3", "Database URI to read and annotate")
	command.Flags().StringVarP(&clusterCmdFlags.OutputFile, "output-file", "o", "", "Output: .json for cluster analysis, .jsonl for merged results")
	command.Flags().StringVar(&clusterCmdFlags.OutputFile, "output", "", "Alias for --output-file")
	command.Flags().StringVar(&clusterCmdFlags.OutputFile, "to-file", "", "Output path (compatibility alias; .json or .jsonl)")
	command.Flags().StringVar(&clusterCmdFlags.OutputFile, "destination", "", "Output path (compatibility alias)")
	command.Flags().StringVar(&clusterCmdFlags.ClusterFile, "cluster-file", "gowitness-clusters.json", "JSON file containing all cluster views")
	command.Flags().StringVar(&clusterCmdFlags.MergedJSONL, "merged-jsonl", "gowitness-dedup.jsonl", "Compatible JSONL file containing canonical results")
	command.Flags().StringVar(&clusterCmdFlags.MergedCSV, "merged-csv", "", "Optional compatible CSV file containing canonical results")
	command.Flags().StringVarP(&clusterCmdFlags.ScreenshotPath, "screenshot-path", "s", "./screenshots", "Directory used when a result lacks perception_hash")
	command.Flags().IntVar(&clusterCmdFlags.Threshold, "threshold", clustering.DefaultHammingThreshold, "Maximum inclusive pHash Hamming distance")
	command.Flags().IntVar(&clusterCmdFlags.Threshold, "hamming-threshold", clustering.DefaultHammingThreshold, "Alias for --threshold")
	command.Flags().BoolVar(&clusterCmdFlags.IncludeSingles, "include-singletons", false, "Include unique groups in JSON cluster arrays")
	command.Flags().BoolVar(&clusterCmdFlags.UseExistingGroups, "use-existing-groups", false, "Trust existing perception_hash_group_id values")
	command.Flags().BoolVar(&clusterCmdFlags.NoHydrate, "no-hydrate", false, "Do not decode embedded or file screenshots to fill missing hashes")
	command.Flags().BoolVar(&clusterCmdFlags.ScreenshotsOnly, "screenshots-only", false, "Treat --screenshot-path as an image source when no report is available")
	command.Flags().BoolVar(&clusterCmdFlags.UpdateDB, "update-db", true, "Write cluster IDs and duplicate markers back to the source database")
	command.Flags().BoolVar(&clusterCmdFlags.MergeOutput, "merge", true, "Write canonical merged JSONL output")
	command.Flags().BoolVar(&clusterCmdFlags.MergeOutput, "merge-duplicates", true, "Alias for --merge")
	command.Flags().BoolVar(&clusterCmdFlags.MergeOutput, "deduplicate", true, "Alias for --merge")
}

func loadClusterResults() ([]*models.Result, *gorm.DB, error) {
	if clusterCmdFlags.ScreenshotsOnly {
		results, err := loadScreenshotResults(clusterCmdFlags.ScreenshotPath)
		return results, nil, err
	}

	if clusterCmdFlags.JSONFile != "" {
		results, err := loadClusterJSON(clusterCmdFlags.JSONFile)
		return results, nil, err
	}

	conn, err := database.Connection(clusterCmdFlags.DbURI, true, false)
	if err != nil {
		return nil, nil, err
	}

	var results []*models.Result
	if err := conn.Model(&models.Result{}).
		Order("id ASC").
		Preload(clause.Associations).
		Preload("TLS.SanList").
		Find(&results).Error; err != nil {
		return nil, conn, err
	}

	return results, conn, nil
}

func directoryHasImages(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg", ".gif":
			return true
		}
	}
	return false
}

func loadScreenshotResults(directory string) ([]*models.Result, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	results := make([]*models.Result, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" {
			continue
		}

		absolutePath, err := filepath.Abs(filepath.Join(directory, entry.Name()))
		if err != nil {
			absolutePath = entry.Name()
		}
		results = append(results, &models.Result{
			ID:       uint(len(results) + 1),
			URL:      "file://" + absolutePath,
			Filename: entry.Name(),
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no screenshots found in %q", directory)
	}
	return results, nil
}

func loadClusterJSON(path string) ([]*models.Result, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return []*models.Result{}, nil
	}

	if trimmed[0] == '[' {
		var values []models.Result
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return nil, err
		}
		results := make([]*models.Result, len(values))
		for i := range values {
			results[i] = &values[i]
		}
		return results, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	results := []*models.Result{}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				if len(bytes.TrimSpace(line)) == 0 {
					break
				}
			} else {
				return nil, err
			}
		}

		if trimmedLine := bytes.TrimSpace(line); len(trimmedLine) > 0 {
			var result models.Result
			if unmarshalErr := json.Unmarshal(trimmedLine, &result); unmarshalErr != nil {
				log.Error("skipping invalid JSON line", "err", unmarshalErr)
			} else {
				results = append(results, &result)
			}
		}

		if err == io.EOF {
			break
		}
	}

	return results, nil
}

func hydrateClusterResults(results []*models.Result) {
	for _, result := range results {
		if result == nil {
			continue
		}

		hydrateFaviconHashFromNetwork(result)

		if strings.TrimSpace(result.PerceptionHash) != "" {
			continue
		}

		var imageBytes []byte
		if result.Screenshot != "" {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(result.Screenshot, "data:image/jpeg;base64,"))
			if err == nil {
				imageBytes = decoded
			}
		}

		if len(imageBytes) == 0 && result.Filename != "" {
			path := result.Filename
			if !filepath.IsAbs(path) {
				path = filepath.Join(clusterCmdFlags.ScreenshotPath, result.Filename)
			}
			if data, err := os.ReadFile(path); err == nil {
				imageBytes = data
			}
		}

		if len(imageBytes) == 0 {
			continue
		}

		result.ScreenshotHash = hashBytes(imageBytes)
		decoded, _, err := image.Decode(bytes.NewReader(imageBytes))
		if err != nil {
			continue
		}
		if hash, err := imagehash.PerceptionHash(decoded); err == nil {
			result.PerceptionHash = hash
		}
	}
}

func hydrateFaviconHashFromNetwork(result *models.Result) {
	if result.FaviconHash != "" {
		return
	}

	for _, networkLog := range result.Network {
		if len(networkLog.Content) == 0 {
			continue
		}
		urlLower := strings.ToLower(networkLog.URL)
		mimeLower := strings.ToLower(networkLog.MIMEType)
		if strings.Contains(urlLower, "favicon") || strings.Contains(mimeLower, "icon") {
			result.FaviconHash = hashBytes(networkLog.Content)
			if result.Favicon == "" {
				result.Favicon = networkLog.URL
			}
			return
		}
	}
}

func writeClusterReport(report *clustering.Report, path string) error {
	if strings.TrimSpace(path) == "" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func writeMergedJSONL(results []*models.Result, path string) error {
	writer, err := writers.NewJsonWriter(path)
	if err != nil {
		return err
	}

	for _, result := range results {
		if result == nil {
			continue
		}
		if err := writer.Write(result); err != nil {
			return err
		}
	}

	return nil
}

func writeMergedCSV(results []*models.Result, path string) error {
	writer, err := writers.NewCsvWriter(path)
	if err != nil {
		return err
	}

	for _, result := range results {
		if result == nil {
			continue
		}
		if err := writer.Write(result); err != nil {
			return err
		}
	}

	return nil
}

func persistClusterMetadata(conn *gorm.DB, results []*models.Result) error {
	for _, result := range results {
		if result == nil || result.ID == 0 {
			continue
		}

		updates := map[string]interface{}{
			"favicon":                  result.Favicon,
			"favicon_url":              result.FaviconURL,
			"favicon_hash":             result.FaviconHash,
			"screenshot_hash":          result.ScreenshotHash,
			"title_normalized":         result.TitleNormalized,
			"perception_hash_group_id": result.PerceptionHashGroupId,
			"screenshot_cluster_id":    result.ScreenshotClusterID,
			"cluster_id":               result.ClusterID,
			"favicon_cluster_id":       result.FaviconClusterID,
			"title_cluster_id":         result.TitleClusterID,
			"duplicate_of_id":          result.DuplicateOfID,
			"canonical_url":            result.CanonicalURL,
			"is_duplicate":             result.IsDuplicate,
			"duplicate_count":          result.DuplicateCount,
			"duplicate_urls":           duplicateURLsForDB(result.DuplicateURLs),
		}

		if err := conn.Model(&models.Result{}).Where("id = ?", result.ID).Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}

func duplicateURLsForDB(values []string) interface{} {
	if values == nil {
		return nil
	}

	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
