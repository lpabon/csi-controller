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
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/external-resizer/pkg/controller"
	"github.com/kubernetes-csi/external-resizer/pkg/resizer"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/klog"
)

func Resizer(cc *ControllerClient) (func(ctx context.Context, stopCh <-chan struct{}), error) {

	if !cc.ControllerCapabilites[csi.ControllerServiceCapability_RPC_EXPAND_VOLUME] {
		klog.Infof("Driver %s does not support resize", cc.DriverName)
		return nil, nil
	}
	klog.Infof("Driver %s supports resize", cc.DriverName)

	args := cc.ControllerArgs
	informerFactory := informers.NewSharedInformerFactory(cc.KubernetesClientSet, args.Resync)
	csiResizer, err := resizer.NewResizer(args.CsiAddress, args.Timeout, cc.KubernetesClientSet, informerFactory)
	if err != nil {
		klog.Fatal("Failed to start resizer: %v", err)
	}

	resizerName := cc.DriverName
	rc := controller.NewResizeController(resizerName, csiResizer, cc.KubernetesClientSet, args.Resync, informerFactory)
	run := func(ctx context.Context, stopCh <-chan struct{}) {
		klog.Info("Starting resizer controller...")
		informerFactory.Start(wait.NeverStop)
		rc.Run(args.WorkerThreads, ctx)
	}

	return run, nil
}
