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

// The MachineRole indicates the purpose of the Machine, and will determine
// what software and configuration will be used when provisioning and managing
// the Machine. A single Machine may have more than one role, and the list and
// definitions of supported roles is expected to evolve over time.
//
// Currently, only two roles are supported: Master and Node. In the future, we
// expect user needs to drive the evolution and granularity of these roles,
// with new additions accommodating common cluster patterns, like dedicated
// etcd Machines.
//
//                 +-----------------------+------------------------+
//                 | Master present        | Master absent          |
// +---------------+-----------------------+------------------------|
// | Node present: | Install control plane | Join the cluster as    |
// |               | and be schedulable    | just a node            |
// |---------------+-----------------------+------------------------|
// | Node absent:  | Install control plane | Invalid configuration  |
// |               | and be unschedulable  |                        |
// +---------------+-----------------------+------------------------+

type MachineRole string

const (
	MasterRole MachineRole = "Master"
	NodeRole   MachineRole = "Node"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BareMetalMachineProviderConfig provides machine configuration struct
type BareMetalMachineProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	// FencingConfig specify machine power management configuration
	FencingConfig *FencingConfig `json:"fencingConfig"`
	// Label give possibility to map between machine to specific configuration under configMap
	Label string `json:"label,omitempty"`
	// Roles specify which role will server machine under the cluster
	Roles []MachineRole `json:"roles,omitempty"`
}

// FencingConfig container information relating to bare metal power management configuration
type FencingConfig struct {
	AgentType    string            `json:"agentType"`
	AgentAddress string            `json:"agentAddress"`
	AgentOptions map[string]string `json:"agentOptions,omitempty"`
	AgentSecret  *corev1.Secret    `json:"agentSecret"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BareMetalMachineProviderConfigList contains a list of BareMetalMachineProviderConfig
type BareMetalMachineProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalMachineProviderConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalMachineProviderConfig{}, &BareMetalMachineProviderConfigList{})
}
