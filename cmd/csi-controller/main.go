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
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	ctrl "github.com/kubernetes-csi/external-provisioner/pkg/controller"

	"google.golang.org/grpc"
)

const (

	// Default timeout of short CSI calls like GetPluginInfo
	csiTimeout = time.Second

	leaderElectionTypeLeases     = "leases"
	leaderElectionTypeConfigMaps = "configmaps"
)

type CommonOpts struct {
	MasterURL               string
	Kubeconfig              string
	Resync                  time.Duration
	CsiAddress              string
	LeaderElectionType      string
	LeaderElectionNamespace string
	WorkerThreads           int
	Timeout                 time.Duration
	RetryIntervalStart      time.Duration
	RetryIntervalMax        time.Duration
}

type ProvisionerOpts struct {
	ProvisionerVolumeNamePrefix     string
	ProvisionerVolumeNameUUIDLength int
	ProvisionerStrictTopology       bool
}

type SnapshotterOpts struct {
	SnapshotterSnapshotContentRetryCount int
	SnapshotterSnapshotContentInterval   time.Duration
	SnapshotterNamePrefix                string
	SnapshotterNameUUIDLength            int
}

type CliOpts struct {
	CommonOpts
	ProvisionerOpts
	SnapshotterOpts
}

type ControllerClient struct {
	ControllerArgs        CliOpts
	FeatureGates          map[string]bool
	RestConfig            *rest.Config
	DriverName            string
	PluginCapabilites     connection.PluginCapabilitySet
	ControllerCapabilites connection.ControllerCapabilitySet
	KubernetesClientSet   *kubernetes.Clientset
	CsiConn               *grpc.ClientConn
}

// RunnerHandler is a function that runs the controller in a leader election loop
type RunnerHandler func(ctx context.Context, stopCh <-chan struct{})

// LeaderElectionRunner defines an interface for the leader election execution function
type LeaderElectionRunner interface {

	// Runner returns a RunnerHandler to be executed by the lead election function.
	// If RunnerHandler is nil, then the controller has detected that it does not support
	// the function.
	Runner() (RunnerHandler, error)
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
	flag.StringVar(&c.MasterURL, "master", "", "Master URL to build a client config from. Either this or kubeconfig needs to be set if the provisioner is being run out of cluster.")
	flag.StringVar(&c.CsiAddress, "csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	flag.DurationVar(&c.Resync, "resync", 10*time.Minute, "Resync interval of the controller.")
	flag.StringVar(&c.LeaderElectionType, "leader-election-type", "endpoints", "The type of leader election, options are 'endpoints' (default) or 'leases' (strongly recommended). The 'endpoints' option is deprecated in favor of 'leases'.")
	flag.StringVar(&c.CsiAddress, "leader-election-namespace", "", "Namespace where the leader election resource lives. Defaults to the pod namespace if not set.")
	flag.IntVar(&c.WorkerThreads, "worker-threads", 10, "Number of attacher worker threads")
	flag.DurationVar(&c.Timeout, "timeout", time.Minute, "Timeout for waiting for driver to be ready")
	flag.DurationVar(&c.RetryIntervalStart, "retry-interval-start", time.Second, "Initial retry interval of failed create volume or deletion. It doubles with each failure, up to retry-interval-max.")
	flag.DurationVar(&c.RetryIntervalMax, "retry-interval-max", 5*time.Minute, "Maximum retry interval of failed create volume or deletion.")

	// Provisioner
	flag.StringVar(&c.ProvisionerVolumeNamePrefix, "provisioner-volume-name-prefix", "pvc", "Prefix to apply to the name of a created volume.")
	flag.IntVar(&c.ProvisionerVolumeNameUUIDLength, "provisioner-volume-name-uuid-length", -1, "Truncates generated UUID of a created volume to this length. Defaults behavior is to NOT truncate.")
	flag.BoolVar(&c.ProvisionerStrictTopology, "strict-topology", false, "Passes only selected node topology to CreateVolume Request, unlike default behavior of passing aggregated cluster topologies that match with topology keys of the selected node.")

	// Snapshotter
	flag.IntVar(&c.SnapshotterSnapshotContentRetryCount, "create-snapshotcontent-retrycount", 5, "Number of retries when we create a snapshot content object for a snapshot.")
	flag.DurationVar(&c.SnapshotterSnapshotContentInterval, "create-snapshotcontent-interval", 10*time.Second, "Interval between retries when we create a snapshot content object for a snapshot.")
	flag.StringVar(&c.SnapshotterNamePrefix, "snapshot-name-prefix", "snapshot", "Prefix to apply to the name of a created snapshot")
	flag.IntVar(&c.SnapshotterNameUUIDLength, "snapshot-name-uuid-length", -1, "Length in characters for the generated uuid of a created snapshot. Defaults behavior is to NOT truncate.")
}

func main() {

	flag.Var(utilflag.NewMapStringBool(&controllerClient.FeatureGates), "feature-gates", "A set of key=value pairs that describe feature gates for alpha/experimental features. "+
		"Options are:\n"+strings.Join(utilfeature.DefaultFeatureGate.KnownFeatures(), "\n"))

	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(controllerClient.FeatureGates); err != nil {
		klog.Fatal(err)
	}

	// COMMON ----------------------------------------------------------
	if *showVersion {
		fmt.Println(os.Args[0], version)
		return
	}
	klog.Infof("Version: %s", version)

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	args := &controllerClient.ControllerArgs
	var err error
	controllerClient.RestConfig, err = buildConfig(args.MasterURL, args.Kubeconfig)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	if args.WorkerThreads == 0 {
		klog.Error("option -worker-threads must be greater than zero")
		os.Exit(1)
	}

	controllerClient.KubernetesClientSet, err = kubernetes.NewForConfig(controllerClient.RestConfig)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// Connect to CSI.
	controllerClient.CsiConn, err = ctrl.Connect(args.CsiAddress)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// Wait for the driver to be ready
	err = rpc.ProbeForever(controllerClient.CsiConn, args.Timeout)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// Find driver name.
	controllerClient.DriverName, err = ctrl.GetDriverName(controllerClient.CsiConn, args.Timeout)
	if err != nil {
		klog.Fatalf("Error getting CSI driver name: %s", err)
	}
	klog.V(2).Infof("Detected CSI driver %s", controllerClient.DriverName)

	// Create a context
	ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
	defer cancel()

	// Determine if the driver supports the controller service
	supportsService, err := supportsPluginControllerService(ctx, controllerClient.CsiConn)
	if err != nil {
		klog.Fatalf("Unable to determine if the CSI driver supports the controller services: %v", err)
	}
	if !supportsService {
		klog.Fatal("CSI driver does not support Plugin Controller Service")
	}

	// Get the capabilities of the driver
	controllerClient.PluginCapabilites,
		controllerClient.ControllerCapabilites,
		err = ctrl.GetDriverCapabilities(controllerClient.CsiConn, args.Timeout)
	if err != nil {
		klog.Fatalf("Error getting CSI driver capabilities: %s", err)
	}

	// Create a list of controller runners
	runners := make([]RunnerHandler, 0)

	// Setup Attacher
	attacher, err := Attacher(&controllerClient)
	if err != nil {
		klog.Fatalf("Error starting attacher: %v", err)
	}
	runners = append(runners, attacher)

	// Setup Provisioner
	provisioner, err := Provisioner(&controllerClient)
	if err != nil {
		klog.Fatalf("Error starting provisioner: %v", err)
	}
	runners = append(runners, provisioner)

	// Setup Snapshotter
	snapshotter, err := Snapshotter(&controllerClient)
	if err != nil {
		klog.Fatalf("Error starting snapshotter: %v", err)
	}
	runners = append(runners, snapshotter)

	// Setup Resizer
	resizer, err := Resizer(&controllerClient)
	if err != nil {
		klog.Fatalf("Error starting resizer: %v", err)
	}
	runners = append(runners, resizer)

	// Leader runner ----------------------------------------------------
	run := func(ctx context.Context) {
		stopCh := ctx.Done()

		for _, runner := range runners {
			if runner != nil {
				go runner(ctx, stopCh)
			}
		}

		// ...until SIGINT
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
	}

	// Name of config map with leader election lock
	lockName := "csi-controller-leader-" + controllerClient.DriverName
	le := leaderelection.NewLeaderElection(controllerClient.KubernetesClientSet, lockName, run)

	if args.LeaderElectionNamespace != "" {
		le.WithNamespace(args.LeaderElectionNamespace)
	}

	if err := le.Run(); err != nil {
		klog.Fatalf("failed to initialize leader election: %v", err)
	}
}

func buildConfig(master, kubeconfig string) (*rest.Config, error) {
	// get the KUBECONFIG from env if specified (useful for local/debug cluster)
	kubeconfigEnv := os.Getenv("KUBECONFIG")

	if kubeconfigEnv != "" {
		klog.Infof("Found KUBECONFIG environment variable set, using that..")
		kubeconfig = kubeconfigEnv
	}

	if kubeconfig != "" || master != "" {
		return clientcmd.BuildConfigFromFlags(master, kubeconfig)
	}
	return rest.InClusterConfig()
}

func supportsPluginControllerService(ctx context.Context, csiConn *grpc.ClientConn) (bool, error) {
	caps, err := rpc.GetPluginCapabilities(ctx, csiConn)
	if err != nil {
		return false, err
	}

	return caps[csi.PluginCapability_Service_CONTROLLER_SERVICE], nil
}
