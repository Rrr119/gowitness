package cmd

import (
	"github.com/sensepost/gowitness/internal/ascii"
	"github.com/spf13/cobra"
)

var dedupeCmd = &cobra.Command{
	Use:     "dedupe",
	Aliases: []string{"dedup", "duplicates", "cluster", "clusters", "deduplicate"},
	Short:   "Merge duplicate screenshots and cluster by favicon/title",
	Long: ascii.LogoHelp(ascii.Markdown(`
# dedupe

An alias for ` + "`gowitness report cluster`" + `. It analyses an existing report
offline and writes a compatible merged JSONL report plus machine-readable
cluster groups.
`)),
	Example: `
- gowitness dedupe --json-file gowitness.jsonl --output-file merged.jsonl
- gowitness cluster --input gowitness.sqlite3`,
	PreRunE: clusterCmd.PreRunE,
	RunE:    clusterCmd.RunE,
}

func init() {
	rootCmd.AddCommand(dedupeCmd)
	registerClusterFlags(dedupeCmd)
}
