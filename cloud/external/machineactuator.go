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

package external

import (
	//	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/client-go/kubernetes"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/kubevirt/cluster-api-provider-external/cloud/external/machinesetup"
	providerconfigv1 "github.com/kubevirt/cluster-api-provider-external/cloud/external/providerconfig/v1alpha1"

	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/errors"
	//	"sigs.k8s.io/cluster-api/pkg/util"
)

const (
	BootstrapLabelKey = "bootstrap"

	ProjectAnnotationKey = "ext-project"
	ZoneAnnotationKey    = "ext-zone"
	NameAnnotationKey    = "ext-name"

	// This file is a yaml that will be used to create the machine-setup configmap on the machine controller.
	// It contains the supported machine configurations along with the startup scripts and OS image paths that correspond to each supported configuration.
	MachineSetupConfigsFilename = "machine_setup_configs.yaml"
	ProviderName                = "external"
)

const (
	checkEventAction  = "Status"
	createEventAction = "Create"
	deleteEventAction = "Delete"
	rebootEventAction = "Reboot"
	noEventAction     = ""
)

func init() {
	actuator, err := NewMachineActuator(MachineActuatorParams{})
	if err != nil {
		glog.Fatalf("Error creating cluster provisioner for 'external' : %v", err)
	}
	clustercommon.RegisterClusterProvisioner(ProviderName, actuator)
}

type ExtClientMachineSetupConfigGetter interface {
	GetMachineSetupConfig() (machinesetup.MachineSetupConfig, error)
}

type Instance struct {
	Name   string
	Labels map[string]string
}

type ExtClient struct {
	providerConfigCodec      *providerconfigv1.ExtProviderConfigCodec
	scheme                   *runtime.Scheme
	v1Alpha1Client           client.ClusterV1alpha1Interface
	jobsClient               batchv1client.BatchV1Interface
	coreClient               corev1client.CoreV1Interface
	machineSetupConfigGetter ExtClientMachineSetupConfigGetter
	eventRecorder            record.EventRecorder
}

type MachineActuatorParams struct {
	V1Alpha1Client           client.ClusterV1alpha1Interface
	ClientSet                *kubernetes.Clientset
	MachineSetupConfigGetter ExtClientMachineSetupConfigGetter
	EventRecorder            record.EventRecorder
}

func NewMachineActuator(params MachineActuatorParams) (*ExtClient, error) {
	scheme, err := providerconfigv1.NewScheme()
	if err != nil {
		return nil, err
	}

	codec, err := providerconfigv1.NewCodec()
	if err != nil {
		return nil, err
	}

	return &ExtClient{
		providerConfigCodec:      codec,
		scheme:                   scheme,
		jobsClient:               params.ClientSet.BatchV1(),
		coreClient:               params.ClientSet.CoreV1(),
		v1Alpha1Client:           params.V1Alpha1Client,
		machineSetupConfigGetter: params.MachineSetupConfigGetter,
		eventRecorder:            params.EventRecorder,
	}, nil
}

func (ext *ExtClient) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	instance, err := ext.instanceIfExists(cluster, machine)
	if err != nil {
		return err
	}

	if instance == nil {
		if _, err := ext.executeAction(createEventAction, cluster, machine, false); err != nil {
			return ext.handleMachineError(machine, errors.CreateMachine(
				"error creating instance: %v", err), createEventAction)
		}

		ext.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Created", "Created Machine %v", machine.ObjectMeta.Name)
		// If we have a v1Alpha1Client, then annotate the machine so that we
		// remember exactly what VM we created for it.
		if ext.v1Alpha1Client != nil {
			return ext.updateAnnotations(cluster, machine)
		}
	} else {
		glog.Infof("Skipped creating a VM that already exists.\n")
	}

	return nil
}

func (ext *ExtClient) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	name := machine.ObjectMeta.Name

	// var name string
	// if machine.ObjectMeta.Annotations != nil {
	// 	name = machine.ObjectMeta.Annotations[NameAnnotationKey]
	// }

	instance, err := ext.instanceIfExists(cluster, machine)
	if err != nil {
		return err
	}

	if instance == nil {
		glog.Infof("Skipped deleting a machine that is already deleted.\n")
		return nil
	}

	if _, err := ext.executeAction(deleteEventAction, cluster, machine, true); err != nil {
		return ext.handleMachineError(machine, errors.DeleteMachine(
			"error deleting instance: %v", err), deleteEventAction)
	}

	ext.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Deleted", "Deleted Machine %v", name)

	return err
}

func (ext *ExtClient) PostCreate(cluster *clusterv1.Cluster) error {
	// Ever called?
	return nil
}

func (ext *ExtClient) PostDelete(cluster *clusterv1.Cluster) error {
	// Ever called?
	return nil
}

func (ext *ExtClient) Update(cluster *clusterv1.Cluster, goalMachine *clusterv1.Machine) error {
	// Before updating, do some basic validation of the object first.
	goalConfig, err := ext.machineproviderconfig(goalMachine.Spec.ProviderConfig)
	if err != nil {
		return ext.handleMachineError(goalMachine,
			errors.InvalidMachineConfiguration("Cannot unmarshal machine's providerConfig field: %v", err), noEventAction)
	}
	if verr := ext.validateMachine(goalMachine, goalConfig); verr != nil {
		return ext.handleMachineError(goalMachine, verr, noEventAction)
	}

	status, err := ext.instanceStatus(goalMachine)
	if err != nil {
		return err
	}

	currentMachine := (*clusterv1.Machine)(status)
	if currentMachine == nil {
		instance, err := ext.instanceIfExists(cluster, goalMachine)
		if err != nil {
			return err
		}
		if instance != nil && instance.Labels[BootstrapLabelKey] != "" {
			glog.Infof("Populating current state for bootstrap machine %v", goalMachine.ObjectMeta.Name)
			return ext.updateAnnotations(cluster, goalMachine)
		} else {
			return fmt.Errorf("Cannot retrieve current state to update machine %v", goalMachine.ObjectMeta.Name)
		}
	}

	currentConfig, err := ext.machineproviderconfig(currentMachine.Spec.ProviderConfig)
	if err != nil {
		return ext.handleMachineError(currentMachine, errors.InvalidMachineConfiguration(
			"Cannot unmarshal machine's providerConfig field: %v", err), noEventAction)
	}

	if !ext.requiresUpdate(currentMachine, goalMachine) {
		return nil
	}

	if isMaster(currentConfig.Roles) {
		// glog.Infof("Doing an in-place upgrade for master.\n")
		// err = gce.updateMasterInplace(cluster, currentMachine, goalMachine)
		// if err != nil {
		glog.Errorf("In-place master update failed: %v", err)
		// }
	} else {
		glog.Infof("re-creating machine %s for update.", currentMachine.ObjectMeta.Name)
		err = ext.Delete(cluster, currentMachine)
		if err != nil {
			glog.Errorf("delete machine %s for update failed: %v", currentMachine.ObjectMeta.Name, err)
		} else {
			err = ext.Create(cluster, goalMachine)
			if err != nil {
				glog.Errorf("create machine %s for update failed: %v", goalMachine.ObjectMeta.Name, err)
			}
		}
	}

	if err != nil {
		return err
	}
	return ext.updateInstanceStatus(goalMachine)
}

func (ext *ExtClient) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	i, err := ext.instanceIfExists(cluster, machine)
	if err != nil {
		return false, err
	}
	return (i != nil), err
}

func (ext *ExtClient) executeAction(command string, cluster *clusterv1.Cluster, machine *clusterv1.Machine, doWait bool) (*string, error) {
	machineConfig, err := ext.machineproviderconfig(machine.Spec.ProviderConfig)
	zone := machineConfig.Zone
	if err != nil {
		return nil, ext.handleMachineError(machine,
			errors.InvalidMachineConfiguration("Cannot unmarshal machine's providerConfig field: %v", err), noEventAction)
	}

	clusterConfig, err := ext.providerConfigCodec.ClusterProviderFromProviderConfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return nil, ext.handleMachineError(machine,
			errors.InvalidMachineConfiguration("Cannot unmarshal cluster's providerConfig field: %v", err), noEventAction)
	}

	err, primitives := ext.chooseCRUDConfig(clusterConfig, machine)
	if err != nil {
		return nil, err
	}

	err, job := createCrudJob(command, machine, primitives)
	if err != nil {
		return nil, err
	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

	err = wait.ExponentialBackoff(backoff, func() (bool, error) {
		j, err := ext.jobsClient.Jobs(zone).Create(job)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			//util.JsonLogObject("Bad Job", job)
			return false, err
		}
		job = j
		return true, nil
	})

	if err != nil {
		return nil, err
	}

	if doWait {
		if err := ext.waitForJob(job.Name, job.Namespace, -1); err != nil {
			return nil, err
		}
	}

	return &job.Name, nil
}

func (ext *ExtClient) waitForJob(jobName string, namespace string, retries int) error {

	job, err := ext.jobsClient.Jobs(namespace).Get(jobName, metav1.GetOptions{})

	for lpc := 0; lpc < retries || retries < 0; lpc++ {
		if err != nil {
			return err
		}
		if len(job.Status.Conditions) > 0 {
			for _, condition := range job.Status.Conditions {

				if condition.Type == batchv1.JobComplete {
					if job.Status.Succeeded > 0 {
						return nil
					} else {
						return fmt.Errorf("Job %v failed: %v", job.Name, condition.Message)
					}
				}
			}
		}

		options := metav1.GetOptions{ResourceVersion: job.ObjectMeta.ResourceVersion}
		job, err = ext.jobsClient.Jobs(namespace).Get(job.ObjectMeta.Name, options)
	}

	return fmt.Errorf("Job %v in progress", job.Name)
}

// JobCondition describes current state of a job.
// type JobCondition struct {
// 	// Type of job condition, Complete or Failed.
// 	Type JobConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=JobConditionType"`
// 	// Status of the condition, one of True, False, Unknown.
// 	Status v1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
// 	// Last time the condition was checked.
// 	// +optional
// 	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty" protobuf:"bytes,3,opt,name=lastProbeTime"`
// 	// Last time the condition transit from one status to another.
// 	// +optional
// 	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
// 	// (brief) reason for the condition's last transition.
// 	// +optional
// 	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
// 	// Human readable message indicating details about last transition.
// 	// +optional
// 	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
// }

func (ext *ExtClient) chooseCRUDConfig(clusterConfig *providerconfigv1.ExtClusterProviderConfig, machine *clusterv1.Machine) (error, *providerconfigv1.CRUDConfig) {
	labelThreshold := -1
	machineConfig, err := ext.machineproviderconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return err, nil
	}

	// Prefer primitives defined as part of the Machine object over those
	// defined in the the Cluster
	chosen := machineConfig.CRUDPrimitives

	if chosen == nil {
		for _, cfg := range clusterConfig.CRUDPrimitives {
			if len(cfg.NodeSelector) > labelThreshold {
				err, nodes := ext.ListNodes(cfg.NodeSelector)
				if err == nil && nodeInList(machine.ObjectMeta.Name, nodes) {
					chosen = &cfg
					labelThreshold = len(cfg.NodeSelector)
				}
			}
		}
	}

	if chosen != nil {
		logrus.Infof("Chose %v for %v primitives", chosen.ObjectMeta.Name, machine.ObjectMeta.Name)
		return nil, chosen
	}

	return fmt.Errorf("No valid config for %v", machine.ObjectMeta.Name), nil
}

func isMaster(roles []providerconfigv1.MachineRole) bool {
	for _, r := range roles {
		if r == providerconfigv1.MasterRole {
			return true
		}
	}
	return false
}

func (ext *ExtClient) updateAnnotations(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	machineConfig, err := ext.machineproviderconfig(machine.Spec.ProviderConfig)
	name := machine.ObjectMeta.Name
	zone := machineConfig.Zone
	if err != nil {
		return ext.handleMachineError(machine,
			errors.InvalidMachineConfiguration("Cannot unmarshal machine's providerConfig field: %v", err), noEventAction)
	}

	clusterConfig, err := ext.providerConfigCodec.ClusterProviderFromProviderConfig(cluster.Spec.ProviderConfig)
	project := clusterConfig.Project
	if err != nil {
		return ext.handleMachineError(machine,
			errors.InvalidMachineConfiguration("Cannot unmarshal cluster's providerConfig field: %v", err), noEventAction)
	}

	if machine.ObjectMeta.Annotations == nil {
		machine.ObjectMeta.Annotations = make(map[string]string)
	}
	machine.ObjectMeta.Annotations[ProjectAnnotationKey] = project
	machine.ObjectMeta.Annotations[ZoneAnnotationKey] = zone
	machine.ObjectMeta.Annotations[NameAnnotationKey] = name
	_, err = ext.v1Alpha1Client.Machines(machine.Namespace).Update(machine)
	if err != nil {
		return err
	}
	err = ext.updateInstanceStatus(machine)
	return err
}

// The two machines differ in a way that requires an update
func (ext *ExtClient) requiresUpdate(a *clusterv1.Machine, b *clusterv1.Machine) bool {
	// Do not want status changes. Do want changes that impact machine provisioning
	return !reflect.DeepEqual(a.Spec.ObjectMeta, b.Spec.ObjectMeta) ||
		!reflect.DeepEqual(a.Spec.ProviderConfig, b.Spec.ProviderConfig) ||
		!reflect.DeepEqual(a.Spec.Versions, b.Spec.Versions) ||
		a.ObjectMeta.Name != b.ObjectMeta.Name
}

// Gets the instance represented by the given machine
func (ext *ExtClient) instanceIfExists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (*Instance, error) {
	identifyingMachine := machine

	// Try to use the last saved status locating the machine
	// in case instance details have changed
	status, err := ext.instanceStatus(machine)
	if err != nil {
		return nil, err
	}

	if status != nil {
		identifyingMachine = (*clusterv1.Machine)(status)
	}

	// Get the VM via specified location and name
	if _, err := ext.machineproviderconfig(identifyingMachine.Spec.ProviderConfig); err != nil {
		return nil, err
	}

	// clusterConfig, err := ext.providerConfigCodec.ClusterProviderFromProviderConfig(cluster.Spec.ProviderConfig)
	// if err != nil {
	// 	return nil, err
	// }

	if _, err := ext.executeAction(checkEventAction, cluster, machine, true); err != nil {
		return nil, nil
	}

	return &Instance{Name: identifyingMachine.ObjectMeta.Name}, nil
}

func (ext *ExtClient) ListNodes(selector map[string]string) (error, []corev1.Node) {

	labelSelector := labels.SelectorFromSet(selector).String()
	listOptions := metav1.ListOptions{
		LabelSelector:        labelSelector,
		IncludeUninitialized: false,
		// 	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" protobuf:"varint,5,opt,name=timeoutSeconds"`
		// 	Limit int64 `json:"limit,omitempty" protobuf:"varint,7,opt,name=limit"`
	}

	nodes, err := ext.coreClient.Nodes().List(listOptions)

	return err, nodes.Items
}

func (ext *ExtClient) machineproviderconfig(providerConfig clusterv1.ProviderConfig) (*providerconfigv1.ExtMachineProviderConfig, error) {
	var config providerconfigv1.ExtMachineProviderConfig
	err := ext.providerConfigCodec.DecodeFromProviderConfig(providerConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (ext *ExtClient) validateMachine(machine *clusterv1.Machine, config *providerconfigv1.ExtMachineProviderConfig) *errors.MachineError {
	// if machine.Spec.Versions.Kubelet == "" {
	// 	return errors.InvalidMachineConfiguration("spec.versions.kubelet can't be empty")
	// }
	return nil
}

// If the ExtClient has a client for updating Machine objects, this will set
// the appropriate reason/message on the Machine.Status. If not, such as during
// cluster installation, it will operate as a no-op. It also returns the
// original error for convenience, so callers can do "return handleMachineError(...)".
func (ext *ExtClient) handleMachineError(machine *clusterv1.Machine, err *errors.MachineError, eventAction string) error {
	if ext.v1Alpha1Client != nil {
		reason := err.Reason
		message := err.Message
		machine.Status.ErrorReason = &reason
		machine.Status.ErrorMessage = &message
		ext.v1Alpha1Client.Machines(machine.Namespace).UpdateStatus(machine)
	}

	if eventAction != noEventAction {
		ext.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err.Reason)
	}

	glog.Errorf("Machine error: %v", err.Message)
	return err
}
