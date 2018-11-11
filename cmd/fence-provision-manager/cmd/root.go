package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewRootCommand() *cobra.Command {

	root := &cobra.Command{
		Use:   "fence-provision-manager",
		Short: "fence-provision-manager can execute fencing and provisioning actions",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStderr(), cmd.UsageString())
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.Flags().Bool("dry-run", false, "Dry run of the command, it will only log the command, but will not execute it")
	root.Flags().String("options", "", "Additional options passed to the command(key=value,...)")

	root.AddCommand(
		NewFenceCommand(),
	)

	return root

}

func Execute() {
	flag.Set("logtostderr", "true")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	if err := NewRootCommand().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
