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
		Args:  cobra.NoArgs,
	}

	fence.PersistentFlags().String("agent-type", "", "Fencing device type")
	fence.PersistentFlags().String("action", "", "Power management action(status, reboot, off or on)")
	fence.PersistentFlags().String("ip", "", "IP address or hostname of fencing device")
	fence.PersistentFlags().String("username-secret", "", "Username secret file path")
	fence.PersistentFlags().String("password-secret", "", "Password secret file path")
	return fence
}

func fence(cmd *cobra.Command, _ []string) (err error) {
	fenceArgs := []string{}

	// Set power management agent type
	fenceAgentType, err := cmd.Flags().GetString("agent-type")
	if err != nil {
		return err
	}
	fenceCommand := filepath.Join("sbin", fmt.Sprintf("fence_%s", fenceAgentType))

	// Set power management action
	fenceAction, err := cmd.Flags().GetString("action")
	if err != nil {
		return err
	}
	fenceArgs = append(fenceArgs, fmt.Sprintf("--action=%s", fenceAction))

	// Set power management target host
	targetHost, err := cmd.Flags().GetString("ip")
	if err != nil {
		return err
	}
	fenceArgs = append(fenceArgs, fmt.Sprintf("--ip=%s", targetHost))

	// Set power management username
	usernameSecret, err := cmd.Flags().GetString("username-secret")
	if err != nil {
		return err
	}
	username, err := ioutil.ReadFile(usernameSecret)
	if err != nil {
		return err
	}
	fenceArgs = append(fenceArgs, fmt.Sprintf("--username=%s", username))

	// Set power management password
	passwordSecret, err := cmd.Flags().GetString("password-secret")
	if err != nil {
		return err
	}
	password, err := ioutil.ReadFile(passwordSecret)
	if err != nil {
		return err
	}
	fenceArgs = append(fenceArgs, fmt.Sprintf("--password=%s", password))

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
