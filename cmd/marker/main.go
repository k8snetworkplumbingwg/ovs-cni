package main

import (
	"flag"

	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"kubevirt.io/ovs-cni/pkg/marker"
)

func main() {
	flag.Parse()

	manager := dpm.NewManager(marker.Lister{})
	manager.Run()
}
