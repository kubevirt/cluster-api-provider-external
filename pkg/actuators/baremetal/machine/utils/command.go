/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/golang/glog"

	"kubevirt.io/cluster-api-provider-external/pkg/apis/baremetalproviderconfig/v1alpha1"
)

const actionStatus = "status"

const defaultFailedCode = 1

// RunProvisionCommand runs
func RunProvisionCommand(fencingConfig *v1alpha1.FencingConfig, action string) (bool, error) {
	cmd := "ansible-playbook"

	agentUsername, ok := fencingConfig.AgentSecret.Data["username"]
	if !ok {
		return true, fmt.Errorf("failed to get username from the agentSecret")
	}

	agentPassword, ok := fencingConfig.AgentSecret.Data["password"]
	if !ok {
		return true, fmt.Errorf("failed to get password from the agentSecret")
	}

	extraVars := []string{
		fmt.Sprintf("provision_action=%s", action),
		fmt.Sprintf("agent_address=%s", fencingConfig.AgentAddress),
		fmt.Sprintf("agent_type=%s", fencingConfig.AgentType),
		fmt.Sprintf("agent_username=%s", agentUsername),
		fmt.Sprintf("agent_password=%s", agentPassword),
	}

	agentOptions := []string{}
	for k, v := range fencingConfig.AgentOptions {
		arg := fmt.Sprintf("--%s=%s", k, v)
		if v == "" {
			arg = fmt.Sprintf("--%s", k)
		}
		agentOptions = append(agentOptions, arg)
	}

	if len(agentOptions) != 0 {
		extraVars = append(
			extraVars,
			fmt.Sprintf("%s=\"%s\"", "agent_options", strings.Join(agentOptions, " ")),
		)
	}

	args := []string{
		"/home/non-root/ansible/provision.yml",
		fmt.Sprintf("--extra-vars=%s", strings.Join(extraVars, " ")),
	}

	glog.Infof("run provisioning command %s with args: %v", cmd, args)
	_, stderr, rc := runCommand(cmd, args...)

	if rc == 0 {
		return true, nil
	}

	if rc == 2 && action == actionStatus {
		return false, nil
	}
	
	return false, fmt.Errorf(stderr)
}

func runCommand(command string, args ...string) (stdout string, stderr string, exitCode int) {
	var outbuf, errbuf bytes.Buffer
	cmd := exec.Command(command, args...)
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	err := cmd.Run()
	stdout = outbuf.String()
	stderr = errbuf.String()

	if err != nil {
		// try to get the exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode = ws.ExitStatus()
		} else {
			glog.Warningf("Could not get exit code for failed program: %v, %v", command, args)
			exitCode = defaultFailedCode
			if stderr == "" {
				stderr = err.Error()
			}
		}
	} else {
		// success, exitCode should be 0 if go is ok
		ws := cmd.ProcessState.Sys().(syscall.WaitStatus)
		exitCode = ws.ExitStatus()
	}
	glog.Infof("command result, stdout: %v, stderr: %v, exitCode: %v", stdout, stderr, exitCode)
	return
}
