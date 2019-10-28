# CSI Controller
Goal of this project is to create a simple, single, intelligent CSI controller
which can interrogate the Kubernetes version and the CSI driver to
determine which controller to load automatically. It will also setup the
appropriate objects and settings depending on the Kubernetes version.

Use the container image `quay.io/lpabon/csi-controller:v0.2`. This version of
the CSI controller uses the following versions of the side cars:

* external-provisioner v1.4.0
* external-attacher v2.0.0
* external-resizer master branch
* external-snapshotter release-1.1 branch

RBAC and deployment are available in [deploy](https://github.com/lpabon/csi-controller/tree/master/deploy/kubernetes) directory

### Example output of the logs:

```
I1028 23:17:05.204711       1 feature_gate.go:216] feature gates: &{map[]}
I1028 23:17:05.204831       1 main.go:165] Version: v0.1-5-g0df2487
W1028 23:17:05.222373       1 main.go:229] Skipping CSI NODE CRD check
I1028 23:17:05.222407       1 connection.go:151] Connecting to unix:///csi/csi.sock
I1028 23:17:05.224130       1 common.go:111] Probing CSI driver for readiness
I1028 23:17:05.225893       1 main.go:251] Detected CSI driver pxd.portworx.com
I1028 23:17:05.229003       1 attacher.go:59] Kubernetes 1.13.x detected which requires the attacher process even if the CSI driver does not support it
I1028 23:17:05.229217       1 provisioner.go:44] Driver pxd.portworx.com supports provisioning
I1028 23:17:05.231177       1 controller.go:680] Using saving PVs to API server in background
I1028 23:17:05.231215       1 snapshotter.go:44] Driver pxd.portworx.com supports snapshots
I1028 23:17:05.243319       1 resizer.go:38] Driver pxd.portworx.com supports resize
I1028 23:17:05.243347       1 connection.go:151] Connecting to unix:///csi/csi.sock
I1028 23:17:05.244133       1 common.go:111] Probing CSI driver for readiness
I1028 23:17:05.247155       1 main.go:327] Using endpoints for leader election in Kubernetes version 1.13.x
I1028 23:17:05.247363       1 leaderelection.go:241] attempting to acquire leader lease  kube-system/csi-controller-leader-pxd-portworx-com...
I1028 23:17:24.892907       1 leader_election.go:172] new leader detected, current leader: px-csi-ext-77ffb6d4f9-7ldtr
```

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

* Slack channels
  * [#wg-csi](https://kubernetes.slack.com/messages/wg-csi)
  * [#sig-storage](https://kubernetes.slack.com/messages/sig-storage)
* [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
