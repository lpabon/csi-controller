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

	"k8s.io/client-go/informers"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	ver "github.com/hashicorp/go-version"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/kubernetes-csi/external-attacher/pkg/attacher"
	attacherctl "github.com/kubernetes-csi/external-attacher/pkg/controller"
)

// Attacher returns an attacher controller for the leader election runner
func Attacher(cc *ControllerClient) (RunnerHandler, error) {

	// Create a Kubernetes version constraint
	kVer, err := ver.NewVersion(cc.KubeVersion)
	if err != nil {
		klog.Fatalf("Failed to determine kubernetes version from %s: %v", cc.KubeVersion, err)
	}
	attacherDetectionConstraint, err := ver.NewConstraint(">= 1.14.0")
	if err != nil {
		klog.Fatalf(err.Error())
	}

	// If we have k8s version < v1.14, we include the attacher.
	// This is because the CSIDriver object was alpha until 1.14+
	// To enable the CSIDriver object support, a feature flag would be needed.
	// Instead, we'll just include the attacher if k8s < 1.14.
	if attacherDetectionConstraint.Check(kVer) {
		if !cc.ControllerCapabilites[csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME] {
			klog.V(2).Infof("Driver %s does not support controller attacher", cc.DriverName)
			return nil, nil
		}
		klog.V(2).Infof("Driver %s supports controller attacher", cc.DriverName)
	} else {
		klog.V(2).Info("Kubernetes 1.13.x detected which requires the attacher process even if the CSI driver does not support it")
	}

	// Attacher ----------------------------------------------------------
	var attacherCtrl *attacherctl.CSIAttachController
	args := cc.ControllerArgs
	factory := informers.NewSharedInformerFactory(controllerClient.KubernetesClientSet, args.Resync)

	// Find out if the driver supports attach/detach.
	supportsReadOnly := cc.ControllerCapabilites[csi.ControllerServiceCapability_RPC_PUBLISH_READONLY]
	pvLister := factory.Core().V1().PersistentVolumes().Lister()
	nodeLister := factory.Core().V1().Nodes().Lister()
	vaLister := factory.Storage().V1beta1().VolumeAttachments().Lister()
	csiNodeLister := factory.Storage().V1beta1().CSINodes().Lister()
	attacher := attacher.NewAttacher(cc.CsiConn)
	handler := attacherctl.NewCSIHandler(
		cc.KubernetesClientSet,
		cc.DriverName,
		attacher,
		pvLister,
		nodeLister,
		csiNodeLister,
		vaLister,
		&args.Timeout,
		supportsReadOnly)

	attacherCtrl = attacherctl.NewCSIAttachController(
		cc.KubernetesClientSet,
		cc.DriverName,
		handler,
		factory.Storage().V1beta1().VolumeAttachments(),
		factory.Core().V1().PersistentVolumes(),
		workqueue.NewItemExponentialFailureRateLimiter(args.RetryIntervalStart, args.RetryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(args.RetryIntervalStart, args.RetryIntervalMax),
	)

	run := func(ctx context.Context, stopCh <-chan struct{}) {
		factory.Start(stopCh)

		klog.V(2).Info("Starting attacher controller...")
		attacherCtrl.Run(args.WorkerThreads, stopCh)
	}

	return run, nil
}
