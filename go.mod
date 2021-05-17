module github.com/k8snetworkplumbingwg/ovs-cni

require (
	github.com/Mellanox/sriovnet v1.0.2
	github.com/cenk/hub v1.0.1 // indirect
	github.com/cenkalti/hub v1.0.1 // indirect
	github.com/cenkalti/rpc2 v0.0.0-20180727162946-9642ea02d0aa // indirect
	github.com/containernetworking/cni v0.8.1
	github.com/containernetworking/plugins v0.9.1-0.20210203133829-74a6b28a2c27
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/imdario/mergo v0.3.8
	github.com/j-keck/arping v0.0.0-20160618110441-2cf9dc699c56
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200626054723-37f83d1996bc
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.3
	github.com/pkg/errors v0.9.1
	github.com/socketplane/libovsdb v0.0.0-20170116174820-4de3618546de
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	k8s.io/api v0.19.1
	k8s.io/apimachinery v0.19.1
	k8s.io/client-go v0.18.3
	kubevirt.io/qe-tools v0.1.6
)

// Pinned to kubernetes-1.19.1
replace (
	golang.org/x/text => golang.org/x/text v0.3.3
	k8s.io/api => k8s.io/api v0.19.1
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.1
	k8s.io/apiserver => k8s.io/apiserver v0.19.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.19.1
	k8s.io/client-go => k8s.io/client-go v0.19.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.19.1
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.19.1
	k8s.io/code-generator => k8s.io/code-generator v0.19.1
	k8s.io/component-base => k8s.io/component-base v0.19.1
	k8s.io/cri-api => k8s.io/cri-api v0.19.1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.19.1
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.1
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.19.1
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.19.1
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.19.1
	k8s.io/kubectl => k8s.io/kubectl v0.19.1
	k8s.io/kubelet => k8s.io/kubelet v0.19.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.19.1
	k8s.io/metrics => k8s.io/metrics v0.19.1
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.19.1
)

replace github.com/containernetworking/cni => github.com/containernetworking/cni v0.8.1

go 1.13
