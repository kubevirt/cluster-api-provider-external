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
	"time"

	"github.com/golang/glog"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterclient "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/errors"

	"kubevirt.io/cluster-api-provider-external/pkg/external/machinesetup"
	"kubevirt.io/cluster-api-provider-external/pkg/apis/providerconfig/v1alpha1"
)

const (
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

type ExternalClientMachineSetupConfigGetter interface {
	GetMachineSetupConfig() (machinesetup.MachineSetupConfig, error)
}

type ExternalClient struct {
	providerConfigCodec      *v1alpha1.ExternalProviderConfigCodec
	clusterclient            clusterclient.Interface
	kubeclient               kubernetes.Interface
	machineSetupConfigGetter ExternalClientMachineSetupConfigGetter
	eventRecorder            record.EventRecorder
}

type MachineActuatorParams struct {
	clusterclient            clusterclient.Interface
	kubeclient               kubernetes.Interface
	MachineSetupConfigGetter ExternalClientMachineSetupConfigGetter
	EventRecorder            record.EventRecorder
}

func NewMachineActuator(kubeclient kubernetes.Interface, clusterclient clusterclient.Interface, machineSetupConfigPath string) (*ExternalClient, error) {
	codec, err := v1alpha1.NewCodec()
	if err != nil {
		return nil, err
	}

	machineSetupConfigGetter, err := machinesetup.NewConfigWatch(machineSetupConfigPath)
	if err != nil {
		return nil, err
	}

	glog.V(2).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events(metav1.NamespaceAll)})

	return &ExternalClient{
		providerConfigCodec:      codec,
		kubeclient:               kubeclient,
		clusterclient:            clusterclient,
		machineSetupConfigGetter: machineSetupConfigGetter,
		eventRecorder:            eventBroadcaster.NewRecorder(kubescheme.Scheme, corev1.EventSource{Component: "machine-actuator"}),
	}, nil
}

// Create actuator action powers on the machine
func (c *ExternalClient) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	// TODO: add some logic that will avoid running fencing command on the macine
	// that already has desired state
	glog.Infof("Power-on machine %s", machine.Name)
	if _, err := c.executeAction(createEventAction, cluster, machine, false); err != nil {
		glog.Errorf("Could not fence machine %s: %v", machine.Name, err)
		return c.handleMachineError(machine, errors.CreateMachine(
			"error power-on instance: %v", err), createEventAction)
	}

	c.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Created", "Power-on Machine %s", machine.Name)
	glog.Infof("Machine %s fencing operation succeeded", machine.Name)
	return nil
}

// Delete actuator action powers off the machine
func (c *ExternalClient) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	// TODO: add some logic that will avoid running fencing command on the macine
	// that already has desired state

	glog.Infof("Power-off machine %s", machine.Name)
	if _, err := c.executeAction(deleteEventAction, cluster, machine, true); err != nil {
		return c.handleMachineError(machine, errors.DeleteMachine(
			"error power-off instance: %v", err), deleteEventAction)
	}

	c.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Deleted", "Power-off Machine %s", machine.Name)
	return nil
}

// Update does not run any code
func (c *ExternalClient) Update(cluster *clusterv1.Cluster, goalMachine *clusterv1.Machine) error {
	glog.Infof("NOT IMPLEMENTED: update machine %s", goalMachine.Name)
	return nil
}

// Exists returns true, if machine is power-on
func (c *ExternalClient) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	glog.Infof("Checking if machine %s is power-on", machine.Name)
	if _, err := c.executeAction(checkEventAction, cluster, machine, true); err != nil {
		// TODO: we need to get output from the job
		glog.Infof("Machine %s not found: %v", machine.ObjectMeta.Name, err)
		return false, err
	}

	glog.Infof("Machine %s has status power-on", machine.Name)
	return true, nil
}

func (c *ExternalClient) executeAction(command string, cluster *clusterv1.Cluster, machine *clusterv1.Machine, doWait bool) (*string, error) {
	machineConfig, err := c.machineproviderconfig(machine.Spec.ProviderConfig)
	zone := machineConfig.Zone
	if err != nil {
		return nil, c.handleMachineError(machine,
			errors.InvalidMachineConfiguration("Cannot unmarshal machine's providerConfig field: %v", err), noEventAction)
	}

	clusterConfig, err := c.providerConfigCodec.ClusterProviderFromProviderConfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return nil, c.handleMachineError(machine,
			errors.InvalidMachineConfiguration("Cannot unmarshal cluster's providerConfig field: %v", err), noEventAction)
	}

	err, primitives := c.chooseCRUDConfig(clusterConfig, machine)
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
		j, err := c.kubeclient.BatchV1().Jobs(zone).Create(job)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			//util.JsonLogObject("Bad Job", job)
			return false, err
		}
		job = j
		glog.Infof("Job %v running for %s.", job.Name, machine.ObjectMeta.Name)
		return true, nil
	})

	if err != nil {
		return nil, err
	}

	if doWait {
		if err := c.waitForJob(job.Name, job.Namespace, -1); err != nil {
			glog.Errorf("Job %v error: %v", job.Name, err)
			return nil, err
		}
	}

	glog.Infof("Job %v complete", job.Name)
	return &job.Name, nil
}

func (c *ExternalClient) waitForJob(jobName string, namespace string, retries int) error {

	job, err := c.kubeclient.BatchV1().Jobs(namespace).Get(jobName, metav1.GetOptions{})
	glog.Infof("Waiting %d times for job %v: %v", retries, job.Name, err)

	for lpc := 0; lpc < retries || retries < 0; lpc++ {
		if err != nil {
			return err
		}
		// logrus.Infof("Job %v/%v: %v", job.Name, job.ObjectMeta.ResourceVersion, job.Status)
		if len(job.Status.Conditions) > 0 {
			for _, condition := range job.Status.Conditions {

				if condition.Type == batchv1.JobFailed {
					return fmt.Errorf("Job %v failed: %v", job.Name, condition.Message)

				} else if condition.Type == batchv1.JobComplete {
					if job.Status.Succeeded > 0 {
						return nil
					} else {
						return fmt.Errorf("Job %v failed: %v", job.Name, condition.Message)
					}
				}
			}
		}
		time.Sleep(5 * time.Second)

		options := metav1.GetOptions{ResourceVersion: job.ObjectMeta.ResourceVersion}
		job, err = c.kubeclient.BatchV1().Jobs(namespace).Get(job.ObjectMeta.Name, options)
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

func (c *ExternalClient) chooseCRUDConfig(clusterConfig *v1alpha1.ExternalClusterProviderConfig, machine *clusterv1.Machine) (error, *v1alpha1.CRUDConfig) {
	labelThreshold := -1
	machineConfig, err := c.machineproviderconfig(machine.Spec.ProviderConfig)
	if err != nil {
		glog.Infof("Could not unpack machine provider config for %v: %v", machine.ObjectMeta.Name, err)
		return err, nil
	}

	// Prefer primitives defined as part of the Machine object over those
	// defined in the the Cluster
	chosen := machineConfig.CRUDPrimitives
	if chosen != nil {
		glog.Infof("Using inline %v primitives for machine %v", chosen.ObjectMeta.Name, machine.ObjectMeta.Name)
		return nil, chosen
	}

	// Next try the cluster config
	for _, cfg := range clusterConfig.CRUDPrimitives {
		if len(cfg.NodeSelector) > labelThreshold {
			err, nodes := c.ListNodes(cfg.NodeSelector)
			if err == nil && nodeInList(machine.ObjectMeta.Name, nodes) {
				chosen = &cfg
				labelThreshold = len(cfg.NodeSelector)
			}
		}
	}

	if chosen != nil {
		glog.Infof("Chose %v primitives for machine %v from list", chosen.ObjectMeta.Name, machine.ObjectMeta.Name)
		return nil, chosen
	}

	// Now try machine templates
	configParams := &machinesetup.ConfigParams{
		OS:       machineConfig.OS,
		Roles:    machineConfig.Roles,
		Versions: machine.Spec.Versions,
	}
	machineSetupConfigs, err := c.machineSetupConfigGetter.GetMachineSetupConfig()
	if err != nil {
		glog.Infof("No machine setup config: %v", err)
		return err, nil
	}

	meta, err := machineSetupConfigs.GetMetadata(configParams)
	if err != nil {
		glog.Infof("No matching machine setup: %v", err)
	} else {
		chosen = meta.CRUDPrimitives
	}

	if chosen != nil {
		glog.Infof("Chose %v primitives for machine %v matching: %v", chosen.ObjectMeta.Name, machine.ObjectMeta.Name, configParams)
		return nil, chosen
	}

	return fmt.Errorf("No valid config for %v", machine.ObjectMeta.Name), nil
}

func (c *ExternalClient) ListNodes(selector map[string]string) (error, []corev1.Node) {

	labelSelector := labels.SelectorFromSet(selector).String()
	listOptions := metav1.ListOptions{
		LabelSelector:        labelSelector,
		IncludeUninitialized: false,
		// 	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" protobuf:"varint,5,opt,name=timeoutSeconds"`
		// 	Limit int64 `json:"limit,omitempty" protobuf:"varint,7,opt,name=limit"`
	}

	nodes, err := c.kubeclient.CoreV1().Nodes().List(listOptions)

	return err, nodes.Items
}

func (c *ExternalClient) machineproviderconfig(providerConfig clusterv1.ProviderConfig) (*v1alpha1.ExternalMachineProviderConfig, error) {
	var config v1alpha1.ExternalMachineProviderConfig
	err := c.providerConfigCodec.DecodeFromProviderConfig(providerConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// If the ExternalClient has a client for updating Machine objects, this will set
// the appropriate reason/message on the Machine.Status. If not, such as during
// cluster installation, it will operate as a no-op. It also returns the
// original error for convenience, so callers can do "return handleMachineError(...)".
func (c *ExternalClient) handleMachineError(machine *clusterv1.Machine, err *errors.MachineError, eventAction string) error {
	if c.clusterclient != nil {
		reason := err.Reason
		message := err.Message
		machine.Status.ErrorReason = &reason
		machine.Status.ErrorMessage = &message
		c.clusterclient.ClusterV1alpha1().Machines(machine.Namespace).UpdateStatus(machine)
	}

	if eventAction != noEventAction {
		c.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err.Reason)
	}

	glog.Errorf("Machine error: %v", err.Message)
	return err
}
