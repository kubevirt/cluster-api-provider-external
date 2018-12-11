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

package machine

import (
	"fmt"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterclient "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/errors"

	"kubevirt.io/cluster-api-provider-external/pkg/actuators/baremetal/machine/utils"
	"kubevirt.io/cluster-api-provider-external/pkg/apis/baremetalproviderconfig/v1alpha1"
)

const (
	actionStatus = "status"
	actionCreate = "create"
	actionDelete = "delete"
)

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	clusterclient clusterclient.Interface
	kubeclient    kubernetes.Interface
	eventRecorder record.EventRecorder
	codec         codec
}

type codec interface {
	DecodeFromProviderConfig(clusterv1.ProviderConfig, runtime.Object) error
}

// NewActuator creates a new Actuator
func NewActuator(kubeclient kubernetes.Interface, clusterclient clusterclient.Interface) (*Actuator, error) {
	codec, err := v1alpha1.NewCodec()
	if err != nil {
		return nil, err
	}

	glog.V(2).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events(metav1.NamespaceAll)})

	return &Actuator{
		codec:         codec,
		kubeclient:    kubeclient,
		clusterclient: clusterclient,
		eventRecorder: eventBroadcaster.NewRecorder(kubescheme.Scheme, corev1.EventSource{Component: "machine-actuator"}),
	}, nil
}

// Create runs create action on the machine
func (c *Actuator) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("run create action on the machine %s", machine.Name)
	if _, err := c.executeAction(actionCreate, cluster, machine); err != nil {
		glog.Errorf("failed to run create action %s: %v", machine.Name, err)
		return c.handleMachineError(
			machine,
			errors.CreateMachine("error creating the instance: %v", err),
			actionCreate,
		)
	}

	c.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Created", "Created the Machine %s", machine.Name)
	return nil
}

// Delete runs delete action on the machine
func (c *Actuator) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("run delete action on the machine %s", machine.Name)
	if _, err := c.executeAction(actionDelete, cluster, machine); err != nil {
		glog.Errorf("failed to run delete action %s: %v", machine.Name, err)
		return c.handleMachineError(
			machine,
			errors.DeleteMachine("error deleting the instance: %v", err),
			actionDelete,
		)
	}

	c.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Deleted", "Deleted the Machine %s", machine.Name)
	return nil
}

// Update does not run any code
func (c *Actuator) Update(cluster *clusterv1.Cluster, goalMachine *clusterv1.Machine) error {
	glog.Infof("NOT IMPLEMENTED: update machine %s", goalMachine.Name)
	return nil
}

// Exists returns true, if machine is power-on
func (c *Actuator) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	glog.Infof("Checking if machine %s is existing", machine.Name)
	state, err := c.executeAction(actionStatus, cluster, machine)
	if err != nil {
		return false, err
	}

	glog.Infof("Machine %s has status equals to %b", machine.Name, state)
	return state, nil
}

// ProviderConfigMachine gets the machine provider config MachineSetSpec from the
// specified cluster-api MachineSpec.
func ProviderConfigMachine(codec codec, ms *clusterv1.MachineSpec) (*v1alpha1.BareMetalMachineProviderConfig, error) {
	if ms.ProviderConfig.Value == nil {
		return nil, fmt.Errorf("no Value in ProviderConfig")
	}

	var config v1alpha1.BareMetalMachineProviderConfig
	if err := codec.DecodeFromProviderConfig(ms.ProviderConfig, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Actuator) executeAction(action string, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	machineProviderConfig, err := ProviderConfigMachine(c.codec, &machine.Spec)
	if err != nil {
		return false, err
	}
	return utils.RunProvisionCommand(machineProviderConfig.FencingConfig, action)
}

func (c *Actuator) machineproviderconfig(providerConfig clusterv1.ProviderConfig) (*v1alpha1.BareMetalMachineProviderConfig, error) {
	var config v1alpha1.BareMetalMachineProviderConfig
	err := c.codec.DecodeFromProviderConfig(providerConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Actuator) handleMachineError(machine *clusterv1.Machine, err *errors.MachineError, action string) error {
	reason := err.Reason
	message := err.Message
	machine.Status.ErrorReason = &reason
	machine.Status.ErrorMessage = &message
	c.clusterclient.ClusterV1alpha1().Machines(machine.Namespace).UpdateStatus(machine)

	c.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+action, "%v", err.Reason)

	glog.Errorf("Machine error: %v", err.Message)
	return err
}
