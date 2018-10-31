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
		Use:   "fencing-agents",
		Short: "fencing-agents helps you to call fence actions on the host",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStderr(), cmd.UsageString())
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String("type", "", "Fencing device type")
	root.PersistentFlags().String("action", "", "Power management action(status, reboot, off or on)")
	root.PersistentFlags().String("ip", "", "IP address or hostname of fencing device")
	root.PersistentFlags().String("username-secret", "", "Username secret file path")
	root.PersistentFlags().String("password-secret", "", "Password secret file path")

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
