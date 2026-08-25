package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// RegisterDocs registers documentation generation commands.
func RegisterDocs(rootCmd *cobra.Command) {
	var outputDir string

	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate CLI reference documentation",
		Long:  "Generate CLI reference documentation from the command tree.",
	}

	markdownCmd := &cobra.Command{
		Use:   "markdown",
		Short: "Generate Markdown CLI reference docs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			return doc.GenMarkdownTree(rootCmd, outputDir)
		},
	}

	manCmd := &cobra.Command{
		Use:   "man",
		Short: "Generate man pages",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			header := &doc.GenManHeader{Title: "PORT", Section: "1"}
			return doc.GenManTree(rootCmd, header, outputDir)
		},
	}

	docsCmd.PersistentFlags().StringVarP(&outputDir, "output", "o", filepath.Join("docs", "cli"), "Output directory")
	docsCmd.AddCommand(markdownCmd, manCmd)
	rootCmd.AddCommand(docsCmd)
}
