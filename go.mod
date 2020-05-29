module github.com/kubevirt/ovs-cni

go 1.12

require (
	github.com/Mellanox/sriovnet v0.0.0-20190516174650-73402dc8fcaa
	github.com/cenk/hub v1.0.1 // indirect
	github.com/cenkalti/hub v1.0.1 // indirect
	github.com/cenkalti/rpc2 v0.0.0-20180727162946-9642ea02d0aa // indirect
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.8.6
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/googleapis/gnostic v0.3.1 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/json-iterator/go v1.1.8 // indirect
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/satori/go.uuid v1.2.1-0.20181028125025-b2ce2384e17b // indirect
	github.com/socketplane/libovsdb v0.0.0-20170116174820-4de3618546de
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/vishvananda/netlink v0.0.0-20181108222139-023a6dafdcdf
	github.com/vishvananda/netns v0.0.0-20200520041808-52d707b772fe // indirect
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37 // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/api v0.0.0-00010101000000-000000000000 // indirect
	k8s.io/apimachinery v0.0.0-00010101000000-000000000000
	k8s.io/client-go v0.0.0-00010101000000-000000000000
	k8s.io/utils v0.0.0-20191030222137-2b95a09bc58d // indirect
	kubevirt.io/qe-tools v0.1.3
	sigs.k8s.io/yaml v1.1.0 // indirect
)

// Resolve issues with mod and Kubernetes
replace (
	// curl -s https://proxy.golang.org/k8s.io/api/@v/kubernetes-1.14.8.info | jq -r .Version
	k8s.io/api => k8s.io/api v0.0.0-20191004102349-159aefb8556b
	// curl -s https://proxy.golang.org/k8s.io/apimachinery/@v/kubernetes-1.14.8.info | jq -r .Version
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191004074956-c5d2f014d689
	// curl -s https://proxy.golang.org/k8s.io/client-go/@v/kubernetes-1.14.8.info | jq -r .Version
	k8s.io/client-go => k8s.io/client-go v11.0.1-0.20191004102930-01520b8320fc+incompatible
)
