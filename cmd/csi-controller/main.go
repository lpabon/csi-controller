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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/kubernetes-csi/external-attacher/pkg/attacher"
	"github.com/kubernetes-csi/external-attacher/pkg/controller"
	"google.golang.org/grpc"
)

const (

	// Default timeout of short CSI calls like GetPluginInfo
	csiTimeout = time.Second

	leaderElectionTypeLeases     = "leases"
	leaderElectionTypeConfigMaps = "configmaps"
)

// Command line flags
var (
	kubeconfig    = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	resync        = flag.Duration("resync", 10*time.Minute, "Resync interval of the controller.")
	csiAddress    = flag.String("csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	showVersion   = flag.Bool("version", false, "Show version.")
	timeout       = flag.Duration("timeout", 15*time.Second, "Timeout for waiting for attaching or detaching the volume.")
	workerThreads = flag.Uint("worker-threads", 10, "Number of attacher worker threads")

	retryIntervalStart = flag.Duration("retry-interval-start", time.Second, "Initial retry interval of failed create volume or deletion. It doubles with each failure, up to retry-interval-max.")
	retryIntervalMax   = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval of failed create volume or deletion.")

	enableLeaderElection    = flag.Bool("leader-election", false, "Enable leader election.")
	leaderElectionNamespace = flag.String("leader-election-namespace", "", "Namespace where the leader election resource lives. Defaults to the pod namespace if not set.")
)

var (
	version = "unknown"
)

type leaderElection interface {
	Run() error
	WithNamespace(namespace string)
}

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		return
	}
	klog.Infof("Version: %s", version)

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	if *workerThreads == 0 {
		klog.Error("option -worker-threads must be greater than zero")
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	factory := informers.NewSharedInformerFactory(clientset, *resync)
	var handler controller.Handler
	// Connect to CSI.
	csiConn, err := connection.Connect(*csiAddress, connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	err = rpc.ProbeForever(csiConn, *timeout)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// Find driver name.
	ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
	defer cancel()
	csiAttacher, err := rpc.GetDriverName(ctx, csiConn)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
	klog.V(2).Infof("CSI driver name: %q", csiAttacher)

	supportsService, err := supportsPluginControllerService(ctx, csiConn)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
	if !supportsService {
		handler = controller.NewTrivialHandler(clientset)
		klog.V(2).Infof("CSI driver does not support Plugin Controller Service, using trivial handler")
	} else {
		// Find out if the driver supports attach/detach.
		supportsAttach, supportsReadOnly, err := supportsControllerPublish(ctx, csiConn)
		if err != nil {
			klog.Error(err.Error())
			os.Exit(1)
		}
		if supportsAttach {
			pvLister := factory.Core().V1().PersistentVolumes().Lister()
			nodeLister := factory.Core().V1().Nodes().Lister()
			vaLister := factory.Storage().V1beta1().VolumeAttachments().Lister()
			csiNodeLister := factory.Storage().V1beta1().CSINodes().Lister()
			attacher := attacher.NewAttacher(csiConn)
			handler = controller.NewCSIHandler(clientset, csiAttacher, attacher, pvLister, nodeLister, csiNodeLister, vaLister, timeout, supportsReadOnly)
			klog.V(2).Infof("CSI driver supports ControllerPublishUnpublish, using real CSI handler")
		} else {
			handler = controller.NewTrivialHandler(clientset)
			klog.V(2).Infof("CSI driver does not support ControllerPublishUnpublish, using trivial handler")
		}
	}

	ctrl := controller.NewCSIAttachController(
		clientset,
		csiAttacher,
		handler,
		factory.Storage().V1beta1().VolumeAttachments(),
		factory.Core().V1().PersistentVolumes(),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
	)

	run := func(ctx context.Context) {
		stopCh := ctx.Done()
		factory.Start(stopCh)
		ctrl.Run(int(*workerThreads), stopCh)
	}

	if !*enableLeaderElection {
		run(context.TODO())
	} else {
		// Name of config map with leader election lock
		lockName := "external-attacher-leader-" + csiAttacher
		le := leaderelection.NewLeaderElection(clientset, lockName, run)

		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}

		if err := le.Run(); err != nil {
			klog.Fatalf("failed to initialize leader election: %v", err)
		}
	}
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func supportsControllerPublish(ctx context.Context, csiConn *grpc.ClientConn) (supportsControllerPublish bool, supportsPublishReadOnly bool, err error) {
	caps, err := rpc.GetControllerCapabilities(ctx, csiConn)
	if err != nil {
		return false, false, err
	}

	supportsControllerPublish = caps[csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME]
	supportsPublishReadOnly = caps[csi.ControllerServiceCapability_RPC_PUBLISH_READONLY]
	return supportsControllerPublish, supportsPublishReadOnly, nil
}

func supportsPluginControllerService(ctx context.Context, csiConn *grpc.ClientConn) (bool, error) {
	caps, err := rpc.GetPluginCapabilities(ctx, csiConn)
	if err != nil {
		return false, err
	}

	return caps[csi.PluginCapability_Service_CONTROLLER_SERVICE], nil
}
