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

package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	clusterapis "sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	machineactuator "kubevirt.io/cluster-api-provider-external/pkg/actuators/baremetal/machine"
	"kubevirt.io/cluster-api-provider-external/pkg/apis"
	"kubevirt.io/cluster-api-provider-external/pkg/controller"
)

func main() {
	flag.Set("logtostderr", "true")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	flag.Parse()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		glog.Fatal(err)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		glog.Fatal(err)
	}

	glog.Info("Registering Components")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		glog.Fatal(err)
	}

	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		glog.Fatal(err)
	}

	clusterClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	// Setup machine controller
	machineactuator.MachineActuator, err = machineactuator.NewActuator(kubeClient, clusterClient)
	if err != nil {
		panic(err)
	}
	if err := controller.AddToManager(mgr); err != nil {
		panic(err)
	}

	glog.Info("Starting the manager")

	// Start the Cmd
	glog.Fatal(mgr.Start(signals.SetupSignalHandler()))
}
