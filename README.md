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
