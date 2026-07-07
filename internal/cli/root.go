package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

type Streams struct {
	Out io.Writer
	Err io.Writer
}

func NewRootCommand(streams Streams) *cobra.Command {
	streams = normalizeStreams(streams)

	command := &cobra.Command{
		Use:          "stackradar",
		Short:        "StackRadar dependency evidence uploader",
		Long:         "StackRadar dependency evidence uploader for CI environments.",
		SilenceUsage: true,
	}

	command.SetOut(streams.Out)
	command.SetErr(streams.Err)
	command.AddCommand(newBundleCommand(streams))
	command.AddCommand(newVersionCommand(streams))
	command.AddCommand(newUploadCommand(streams))

	return command
}

func normalizeStreams(streams Streams) Streams {
	if streams.Out == nil {
		streams.Out = os.Stdout
	}

	if streams.Err == nil {
		streams.Err = os.Stderr
	}

	return streams
}
