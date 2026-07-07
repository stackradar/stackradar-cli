package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackradar/stackradar-cli/internal/buildinfo"
	"github.com/stackradar/stackradar-cli/internal/upload"
)

const uploadTokenEnv = "STACKRADAR_TOKEN"

var ErrUploadTokenRequired = errors.New("upload token is required; pass --token or set STACKRADAR_TOKEN")

func newUploadCommand(streams Streams) *cobra.Command {
	var apiURL string
	var token string
	var dryRun bool
	var verbose bool

	command := &cobra.Command{
		Use:   "upload <bundle.zip>",
		Short: "Upload a dependency evidence bundle",
		Long:  "Upload a dependency evidence bundle created by stackradar bundle.",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			bundlePath := args[0]

			if dryRun {
				metadata, err := upload.InspectBundle(bundlePath)
				if err != nil {
					return err
				}

				return writeUploadDryRunReport(streams.Out, bundlePath, apiURL, metadata)
			}

			resolvedToken, err := resolveUploadToken(token)
			if err != nil {
				return err
			}

			result, err := upload.UploadBundle(upload.UploadOptions{
				APIURL:        apiURL,
				Token:         resolvedToken,
				BundlePath:    bundlePath,
				ClientName:    "stackradar-cli",
				ClientVersion: buildinfo.Current().Version,
			})
			if err != nil {
				return err
			}

			return writeUploadReport(streams.Out, result)
		},
	}

	command.Flags().StringVar(&apiURL, "api-url", "https://stackradar.com", "StackRadar API base URL")
	command.Flags().StringVar(&token, "token", "", "Upload authentication token; defaults to STACKRADAR_TOKEN")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the bundle and print upload metadata without uploading")
	command.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose diagnostic output")

	return command
}

func resolveUploadToken(flagToken string) (string, error) {
	if flagToken != "" {
		return flagToken, nil
	}

	envToken := os.Getenv(uploadTokenEnv)
	if envToken != "" {
		return envToken, nil
	}

	return "", ErrUploadTokenRequired
}

func writeUploadReport(writer io.Writer, result upload.UploadResult) error {
	if _, err := fmt.Fprintln(writer, "Bundle uploaded:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  upload_id: %s\n", result.UploadID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  artifact_id: %s\n", result.ArtifactID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  status: %s\n", result.Status); err != nil {
		return err
	}

	return nil
}

func writeUploadDryRunReport(writer io.Writer, bundlePath string, apiURL string, metadata upload.BundleMetadata) error {
	if _, err := fmt.Fprintln(writer, "Upload dry run:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  bundle: %s\n", bundlePath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  format: %s\n", metadata.Format); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  content_type: %s\n", metadata.ContentType); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  size: %s\n", formatBytes(metadata.SizeBytes)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  sha256: %s\n", metadata.SHA256); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  api_url: %s\n", apiURL); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, "  status: dry-run"); err != nil {
		return err
	}

	return nil
}
