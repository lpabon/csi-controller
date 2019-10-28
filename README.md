# CSI Controller
Goal of this project is to create a simple, single, intelligent CSI controller
which can interrogate the Kubernetes version and the CSI driver to
determine which controller to load automatically. It will also setup the
appropriate objects and settings depending on the Kubernetes version.

This version of the CSI controller uses the following versions of the side cars:

* external-provisioner v1.4.0
* external-attacher v2.0.0
* external-resizer master branch
* external-snapshotter release-1.1 branch

RBAC and deployment are available in [deploy](https://github.com/lpabon/csi-controller/tree/master/deploy/kubernetes) directory

### Example output of the logs:

```
I1028 22:29:07.857386       1 feature_gate.go:216] feature gates: &{map[]}
I1028 22:29:07.857476       1 main.go:165] Version: ac8e2f1381bcf40b04bf87b802dbf76976a1d5c4
W1028 22:29:07.871800       1 main.go:229] Skipping CSI NODE CRD check
I1028 22:29:07.871834       1 connection.go:151] Connecting to unix:///csi/csi.sock
I1028 22:29:07.874311       1 common.go:111] Probing CSI driver for readiness
I1028 22:29:07.876505       1 main.go:251] Detected CSI driver pxd.portworx.com
I1028 22:29:07.882236       1 attacher.go:58] Kubernetes 1.13.x detected which requires the attacher process even if the CSI driver does not support it
I1028 22:29:07.882552       1 provisioner.go:44] Driver pxd.portworx.com supports provisioning
I1028 22:29:07.884601       1 controller.go:680] Using saving PVs to API server in background
I1028 22:29:07.884705       1 snapshotter.go:44] Driver pxd.portworx.com supports snapshots
I1028 22:29:07.900629       1 resizer.go:38] Driver pxd.portworx.com supports resize
I1028 22:29:07.900661       1 connection.go:151] Connecting to unix:///csi/csi.sock
I1028 22:29:07.904069       1 common.go:111] Probing CSI driver for readiness
I1028 22:29:07.906901       1 main.go:327] Using endpoints for leader election in Kubernetes version 1.13.x
I1028 22:29:07.907061       1 leaderelection.go:241] attempting to acquire leader lease  kube-system/csi-controller-leader-pxd-portworx-com...
I1028 22:29:07.915215       1 leader_election.go:172] new leader detected, current leader: px-csi-ext-75948bcdc6-qtx7c
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
