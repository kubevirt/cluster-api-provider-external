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
	"context"
	"fmt"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
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
	actionStatus       = "status"
	actionCreate       = "create"
	actionDelete       = "delete"
	actionUpdateStatus = "updateStatus"
)

var MachineActuator *Actuator

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	clusterclient clusterclient.Interface
	kubeclient    kubernetes.Interface
	eventRecorder record.EventRecorder
	codec         codec
}

type codec interface {
	DecodeFromProviderSpec(clusterv1.ProviderSpec, runtime.Object) error
	DecodeProviderStatus(*runtime.RawExtension, runtime.Object) error
	EncodeProviderStatus(runtime.Object) (*runtime.RawExtension, error)
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
func (a *Actuator) Create(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("run create action on the machine %s", machine.Name)
	if _, err := a.executeAction(actionCreate, machine); err != nil {
		glog.Errorf("failed to run create action %s: %v", machine.Name, err)
		return a.handleMachineError(
			machine,
			errors.CreateMachine("error creating the instance: %v", err),
			actionCreate,
		)
	}
	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Created", "Created the Machine %s", machine.Name)

	if err := a.updateStatus(machine); err != nil {
		return fmt.Errorf("%s/%s: error updating machine status: %v", cluster.Name, machine.Name, err)
	}
	return nil
}

// Delete runs delete action on the machine
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("run delete action on the machine %s", machine.Name)
	if _, err := a.executeAction(actionDelete, machine); err != nil {
		glog.Errorf("failed to run delete action %s: %v", machine.Name, err)
		return a.handleMachineError(
			machine,
			errors.DeleteMachine("error deleting the instance: %v", err),
			actionDelete,
		)
	}

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Deleted", "Deleted the Machine %s", machine.Name)
	return nil
}

// Update does not run any code
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)

	if err := a.updateStatus(machine); err != nil {
		return fmt.Errorf("%s/%s: error updating machine status: %v", cluster.Name, machine.Name, err)
	}

	return nil
}

// Exists returns true, if machine is power-on
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	glog.Infof("Checking if machine %s is existing", machine.Name)
	state, err := a.executeAction(actionStatus, machine)
	if err != nil {
		return false, err
	}

	glog.Infof("Machine %s has status equals to %v", machine.Name, state)
	return state, nil
}

// ProviderSpecMachine gets the machine provider config MachineSetSpec from the
// specified cluster-api MachineSpea.
func ProviderSpecMachine(codec codec, ms *clusterv1.MachineSpec) (*v1alpha1.BareMetalMachineProviderSpec, error) {
	if ms.ProviderSpec.Value == nil {
		return nil, fmt.Errorf("no Value in ProviderSpec")
	}

	var config v1alpha1.BareMetalMachineProviderSpec
	if err := codec.DecodeFromProviderSpec(ms.ProviderSpec, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// EncodeProviderStatus encodes a libvirt provider
// status as a runtime.RawExtension for inclusion in a MachineStatus
// object.
func EncodeProviderStatus(codec codec, status *v1alpha1.BareMetalMachineProviderStatus) (*runtime.RawExtension, error) {
	return codec.EncodeProviderStatus(status)
}

// ProviderStatusFromMachine deserializes a libvirt provider status
// from a machine object.
func ProviderStatusFromMachine(codec codec, machine *clusterv1.Machine) (*v1alpha1.BareMetalMachineProviderStatus, error) {
	status := &v1alpha1.BareMetalMachineProviderStatus{}
	var err error
	if machine.Status.ProviderStatus != nil {
		err = codec.DecodeProviderStatus(machine.Status.ProviderStatus, status)
	}

	return status, err
}

func (a *Actuator) executeAction(action string, machine *clusterv1.Machine) (bool, error) {
	machineProviderSpec, err := ProviderSpecMachine(a.codec, &machine.Spec)
	if err != nil {
		return false, err
	}
	return utils.RunProvisionCommand(machineProviderSpec.FencingConfig, action)
}

func (a *Actuator) machineproviderconfig(providerSpec clusterv1.ProviderSpec) (*v1alpha1.BareMetalMachineProviderSpec, error) {
	var config v1alpha1.BareMetalMachineProviderSpec
	err := a.codec.DecodeFromProviderSpec(providerSpec, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (a *Actuator) handleMachineError(machine *clusterv1.Machine, err *errors.MachineError, action string) error {
	reason := err.Reason
	message := err.Message
	machine.Status.ErrorReason = &reason
	machine.Status.ErrorMessage = &message
	a.clusterclient.ClusterV1alpha1().Machines(machine.Namespace).UpdateStatus(machine)

	a.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+action, "%v", err.Reason)

	glog.Errorf("Machine error: %v", err.Message)
	return err
}

// updateStatus updates a machine object's status.
func (a *Actuator) updateStatus(machine *clusterv1.Machine) error {
	machineProviderSpec, err := ProviderSpecMachine(a.codec, &machine.Spec)
	if err != nil {
		return err
	}
	node, err := a.kubeclient.CoreV1().Nodes().Get(machineProviderSpec.NodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	glog.Infof("Updating status for %s", machine.Name)
	status, err := ProviderStatusFromMachine(a.codec, machine)
	if err != nil {
		glog.Error("Unable to get provider status from machine: %v", err)
		return err
	}

	uuid := node.Status.NodeInfo.SystemUUID
	state, err := a.executeAction(actionStatus, machine)
	if err != nil {
		return err
	}
	stateString := "ON"
	if !state {
		stateString = "OFF"
	}

	status.InstanceUUID = &uuid
	status.InstanceState = &stateString

	if err := a.applyMachineStatus(machine, status); err != nil {
		glog.Errorf("Unable to apply machine status: %v", err)
		return err
	}

	return nil
}

func (a *Actuator) applyMachineStatus(machine *clusterv1.Machine, status *v1alpha1.BareMetalMachineProviderStatus) error {
	// Encode the new status as a raw extension.
	rawStatus, err := EncodeProviderStatus(a.codec, status)
	if err != nil {
		return err
	}

	machineCopy := machine.DeepCopy()
	machineCopy.Status.ProviderStatus = rawStatus

	if equality.Semantic.DeepEqual(machine.Status, machineCopy.Status) {
		glog.V(4).Infof("Machine %s status is unchanged", machine.Name)
		return nil
	}

	now := metav1.Now()
	machineCopy.Status.LastUpdated = &now
	_, err = a.clusterclient.ClusterV1alpha1().Machines(machineCopy.Namespace).UpdateStatus(machineCopy)
	return err
}
