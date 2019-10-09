module github.com/kubernetes-csi/csi-controller

go 1.13

require (
	github.com/container-storage-interface/spec v1.1.0
	github.com/davecgh/go-spew v1.1.1
	github.com/evanphx/json-patch v4.1.0+incompatible
	github.com/gogo/protobuf v1.2.1
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef
	github.com/golang/mock v1.2.0
	github.com/golang/protobuf v1.3.1
	github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf
	github.com/googleapis/gnostic v0.2.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/imdario/mergo v0.3.7
	github.com/json-iterator/go v1.1.6
	github.com/kubernetes-csi/csi-lib-utils v0.6.1
	github.com/kubernetes-csi/csi-test v2.0.0+incompatible
	github.com/kubernetes-csi/external-attacher v2.0.0+incompatible
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 v1.0.1
	github.com/spf13/pflag v1.0.3
	golang.org/x/crypto v0.0.0-20190403202508-8e1b8d32e692
	golang.org/x/net v0.0.0-20190403144856-b630fd6fe46b
	golang.org/x/oauth2 v0.0.0-20190402181905-9f3314589c9a
	golang.org/x/sys v0.0.0-20190403152447-81d4e9dc473e
	golang.org/x/text v0.3.1-0.20181227161524-e6919f6577db
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/appengine v1.5.0
	google.golang.org/genproto v0.0.0-20190401181712-f467c93bbac2
	google.golang.org/grpc v1.19.1
	gopkg.in/inf.v0 v0.9.1
	gopkg.in/yaml.v2 v2.2.2
	k8s.io/api v0.0.0-20190620084959-7cf5895f2711
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/apiserver v0.0.0-20190404070728-fb0d7b20d176 // indirect
	k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
	k8s.io/cloud-provider v0.0.0-20190614051103-08f4196f6866
	k8s.io/csi-translation-lib v0.0.0-20190615091142-9ff632302e7e
	k8s.io/klog v0.3.2
	k8s.io/kube-openapi v0.0.0-20190401085232-94e1e7b7574c
	k8s.io/utils v0.0.0-20190308190857-21c4ce38f2a7
	sigs.k8s.io/yaml v1.1.0
)
