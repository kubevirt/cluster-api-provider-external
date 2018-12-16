/*
Copyright 2017 The Kubernetes Authors.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ServiceAccountAnsibleJob = "ansible-job"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BareMetalMachineProviderConfig provides machine configuration struct
type BareMetalMachineProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	// FencingConfig specify machine power management configuration
	FencingConfig *FencingConfig `json:"fencingConfig"`
}

// FencingConfig container information relating to bare metal power management configuration
type FencingConfig struct {
	// AgentType is the type of the fence device
	AgentType string `json:"agentType"`

	// AgentAddress is the address of the fence device
	AgentAddress string `json:"agentAddress"`

	// AgentOptions is additional options that you can send to the fence agent
	AgentOptions map[string]string `json:"agentOptions,omitempty"`

	// AgentSecret container username and password of the fence agent
	AgentSecret *corev1.Secret `json:"agentSecret"`
}

// BareMetalClusterProviderConfig is the type that will be embedded in a Cluster.Spec.ProviderConfig field.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BareMetalClusterProviderConfig struct {
	metav1.TypeMeta `json:",inline"`
}

// BareMetalMachineProviderStatus is the type that will be embedded in a Machine.Status.ProviderStatus field.
// It contains bare-metal specific status information.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BareMetalMachineProviderStatus struct {
	metav1.TypeMeta `json:",inline"`

	// InstanceUUID is the instance UUID of the bare metal instance for this machine
	InstanceUUID *string `json:"instanceUUID"`

	// InstanceState is the state of the bare metal instance for this machine
	InstanceState *string `json:"instanceState"`

	// Conditions is a set of conditions associated with the Machine to indicate
	// errors or other status
	Conditions []BareMetalMachineProviderCondition `json:"conditions"`
}

// BareMetalMachineProviderConditionType is a valid value for BareMetalMachineProviderCondition.Type
type BareMetalMachineProviderConditionType string

// Valid conditions for an Bare Metal machine instance
const (
	// MachineCreated indicates whether the machine has been created or not. If not,
	// it should include a reason and message for the failure.
	MachineCreated BareMetalMachineProviderConditionType = "MachineCreated"
)

// BareMetalMachineProviderCondition is a condition in a BareMetalMachineProviderStatus
type BareMetalMachineProviderCondition struct {
	// Type is the type of the condition.
	Type BareMetalMachineProviderConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason"`
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message"`
}

// BareMetalClusterProviderStatus is the type that will be embedded in a Cluster.Status.ProviderStatus field.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BareMetalClusterProviderStatus struct {
	metav1.TypeMeta `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BareMetalMachineProviderConfigList contains a list of BareMetalMachineProviderConfig
type BareMetalMachineProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalMachineProviderConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalMachineProviderConfig{}, &BareMetalMachineProviderConfigList{}, &BareMetalMachineProviderStatus{})
}
