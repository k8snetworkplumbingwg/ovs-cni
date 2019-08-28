// Copyright 2018 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugin

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const BRIDGE_NAME = "test-bridge"
const VLAN_ID = 100
const MTU = 1456

var _ = BeforeSuite(func() {
	output, err := exec.Command("ovs-vsctl", "show").CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Open vSwitch is not available, if you have it installed and running, try to run tests with `sudo -E`: %v", string(output[:]))
})

var _ = AfterSuite(func() {
	exec.Command("ovs-vsctl", "del-br", "--if-exists", BRIDGE_NAME).Run()
})

var _ = Describe("CNI Plugin", func() {

	BeforeEach(func() {
		output, err := exec.Command("ovs-vsctl", "add-br", BRIDGE_NAME).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to create testing OVS bridge: %v", string(output[:]))

		bridgeLink, err := netlink.LinkByName(BRIDGE_NAME)
		Expect(err).NotTo(HaveOccurred(), "Interface of testing OVS bridge was not found in the system")

		err = netlink.LinkSetUp(bridgeLink)
		Expect(err).NotTo(HaveOccurred(), "Was not able to set bridge UP")
	})

	AfterEach(func() {
		output, err := exec.Command("ovs-vsctl", "del-br", BRIDGE_NAME).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to remove testing OVS bridge: %v", string(output[:]))
	})

	testAddDel := func(conf string, setVlan, setMtu bool) {
		const IFNAME = "eth0"

		By("Creating temporary target namespace to simulate a container")
		targetNs, err := ns.NewNS()
		Expect(err).NotTo(HaveOccurred())
		defer targetNs.Close()

		args := &skel.CmdArgs{
			ContainerID: "dummy",
			Netns:       targetNs.Path(),
			IfName:      IFNAME,
			StdinData:   []byte(conf),
		}

		var result *current.Result

		By("Calling ADD command")
		r, _, err := cmdAddWithArgs(args, func() error {
			return CmdAdd(args)
		})
		Expect(err).NotTo(HaveOccurred())

		By("Checking that result of ADD command in in expected format")
		result, err = current.GetResult(r)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(result.Interfaces)).To(Equal(3))
		Expect(len(result.IPs)).To(Equal(0))

		brIface := result.Interfaces[0]
		hostIface := result.Interfaces[1]
		contIface := result.Interfaces[2]

		By("Checking that bridge interface name in the result matches requested")
		Expect(brIface.Name).To(Equal(BRIDGE_NAME))

		By("Checking that bridge interface MAC in the result matches reality")
		brLink, err := netlink.LinkByName(BRIDGE_NAME)
		Expect(err).NotTo(HaveOccurred())
		brHwaddr, err := net.ParseMAC(brIface.Mac)
		Expect(err).NotTo(HaveOccurred())
		Expect(brLink.Attrs().HardwareAddr).To(Equal(brHwaddr))

		By("Checking that host interface MAC in the result matches reality")
		hostLink, err := netlink.LinkByName(hostIface.Name)
		Expect(err).NotTo(HaveOccurred())
		hostHwaddr, err := net.ParseMAC(hostIface.Mac)
		Expect(err).NotTo(HaveOccurred())
		Expect(hostLink.Attrs().HardwareAddr).To(Equal(hostHwaddr))

		By("Checking that the host iface is connected as a port to the bridge")
		brPorts, err := listBridgePorts(BRIDGE_NAME)
		Expect(err).NotTo(HaveOccurred())
		Expect(brPorts).To(Equal([]string{hostIface.Name}))

		By("Checking that port the VLAN ID matches expected state")
		portVlan, err := getPortAttribute(hostIface.Name, "tag")
		Expect(err).NotTo(HaveOccurred())
		if setVlan {
			Expect(portVlan).To(Equal(strconv.Itoa(VLAN_ID)))
		} else {
			Expect(portVlan).To(Equal("[]"))
		}

		if setMtu {
			Expect(hostLink.Attrs().MTU).To(Equal(MTU))
		}

		By("Checking that port external-id:contIface contains reference to container interface name")
		externalIdContIface, err := getPortAttribute(hostIface.Name, "external-ids:contIface")
		Expect(err).NotTo(HaveOccurred())
		Expect(externalIdContIface).To(Equal("\"" + contIface.Name + "\""))

		By("Checking that port external-id:contNetns contains reference to container namespace path")
		externalIdContNetns, err := getPortAttribute(hostIface.Name, "external-ids:contNetns")
		Expect(err).NotTo(HaveOccurred())
		Expect(externalIdContNetns).To(Equal("\"" + targetNs.Path() + "\""))

		By("Verifying situation inside the container")
		err = targetNs.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			By("Checking that veth interface was created inside the container")
			contLink, err := netlink.LinkByName(IFNAME)
			Expect(err).NotTo(HaveOccurred())

			By("Checking that container interface MAC in the result matches reality")
			contHwaddr, err := net.ParseMAC(contIface.Mac)
			Expect(err).NotTo(HaveOccurred())
			Expect(contLink.Attrs().HardwareAddr).To(Equal(contHwaddr))

			By("Checking that container interface is set UP")
			Expect(contLink.Attrs().OperState).To(Equal(netlink.LinkOperState(netlink.OperUp)))

			if setMtu {
				Expect(contLink.Attrs().MTU).To(Equal(MTU))
			}

			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Calling DEL command")
		err = cmdDelWithArgs(args, func() error {
			return CmdDel(args)
		})
		Expect(err).NotTo(HaveOccurred())

		By("Verifying situation inside the container")
		err = targetNs.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			By("Checking that container side of the veth pair was deleted")
			contLink, err := netlink.LinkByName(IFNAME)
			Expect(err).To(HaveOccurred())
			Expect(contLink).To(BeNil())

			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Checking that host side of the veth pair was deleted")
		hostLink, err = netlink.LinkByName(hostIface.Name)
		Expect(err).To(HaveOccurred())
		Expect(hostLink).To(BeNil())

		By("Checking that the port on OVS bridge was deleted")
		brPorts, err = listBridgePorts(BRIDGE_NAME)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(brPorts)).To(Equal(0))
	}

	Context("connecting container to a bridge", func() {
		Context("with VLAN ID set on port", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
				"vlan": %d
			}`, BRIDGE_NAME, VLAN_ID)
			It("should successfully complete ADD and DEL commands", func() {
				testAddDel(conf, true, false)
			})
		})
		Context("without a VLAN ID set on port", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s"
			}`, BRIDGE_NAME)
			It("should successfully complete ADD and DEL commands", func() {
				testAddDel(conf, false, false)
			})
		})
		Context("with MTU set on port", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
                "mtu": %d
			}`, BRIDGE_NAME, MTU)
			It("should successfully complete ADD and DEL commands", func() {
				testAddDel(conf, false, true)
			})
		})
		Context("random mac address on container interface", func() {
			It("should create eth0 on two different namespace with different mac addresses", func() {
				conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
				"vlan": %d
				}`, BRIDGE_NAME, VLAN_ID)

				const IFNAME = "eth0"

				By("Creating two temporary target namespace to simulate two containers")
				targetNsOne, err := ns.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer targetNsOne.Close()
				targetNsTwo, err := ns.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer targetNsTwo.Close()

				By("Checking that both namespaces have different mac addresses on eth0")
				resultOne := attach(targetNsOne, conf, IFNAME, "", "")
				contOneIface := resultOne.Interfaces[2]

				resultTwo := attach(targetNsTwo, conf, IFNAME, "", "")
				contTwoIface := resultTwo.Interfaces[2]

				Expect(contOneIface.Mac).NotTo(Equal(contTwoIface.Mac))
			})
		})
		Context("specified mac address on container interface", func() {
			It("should create eth0 with the specified mac address", func() {
				conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
				"vlan": %d
				}`, BRIDGE_NAME, VLAN_ID)

				const IFNAME = "eth0"

				By("Creating temporary target namespace to simulate a container")
				targetNs, err := ns.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer targetNs.Close()

				By("Checking that the mac address on eth0 equals to the requested one")
				mac := "0a:00:00:00:00:80"
				result := attach(targetNs, conf, IFNAME, mac, "")
				contIface := result.Interfaces[2]

				Expect(contIface.Mac).To(Equal(mac))
			})
		})
		Context("specified OvnPort", func() {
			It("should configure and ovs interface with iface-id", func() {
				const IFNAME = "eth0"
				const ovsOutput = "external_ids        : {iface-id=test-port}"

				conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"OvnPort": "test-port",
				"bridge": "%s"}`, BRIDGE_NAME)

				targetNs, err := ns.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer targetNs.Close()

				OvnPort := "test-port"
				result := attach(targetNs, conf, IFNAME, "", OvnPort)
				hostIface := result.Interfaces[1]
				output, err := exec.Command("ovs-vsctl", "--colum=external_ids", "find", "Interface", fmt.Sprintf("name=%s", hostIface.Name)).CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output[:len(output)-1])).To(Equal(ovsOutput))
			})
		})
	})
})

func attach(namespace ns.NetNS, conf, ifName, mac, ovnPort string) *current.Result {
	extraArgs := ""
	if mac != "" {
		extraArgs += fmt.Sprintf("MAC=%s,", mac)
	}

	if ovnPort != "" {
		extraArgs += fmt.Sprintf("OvnPort=%s", ovnPort)
	}

	if strings.HasSuffix(extraArgs, ",") {
		extraArgs = extraArgs[:len(extraArgs)-1]
	}

	args := &skel.CmdArgs{
		ContainerID: "dummy",
		Netns:       namespace.Path(),
		IfName:      ifName,
		StdinData:   []byte(conf),
		Args:        extraArgs,
	}

	By("Calling ADD command")
	r, _, err := cmdAddWithArgs(args, func() error {
		return CmdAdd(args)
	})
	Expect(err).NotTo(HaveOccurred())

	By("Checking that result of ADD command in in expected format")
	result, err := current.GetResult(r)
	Expect(err).NotTo(HaveOccurred())

	return result
}

func cmdAddWithArgs(args *skel.CmdArgs, f func() error) (types.Result, []byte, error) {
	return testutils.CmdAddWithResult(args.Netns, args.IfName, args.StdinData, f)
}

func cmdDelWithArgs(args *skel.CmdArgs, f func() error) error {
	return testutils.CmdDelWithResult(args.Netns, args.IfName, f)
}

func listBridgePorts(brName string) ([]string, error) {
	output, err := exec.Command("ovs-vsctl", "list-ports", brName).CombinedOutput()
	if err != nil {
		return make([]string, 0), fmt.Errorf("failed to list bridge ports: %v", string(output[:]))
	}

	outputLines := strings.Split(string(output[:]), "\n")

	return outputLines[:len(outputLines)-1], nil
}

func getPortAttribute(portName string, attributeName string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "get", "Port", portName, attributeName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get port attribute: %v", string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}
