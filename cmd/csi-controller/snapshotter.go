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
	"os"
	"os/signal"

	"google.golang.org/grpc"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/kubernetes-csi/external-snapshotter/pkg/controller"
	"github.com/kubernetes-csi/external-snapshotter/pkg/snapshotter"

	snapshotscheme "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned/scheme"
	informers "github.com/kubernetes-csi/external-snapshotter/pkg/client/informers/externalversions"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	coreinformers "k8s.io/client-go/informers"
)

func Snapshotter(cc *ControllerClient) (func(ctx context.Context), error) {
	// Check if the driver supports Snapshots
	if !cc.ControllerCapabilites[csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT] {
		klog.Infof("Driver %s does not support snapshots", cc.DriverName)
		return nil, nil
	}

	klog.Info("Driver %s supports snapshots", cc.DriverName)

	// initialize
	factory := informers.NewSharedInformerFactory(snapClient, args.Resync)
	coreFactory := coreinformers.NewSharedInformerFactory(cc.KubernetesClientSet, args.Resync)

	// Create CRD resource
	aeclientset, err := apiextensionsclient.NewForConfig(cc.RestConfig)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// initialize CRD resource if it does not exist
	err = CreateCRD(aeclientset)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// Add Snapshot types to the defualt Kubernetes so events can be logged for them
	snapshotscheme.AddToScheme(scheme.Scheme)

	if len(args.SnapshotterNamePrefix) == 0 {
		klog.Error("Snapshot name prefix cannot be of length 0")
		os.Exit(1)
	}

	klog.V(2).Infof(
		"Start NewCSISnapshotController with snapshotter [%s] connectionTimeout [%+v] "+
			"csiAddress [%s] createSnapshotContentRetryCount [%d] createSnapshotContentInterval [%+v] "+
			"resyncPeriod [%+v] snapshotNamePrefix [%s] snapshotNameUUIDLength [%d]",
		cc.DriverName,
		args.Timeout,
		args.CsiAddress,
		args.SnapshotterSnapshotContentRetryCount,
		args.SnapshotterSnapshotContentInterval,
		args.Resync,
		args.SnapshotterNamePrefix,
		args.SnapshotterNameUUIDLength,
	)

	snapShotter := snapshotter.NewSnapshotter(cc.CsiConn)
	ctrl := controller.NewCSISnapshotController(
		snapClient,
		cc.KubernetesClientSet,
		cc.DriverName,
		factory.Snapshot().V1alpha1().VolumeSnapshots(),
		factory.Snapshot().V1alpha1().VolumeSnapshotContents(),
		factory.Snapshot().V1alpha1().VolumeSnapshotClasses(),
		coreFactory.Core().V1().PersistentVolumeClaims(),
		args.SnapshotterSnapshotContentRetryCount,
		args.SnapshotterSnapshotContentInterval,
		snapShotter,
		args.Timeout,
		args.Resync,
		args.SnapshotterNamePrefix,
		args.SnapshotterNameUUIDLength,
	)

	run := func(context.Context) {
		// run...
		stopCh := make(chan struct{})
		factory.Start(stopCh)
		coreFactory.Start(stopCh)
		go ctrl.Run(threads, stopCh)
	}

	return run, nil
}
