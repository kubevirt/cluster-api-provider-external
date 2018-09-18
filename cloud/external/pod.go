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
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	v1batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	providerconfigv1 "github.com/kubevirt/cluster-api-provider-external/cloud/external/providerconfig/v1alpha1"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	secretsDir = "/etc/fencing/secrets/"
)

func nodeInList(name string, nodes []v1.Node) bool {
	for _, node := range nodes {
		if node.Name == name {
			return true
		}
	}
	return false
}

func createCrudJob(action string, machine *clusterv1.Machine, method *providerconfigv1.CRUDConfig) (error, *v1batch.Job) {
	// Create a Job with a container for each mechanism

	volumeMap := map[string]v1.Volume{}
	containers := []v1.Container{}

	labels := map[string]string{"foo": "bar"} //req.JobLabels(method.Name)

	container := method.Container.DeepCopy()

	if err, cmd := getContainerCommand(method, action, machine.ObjectMeta.Name); err != nil {
		return fmt.Errorf("Method %s aborted: %v", action, err), nil
	} else if len(cmd) > 0 {
		container.Command = cmd
	}

	if err, env := getContainerEnv(method, machine.ObjectMeta.Name, secretsDir); err != nil {
		return fmt.Errorf("Method %s aborted: %v", action, err), nil
	} else {
		container.Env = env
	}

	logrus.Infof("template: %v", method.Container)
	logrus.Infof("instance: %v", container)

	for _, v := range processSecrets(method, container) {
		if _, ok := volumeMap[v.Name]; !ok {
			volumeMap[v.Name] = v
		}
	}
	// Add the container to the PodSpec
	containers = append(containers, *container)

	volumes := []v1.Volume{}
	for _, v := range volumeMap {
		volumes = append(volumes, v)
	}

	timeout := int64(300) // TODO: Make this configurable
	//	numContainers := int32(len(containers))
	numContainers := int32(1)

	// Parallel Jobs with a fixed completion count
	// - https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	return nil, &v1batch.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%v-%v-job-", machine.ObjectMeta.Name, action),
			// Namespace:    req.Namespace,
			// OwnerReferences: []metav1.OwnerReference{
			// 	*metav1.NewControllerRef(req, schema.GroupVersionKind{
			// 		Group:   providerconfigv1.SchemeGroupVersion.Group,
			// 		Version: providerconfigv1.SchemeGroupVersion.Version,
			// 		Kind:    "FencingRequest",
			// 	}),
			// },
			Labels: labels,
		},
		Spec: v1batch.JobSpec{
			BackoffLimit:          method.Retries,
			Parallelism:           &numContainers,
			Completions:           &numContainers,
			ActiveDeadlineSeconds: &timeout,
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers:    containers,
					RestartPolicy: v1.RestartPolicyOnFailure,
					Volumes:       volumes,
				},
			},
		},
	}
}

func volumeNameMap(r rune) rune {
	switch {
	case r >= 'A' && r <= 'Z':
		return 'a' + (r - 'A')
	case r >= 'a' && r <= 'z':
		return r
	case r >= '0' && r <= '9':
		return r
	default:
		return '-'
	}
}

func processSecrets(method *providerconfigv1.CRUDConfig, c *v1.Container) []v1.Volume {
	volumes := []v1.Volume{}
	for key, s := range method.Secrets {

		// volumeName must contain only a-z, 0-9, and -
		volumeName := strings.Map(volumeNameMap, fmt.Sprintf("secret-%s", key))

		// Create volumes for any sensitive parameters that are stored as k8s secrets
		volumes = append(volumes, v1.Volume{
			Name: volumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: s,
				},
			},
		})

		// Relies on an ENTRYPOINT that looks for SECRETPATH_field=/path/to/file and add: --field=$(cat /path/to/file) to the command line
		c.Env = append(c.Env, v1.EnvVar{
			Name:  fmt.Sprintf("SECRETPATH_%s", key),
			Value: fmt.Sprintf("%s/%s", secretsDir, s),
		})

		// Mount the secrets into the container so they can be easily retrieved
		c.VolumeMounts = append(c.VolumeMounts, v1.VolumeMount{
			Name:      volumeName,
			ReadOnly:  true,
			MountPath: secretsDir + s,
		})
	}
	return volumes
}

func getContainerCommand(m *providerconfigv1.CRUDConfig, primitive string, target string) (error, []string) {
	command := []string{}

	switch primitive {

	case createEventAction:
		command = m.CreateCmd
	case deleteEventAction:
		command = m.DeleteCmd
	case rebootEventAction:
		command = m.RebootCmd
	default:
		return fmt.Errorf("Unknown primitive '%s' requested for '%s'", primitive, target), []string{}
	}

	if m.ArgumentFormat == "env" {

		if len(m.PassTargetAs) == 0 {
			// No other way to pass it in, just append to the existing command
			command = append(command, target)
		}

	} else if m.ArgumentFormat == "cli" {
		for name, value := range m.Config {
			command = append(command, fmt.Sprintf("--%s", name))
			command = append(command, value)
		}

		for _, dc := range m.DynamicConfig {
			command = append(command, fmt.Sprintf("--%s", dc.Field))
			if value, ok := dc.Lookup(target); ok {
				command = append(command, value)
			} else {
				return fmt.Errorf("No value of '%s' found for '%s'", dc.Field, target), []string{}
			}
		}

		if len(m.PassTargetAs) > 0 {
			command = append(command, fmt.Sprintf("--%s", m.PassTargetAs))
		}

		command = append(command, target)

	} else {
		return fmt.Errorf("ArgumentFormat %s not supported", m.ArgumentFormat), []string{}
	}

	logrus.Infof("%s command: %v", m.Container.Name, m.Container.Command)
	return nil, command
}

func getContainerEnv(m *providerconfigv1.CRUDConfig, target string, secretsDir string) (error, []v1.EnvVar) {
	env := []v1.EnvVar{
		{
			Name:  "ARG_FORMAT",
			Value: m.ArgumentFormat,
		},
	}

	for _, val := range m.Container.Env {
		env = append(env, val)
	}

	if m.ArgumentFormat == "cli" {
		return nil, env
	}

	if m.ArgumentFormat == "env" {
		logrus.Infof("Adding env vars")
		for name, value := range m.Config {
			logrus.Infof("Adding %v=%v", name, value)
			env = append(env, v1.EnvVar{
				Name:  name,
				Value: value,
			})
		}

		logrus.Infof("Adding dynamic env vars: %v", m.DynamicConfig)
		for _, dc := range m.DynamicConfig {
			if value, ok := dc.Lookup(target); ok {
				logrus.Infof("Adding %v=%v (dynamic)", dc.Field, value)
				env = append(env, v1.EnvVar{
					Name:  dc.Field,
					Value: value,
				})
			} else {
				logrus.Errorf("not adding %v (dynamic)", dc.Field)
				return fmt.Errorf("No value of '%s' found for '%s'", dc.Field, target), nil
			}
		}

		if len(m.PassTargetAs) > 0 {
			env = append(env, v1.EnvVar{
				Name:  m.PassTargetAs,
				Value: target,
			})
		}

		return nil, env
	}
	return fmt.Errorf("ArgumentFormat %s not supported", m.ArgumentFormat), env
}
