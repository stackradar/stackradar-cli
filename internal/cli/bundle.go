package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackradar/stackradar-cli/internal/buildinfo"
	"github.com/stackradar/stackradar-cli/internal/upload"
)

const defaultBundleOutputPath = "stackradar.zip"

func newBundleCommand(streams Streams) *cobra.Command {
	var path string
	var excludes []string
	outputPath := defaultBundleOutputPath

	command := &cobra.Command{
		Use:   "bundle",
		Short: "Bundle dependency evidence without uploading it",
		Long:  "Discover dependency manifests and lockfiles, package them into a deterministic zip bundle, and write it locally.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			discovery, bundle, err := prepareBundle(path, excludes)
			if err != nil {
				return err
			}

			if err := os.WriteFile(outputPath, bundle.Bytes, 0o644); err != nil {
				return err
			}

			return writeBundleReport(streams.Out, discovery, bundle, outputPath)
		},
	}

	command.Flags().StringVar(&path, "path", ".", "Repository path to scan")
	command.Flags().StringArrayVar(&excludes, "exclude", nil, "Glob pattern to exclude from discovery; repeat for multiple patterns")
	command.Flags().StringVar(&outputPath, "output", defaultBundleOutputPath, "Path to write the upload bundle zip")

	return command
}

func prepareBundle(root string, excludes []string) (upload.Discovery, upload.Bundle, error) {
	discovery, err := upload.Discover(upload.DiscoverOptions{
		Root:     root,
		Excludes: excludes,
	})
	if err != nil {
		return upload.Discovery{}, upload.Bundle{}, err
	}

	bundle, err := upload.BuildBundleWithOptions(upload.BuildBundleOptions{
		Root:          root,
		Files:         discovery.Files,
		ClientName:    "stackradar-cli",
		ClientVersion: buildinfo.Version,
	})
	if err != nil {
		return upload.Discovery{}, upload.Bundle{}, err
	}

	return discovery, bundle, nil
}

func writeBundleReport(writer io.Writer, discovery upload.Discovery, bundle upload.Bundle, outputPath string) error {
	if _, err := fmt.Fprintln(writer, "Discovered dependency files:"); err != nil {
		return err
	}

	for _, file := range discovery.Files {
		if _, err := fmt.Fprintf(writer, "  - %s (%s, %s)\n", file.Path, file.Ecosystem, formatBytes(file.SizeBytes)); err != nil {
			return err
		}
	}

	if len(discovery.SkippedDirectories) > 0 {
		if _, err := fmt.Fprintln(writer, "Skipped directories:"); err != nil {
			return err
		}

		for _, directory := range discovery.SkippedDirectories {
			if _, err := fmt.Fprintf(writer, "  - %s (%s)\n", directory.Path, directory.Reason); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(writer, "Bundle:"); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "  output: %s\n", outputPath); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "  files: %d\n", len(bundle.Files)); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "  size: %s\n", formatBytes(bundle.SizeBytes)); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "  sha256: %s\n", bundle.SHA256); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer, "bundle written."); err != nil {
		return err
	}

	return nil
}

func formatBytes(size int64) string {
	if size == 1 {
		return "1 byte"
	}

	return fmt.Sprintf("%d bytes", size)
}
