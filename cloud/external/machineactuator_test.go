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

package external_test

import (
	"flag"
	"testing"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/controller"

	"github.com/kubevirt/cluster-api-provider-external/cloud/external"
	"github.com/kubevirt/cluster-api-provider-external/cloud/external/machinesetup"
	configv1 "github.com/kubevirt/cluster-api-provider-external/cloud/external/providerconfig/v1alpha1"
	machineoptions "github.com/kubevirt/cluster-api-provider-external/cmd/external-controller/machine-controller-app/options"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterclient "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

var (
	kubeconfig string
)

func init() {
	// test_cmd_runner.RegisterCallback(tokenCreateCommandCallback)
	// test_cmd_runner.RegisterCallback(tokenCreateErrorCommandCallback)
	flag.StringVar(&kubeconfig, "kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
}

// const (
// 	tokenCreateCmdOutput = "c582f9.65a6f54fa78da5ae\n"
// 	tokenCreateCmdError  = "failed to load admin kubeconfig [open /etc/kubernetes/admin.conf: permission denied]"
// )

func TestActuatorSetup(t *testing.T) {
	flag.Parse()
	_, err := setupActuator(t)
	if err != nil {
		t.Fatalf("unable to set up actuator: %v", err)
	}
}

func TestClusterCreate(t *testing.T) {

	act, err := setupActuator(t)
	if err != nil {
		t.Fatalf("unable to set up actuator: %v", err)
	}

	config := newExtMachineProviderConfigFixture()
	config.Disks = make([]configv1.Disk, 0)

	cluster := newDefaultClusterFixture(t)
	machine := newMachine(t, config)

	err = act.Create(cluster, machine)
	if err != nil {
		t.Fatalf("unable to create cluster: %v", err)
	}
}

func setupActuator(t *testing.T) (*external.ExtClient, error) {

	machineServer := machineoptions.NewMachineControllerServer("machine_setup_configs.yaml")
	machineServer.CommonConfig.Kubeconfig = kubeconfig

	kubeConfig, err := controller.GetConfig(machineServer.CommonConfig.Kubeconfig)
	if err != nil {
		glog.Fatalf("Could not create Config (%v) for talking to the apiserver: %v", machineServer.CommonConfig.Kubeconfig, err)
	}

	clusterClient, err := clusterclient.NewForConfig(kubeConfig)
	if err != nil {
		glog.Fatalf("Could not create client for talking to the apiserver: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(
		rest.AddUserAgent(kubeConfig, "machine-controller-manager"),
	)
	if err != nil {
		glog.Fatalf("Invalid API configuration for kubeconfig-control: %v", err)
	}

	configWatch, err := machinesetup.NewConfigWatch(machineServer.MachineSetupConfigsPath)
	if err != nil {
		glog.Errorf("Could not create config watch: %v", err)
	}

	params := external.MachineActuatorParams{
		V1Alpha1Client:           clusterClient.ClusterV1alpha1(),
		ClientSet:                kubeClient,
		MachineSetupConfigGetter: configWatch,
		EventRecorder:            &record.FakeRecorder{},
	}

	return external.NewMachineActuator(params)
}

func newExtMachineProviderConfigFixture() configv1.ExtMachineProviderConfig {
	return configv1.ExtMachineProviderConfig{
		TypeMeta: v1.TypeMeta{
			APIVersion: "extproviderconfig/v1alpha1",
			Kind:       "ExtMachineProviderConfig",
		},
		Roles: []configv1.MachineRole{
			configv1.MasterRole,
		},
		Zone:  "default",
		OS:    "os-name",
		Disks: make([]configv1.Disk, 0),
		CRUDPrimitives: &configv1.CRUDConfig{
			ObjectMeta: v1.ObjectMeta{
				Name: "crud-test",
			},
			Container: &corev1.Container{
				Name:  "baremetal-fencing",
				Image: "quay.io/beekhof/fence-agents:0.0.1",
			},
			CheckCmd:       []string{"/bin/echo", "/sbin/fence_ipmilan", "--user", "admin", "-o", "status", "--ip", "1.2.3.4"},
			CreateCmd:      []string{"/bin/echo", "/sbin/fence_ipmilan", "--user", "admin", "-o", "on", "--ip", "1.2.3.4"},
			DeleteCmd:      []string{"/bin/echo", "/sbin/fence_ipmilan", "--user", "admin", "-o", "off", "--ip", "1.2.3.4"},
			ArgumentFormat: "cli",
			PassTargetAs:   "port",
			Secrets:        map[string]string{"password": "ipmi-secret"},
		},
	}
}

func newMachine(t *testing.T, extProviderConfig configv1.ExtMachineProviderConfig) *v1alpha1.Machine {
	providerConfigCodec, err := configv1.NewCodec()
	if err != nil {
		t.Fatalf("unable to create GCE provider config codec: %v", err)
	}
	providerConfig, err := providerConfigCodec.EncodeToProviderConfig(&extProviderConfig)
	if err != nil {
		t.Fatalf("unable to encode provider config: %v", err)
	}

	return &v1alpha1.Machine{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test1",
			Namespace: "default",
		},
		Spec: v1alpha1.MachineSpec{
			ProviderConfig: *providerConfig,
			Versions: v1alpha1.MachineVersionInfo{
				Kubelet:      "1.9.4",
				ControlPlane: "1.9.4",
			},
		},
	}
}

func newExtClusterProviderConfigFixture() configv1.ExtClusterProviderConfig {
	return configv1.ExtClusterProviderConfig{
		TypeMeta: v1.TypeMeta{
			APIVersion: "extproviderconfig/v1alpha1",
			Kind:       "ExtClusterProviderConfig",
		},
		Project: "project-name-2000",
	}
}

func newDefaultClusterFixture(t *testing.T) *v1alpha1.Cluster {
	providerConfigCodec, err := configv1.NewCodec()
	if err != nil {
		t.Fatalf("unable to create provider config codec: %v", err)
	}
	extProviderConfig := newExtClusterProviderConfigFixture()
	providerConfig, err := providerConfigCodec.EncodeToProviderConfig(&extProviderConfig)
	if err != nil {
		t.Fatalf("unable to encode provider config: %v", err)
	}

	return &v1alpha1.Cluster{
		TypeMeta: v1.TypeMeta{
			Kind: "Cluster",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "cluster-test",
		},
		Spec: v1alpha1.ClusterSpec{
			ProviderConfig: *providerConfig,
		},
	}
}
