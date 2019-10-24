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

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/kubernetes-csi/external-attacher/pkg/attacher"
	attacherctl "github.com/kubernetes-csi/external-attacher/pkg/controller"
)

// Attacher returns an attacher controller for the leader election runner
func Attacher(cc *ControllerClient) (RunnerHandler, error) {
	if !cc.ControllerCapabilites[csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME] {
		klog.Infof("Driver %s does not support controller attacher", cc.DriverName)
		return nil, nil
	}
	klog.Infof("Driver %s supports controller attacher", cc.DriverName)

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

		klog.Info("Starting attacher controller...")
		attacherCtrl.Run(args.WorkerThreads, stopCh)
	}

	return run, nil
}
