/*
Copyright 2019 The Kubernetes Authors.

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
	"context"
	"math/rand"
	"strconv"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	ctrl "github.com/kubernetes-csi/external-provisioner/pkg/controller"
	snapclientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	csitranslationlib "k8s.io/csi-translation-lib"
)

// Provisioner returns a provisioner controller for the leader election runner
func Provisioner(cc *ControllerClient) (RunnerHandler, error) {

	if !cc.ControllerCapabilites[csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME] {
		klog.V(2).Infof("Driver %s does not support provisioning", cc.DriverName)
		return nil, nil
	}
	klog.V(2).Infof("Driver %s supports provisioning", cc.DriverName)

	// snapclientset.NewForConfig creates a new Clientset for VolumesnapshotV1alpha1Client
	args := cc.ControllerArgs
	snapClient, err := snapclientset.NewForConfig(cc.RestConfig)
	if err != nil {
		klog.Fatalf("Failed to create snapshot client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := controllerClient.KubernetesClientSet.Discovery().ServerVersion()
	if err != nil {
		klog.Fatalf("Error getting server version: %v", err)
	}

	// Generate a unique ID for this provisioner
	timeStamp := time.Now().UnixNano() / int64(time.Millisecond)
	identity := strconv.FormatInt(timeStamp, 10) + "-" + strconv.Itoa(rand.Intn(10000)) + "-" + controllerClient.DriverName

	provisionerOptions := []func(*controller.ProvisionController) error{
		controller.LeaderElection(false), // Always disable leader election in provisioner lib. Leader election should be done here in the CSI provisioner level instead.
		controller.FailedProvisionThreshold(0),
		controller.FailedDeleteThreshold(0),
		controller.RateLimiter(workqueue.NewItemExponentialFailureRateLimiter(args.RetryIntervalStart, args.RetryIntervalMax)),
		controller.Threadiness(args.WorkerThreads),
		controller.CreateProvisionedPVLimiter(workqueue.DefaultControllerRateLimiter()),
	}

	supportsMigrationFromInTreePluginName := ""
	if csitranslationlib.IsMigratedCSIDriverByName(controllerClient.DriverName) {
		supportsMigrationFromInTreePluginName, err = csitranslationlib.GetInTreeNameFromCSIName(controllerClient.DriverName)
		if err != nil {
			klog.Fatalf("Failed to get InTree plugin name for migrated CSI plugin %s: %v", controllerClient.DriverName, err)
		}
		klog.V(2).Infof("Supports migration from in-tree plugin: %s", supportsMigrationFromInTreePluginName)
		provisionerOptions = append(provisionerOptions, controller.AdditionalProvisionerNames([]string{supportsMigrationFromInTreePluginName}))
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	csiProvisioner := ctrl.NewCSIProvisioner(
		controllerClient.KubernetesClientSet,
		args.Timeout,
		identity,
		args.ProvisionerVolumeNamePrefix,
		args.ProvisionerVolumeNameUUIDLength,
		cc.CsiConn,
		snapClient,
		controllerClient.DriverName,
		cc.PluginCapabilites,
		cc.ControllerCapabilites,
		supportsMigrationFromInTreePluginName,
		args.ProvisionerStrictTopology)

	provisionController := controller.NewProvisionController(
		controllerClient.KubernetesClientSet,
		controllerClient.DriverName,
		csiProvisioner,
		serverVersion.GitVersion,
		provisionerOptions...,
	)

	run := func(ctx context.Context, stopCh <-chan struct{}) {
		klog.V(2).Info("Starting provisioner controller...")
		provisionController.Run(wait.NeverStop)
	}

	return run, nil
}
