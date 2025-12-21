package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ansel",
	Short: "A CLI tool for image processing",
	Long: `Ansel is a command-line image processing tool that resizes and frames
images for social media and print.

Features:
  - Linear light resizing using Magic Kernel Sharp 2021
  - Automatic framing with configurable colors and widths
  - Size presets for Instagram, Facebook, Twitter/X, YouTube, LinkedIn
  - High-quality JPEG output

Named after Ansel Adams, the legendary photographer known for his
meticulous attention to image quality.

Example:
  ansel process --size ig-post photo.jpg`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
