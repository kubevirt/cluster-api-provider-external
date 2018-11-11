package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

func NewFenceCommand() *cobra.Command {

	fence := &cobra.Command{
		Use:   "fence",
		Short: "run fencing command on the host",
		RunE:  fence,
		Args:  cobra.ArbitraryArgs,
	}

	fence.PersistentFlags().String("agent-type", "", "Fencing agent type")
	fence.PersistentFlags().String("secret-path", "", "Path to the secret that contains fencing agent username and password")
	fence.PersistentFlags().StringP("action", "o", "", "Fencing action(status, reboot, off or on)")
	return fence
}

func fence(cmd *cobra.Command, args []string) (err error) {
	// Set power management agent type
	fenceAgentType, err := cmd.Flags().GetString("agent-type")
	if err != nil {
		return err
	}
	fenceCommand := filepath.Join("sbin", fmt.Sprintf("fence_%s", fenceAgentType))

	fenceArgs := []string{}
	if args != nil {
		fenceArgs = append(fenceArgs, args...)
	}

	
	secretPath, err := cmd.Flags().GetString("secret-path")
	if err != nil {
		return err
	}
	// Set power management username
	username, err := ioutil.ReadFile(filepath.Join(secretPath, "username"))
	if err != nil {
		return err
	}
	fenceArgs = append(fenceArgs, fmt.Sprintf("--username=%s", username))

	// Set power management password
	password, err := ioutil.ReadFile(filepath.Join(secretPath, "password"))
	if err != nil {
		return err
	}
	fenceArgs = append(fenceArgs, fmt.Sprintf("--password=%s", password))

	// Set power management action
	action, err := cmd.Flags().GetString("action")
	if err != nil {
		return err
	}
	fenceArgs = append(fenceArgs, fmt.Sprintf("--action=%s", action))

	// Set additional arguments
	options, err := cmd.Flags().GetString("options")
	if err == nil && options != "" {
		optionList := strings.Split(options, ",")
		for _, option := range optionList {
			keyVal := strings.Split(option, "=")
			if len(keyVal) != 2 {
				return fmt.Errorf("incorrect option format, please use \"key1=value1,...,keyn=valuen\"")
			}
			fenceArgs = append(fenceArgs, fmt.Sprintf("--%s=%s", keyVal[0], keyVal[1]))
		}
	}

	glog.Infof("run fence command %s with arguments %s", fenceCommand, fenceArgs)
	// Do not run command if dry-run is true
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}

	// Execute fence command
	var out bytes.Buffer
	execCmd := exec.Command(fenceCommand, fenceArgs...)
	execCmd.Stdout = &out
	err = execCmd.Run()
	if err != nil {
		return err
	}
	glog.Infof("fence command output: %s", out.String())
	return nil
}
