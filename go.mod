module github.com/k8snetworkplumbingwg/ovs-cni

require (
	github.com/Mellanox/sriovnet v1.0.2
	github.com/containernetworking/cni v1.0.1
	github.com/containernetworking/plugins v1.0.1
	github.com/golang/glog v1.2.4
	github.com/imdario/mergo v0.3.12
	github.com/j-keck/arping v1.0.2
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200626054723-37f83d1996bc
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.31.0
	github.com/ovn-org/libovsdb v0.6.0
	github.com/pkg/errors v0.9.1
	github.com/vishvananda/netlink v1.1.1-0.20210330154013-f5de75959ad5
	k8s.io/api v0.30.3
	k8s.io/apimachinery v0.30.3
	k8s.io/client-go v0.30.3
	kubevirt.io/qe-tools v0.1.6
)

require (
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cenkalti/hub v1.0.1 // indirect
	github.com/cenkalti/rpc2 v0.0.0-20210220005819-4a29bc83afe1 // indirect
	github.com/coreos/go-iptables v0.6.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/safchain/ethtool v0.0.0-20210803160452-9aa261dae9b1 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/oauth2 v0.13.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/term v0.27.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

// Pinned to kubernetes-1.30.3
replace (
	k8s.io/api => k8s.io/api v0.30.3
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.30.3
	k8s.io/apimachinery => k8s.io/apimachinery v0.30.3
	k8s.io/apiserver => k8s.io/apiserver v0.30.3
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.30.3
	k8s.io/client-go => k8s.io/client-go v0.30.3
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.30.3
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.30.3
	k8s.io/code-generator => k8s.io/code-generator v0.30.3
	k8s.io/component-base => k8s.io/component-base v0.30.3
	k8s.io/cri-api => k8s.io/cri-api v0.30.3
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.30.3
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.30.3
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.30.3
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.30.3
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.30.3
	k8s.io/kubectl => k8s.io/kubectl v0.30.3
	k8s.io/kubelet => k8s.io/kubelet v0.30.3
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.30.3
	k8s.io/metrics => k8s.io/metrics v0.30.3
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.30.3
)

go 1.22.0

toolchain go1.22.5
