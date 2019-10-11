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

type CommonOpts struct {
	Kubeconfig              string
	Resync                  time.Duration
	CsiAddress              string
	LeaderElectionNamespace string
	WorkerThreads           int
	Timeout                 time.Duration
}

type AttacherOpts struct {
	AttacherDetachTimeout      time.Duration
	AttacherRetryIntervalStart time.Duration
	AttacherRetryIntervalMax   time.Duration
}

type CliOpts struct {
	CommonOpts
	AttacherOpts
}

type ControllerClient struct {
	ControllerArgs      CliOpts
	KubernetesClientSet *kubernetes.Clientset
}

type leaderElection interface {
	Run() error
	WithNamespace(namespace string)
}

// Command line flags
var (
	controllerClient ControllerClient
	version          = "unknown"

	showVersion = flag.Bool("version", false, "Show version.")
)

func init() {
	c := &controllerClient.ControllerArgs

	// Common
	flag.StringVar(&c.Kubeconfig, "kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	flag.StringVar(&c.CsiAddress, "csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	flag.DurationVar(&c.Resync, "resync", 10*time.Minute, "Resync interval of the controller.")
	flag.StringVar(&c.CsiAddress, "leader-election-namespace", "", "Namespace where the leader election resource lives. Defaults to the pod namespace if not set.")
	flag.IntVar(&c.WorkerThreads, "worker-threads", 10, "Number of attacher worker threads")
	flag.DurationVar(&c.Timeout, "timeout", 15*time.Second, "Timeout for waiting for driver to be ready")

	// Attacher
	flag.DurationVar(&c.AttacherDetachTimeout, "attacher-timeout", 15*time.Second, "Timeout for waiting for attaching or detaching the volume.")
	flag.DurationVar(&c.AttacherRetryIntervalStart, "attacher-retry-interval-start", time.Second, "Initial retry interval of failed create volume or deletion. It doubles with each failure, up to retry-interval-max.")
	flag.DurationVar(&c.AttacherRetryIntervalMax, "attacher-retry-interval-max", 5*time.Minute, "Maximum retry interval of failed create volume or deletion.")
}

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()
	args := &controllerClient.ControllerArgs

	// COMMON ----------------------------------------------------------
	if *showVersion {
		fmt.Println(os.Args[0], version)
		return
	}
	klog.Infof("Version: %s", version)

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(args.Kubeconfig)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	if args.WorkerThreads == 0 {
		klog.Error("option -worker-threads must be greater than zero")
		os.Exit(1)
	}

	controllerClient.KubernetesClientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	factory := informers.NewSharedInformerFactory(controllerClient.KubernetesClientSet, args.Resync)
	// Connect to CSI.
	csiConn, err := connection.Connect(args.CsiAddress, connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	err = rpc.ProbeForever(csiConn, args.Timeout)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// Find driver name.
	ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
	defer cancel()
	driverName, err := rpc.GetDriverName(ctx, csiConn)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
	klog.V(2).Infof("CSI driver name: %q", driverName)

	// ControllerService ----------------------------------------------------------
	supportsService, err := supportsPluginControllerService(ctx, csiConn)
	if err != nil {
		klog.Error(err.Error())
	}

	if !supportsService {
		klog.Error("CSI driver does not support Plugin Controller Service")
		os.Exit(1)
	}

	// Attacher ----------------------------------------------------------
	var attacherCtrl *controller.CSIAttachController

	// Find out if the driver supports attach/detach.
	supportsAttach, supportsReadOnly, err := supportsControllerPublish(ctx, csiConn)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
	if supportsAttach {
		klog.V(2).Infof("CSI driver supports ControllerPublishUnpublish, using real CSI handler")
		pvLister := factory.Core().V1().PersistentVolumes().Lister()
		nodeLister := factory.Core().V1().Nodes().Lister()
		vaLister := factory.Storage().V1beta1().VolumeAttachments().Lister()
		csiNodeLister := factory.Storage().V1beta1().CSINodes().Lister()
		attacher := attacher.NewAttacher(csiConn)
		handler := controller.NewCSIHandler(controllerClient.KubernetesClientSet, driverName, attacher, pvLister, nodeLister, csiNodeLister, vaLister, &args.Timeout, supportsReadOnly)

		attacherCtrl = controller.NewCSIAttachController(
			controllerClient.KubernetesClientSet,
			driverName,
			handler,
			factory.Storage().V1beta1().VolumeAttachments(),
			factory.Core().V1().PersistentVolumes(),
			workqueue.NewItemExponentialFailureRateLimiter(args.AttacherRetryIntervalStart, args.AttacherRetryIntervalMax),
			workqueue.NewItemExponentialFailureRateLimiter(args.AttacherRetryIntervalStart, args.AttacherRetryIntervalMax),
		)
	}

	run := func(ctx context.Context) {
		stopCh := ctx.Done()
		factory.Start(stopCh)

		if attacherCtrl != nil {
			attacherCtrl.Run(int(args.WorkerThreads), stopCh)
		}
	}

	// Name of config map with leader election lock
	lockName := "csi-controller-leader-" + driverName
	le := leaderelection.NewLeaderElection(controllerClient.KubernetesClientSet, lockName, run)

	if args.LeaderElectionNamespace != "" {
		le.WithNamespace(args.LeaderElectionNamespace)
	}

	if err := le.Run(); err != nil {
		klog.Fatalf("failed to initialize leader election: %v", err)
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
