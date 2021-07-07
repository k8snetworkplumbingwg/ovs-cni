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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

const bridgeName = "test-bridge"
const vlanID = 100
const mtu = 1456
const defaultMTU = 1500

var _ = BeforeSuite(func() {
	output, err := exec.Command("ovs-vsctl", "show").CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Open vSwitch is not available, if you have it installed and running, try to run tests with `sudo -E`: %v", string(output[:]))
})

var _ = AfterSuite(func() {
	exec.Command("ovs-vsctl", "del-br", "--if-exists", bridgeName).Run()
})

var _ = Describe("CNI Plugin", func() {

	BeforeEach(func() {
		output, err := exec.Command("ovs-vsctl", "add-br", bridgeName).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to create testing OVS bridge: %v", string(output[:]))

		bridgeLink, err := netlink.LinkByName(bridgeName)
		Expect(err).NotTo(HaveOccurred(), "Interface of testing OVS bridge was not found in the system")

		err = netlink.LinkSetUp(bridgeLink)
		Expect(err).NotTo(HaveOccurred(), "Was not able to set bridge UP")
	})

	AfterEach(func() {
		output, err := exec.Command("ovs-vsctl", "del-br", bridgeName).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to remove testing OVS bridge: %v", string(output[:]))
	})

	testSplitVlanIds := func(conf string, expTrunks []uint, expErr error, setUnmarshalErr bool) {
		var trunks []*types.Trunk
		err := json.Unmarshal([]byte(conf), &trunks)
		if setUnmarshalErr {
			Expect(err).To(HaveOccurred())
			return
		}
		Expect(err).NotTo(HaveOccurred())
		By("Calling testSplitVlanIds method")
		vlanIds, err := splitVlanIds(trunks)
		if expErr != nil {
			By("Checking expected error is occurred")
			Expect(err).To(Equal(expErr))
		} else {
			By("Checking vlanIds are same as trunk vlans")
			Expect(vlanIds).To(Equal(expTrunks))
		}
	}

	testIPAM := func(conf string, isDual bool, ipPrefix, ip6Prefix string) {
		const IFNAME = "eth0"

		By("Creating temporary target namespace to simulate a container")
		targetNs, err := testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			Expect(targetNs.Close()).To(Succeed())
			Expect(testutils.UnmountNS(targetNs)).To(Succeed())
		}()

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
		Expect(len(result.Interfaces)).To(Equal(2))
		if isDual {
			Expect(len(result.IPs)).To(Equal(2))
		} else {
			Expect(len(result.IPs)).To(Equal(1))
		}

		By("Checking that IP config in result of ADD command is referencing the container interface index")
		Expect(result.IPs[0].Interface).To(Equal(current.Int(1)))

		err = targetNs.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			link, err := netlink.LinkByName(IFNAME)
			Expect(err).NotTo(HaveOccurred())
			Expect(link.Attrs().Name).To(Equal(IFNAME))

			hwaddr, err := net.ParseMAC(result.Interfaces[1].Mac)
			Expect(err).NotTo(HaveOccurred())
			Expect(link.Attrs().HardwareAddr).To(Equal(hwaddr))

			addrs, err := netlink.AddrList(link, syscall.AF_INET)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(addrs)).To(Equal(1))
			Expect(addrs[0].String()).To(HavePrefix(ipPrefix))

			if isDual {
				addrs, err := netlink.AddrList(link, syscall.AF_INET6)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(addrs)).To(Equal(2))
				Expect(addrs[0].String()).To(HavePrefix(ip6Prefix))
				Expect(addrs[1].String()).To(HavePrefix("fe80"))
			}

			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Calling DEL command to cleanup assigned ip address")
		err = cmdDelWithArgs(args, func() error {
			return CmdDel(args)
		})
		Expect(err).NotTo(HaveOccurred())
	}

	testAddDel := func(conf string, setVlan, setMtu bool, Trunk string) {
		const IFNAME = "eth0"

		By("Creating temporary target namespace to simulate a container")
		targetNs, err := testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			Expect(targetNs.Close()).To(Succeed())
			Expect(testutils.UnmountNS(targetNs)).To(Succeed())
		}()

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
		Expect(len(result.Interfaces)).To(Equal(2))
		Expect(len(result.IPs)).To(Equal(0))

		hostIface := result.Interfaces[0]
		contIface := result.Interfaces[1]

		By("Checking that host interface MAC in the result matches reality")
		hostLink, err := netlink.LinkByName(hostIface.Name)
		Expect(err).NotTo(HaveOccurred())
		hostHwaddr, err := net.ParseMAC(hostIface.Mac)
		Expect(err).NotTo(HaveOccurred())
		Expect(hostLink.Attrs().HardwareAddr).To(Equal(hostHwaddr))

		By("Checking that the host iface is connected as a port to the bridge")
		brPorts, err := listBridgePorts(bridgeName)
		Expect(err).NotTo(HaveOccurred())
		Expect(brPorts).To(Equal([]string{hostIface.Name}))

		By("Checking that port the VLAN ID matches expected state")
		portVlan, err := getPortAttribute(hostIface.Name, "tag")
		Expect(err).NotTo(HaveOccurred())
		if setVlan {
			Expect(portVlan).To(Equal(strconv.Itoa(vlanID)))
		} else {
			Expect(portVlan).To(Equal("[]"))
		}

		if setMtu {
			Expect(hostLink.Attrs().MTU).To(Equal(mtu))
		} else {
			Expect(hostLink.Attrs().MTU).To(Equal(defaultMTU))
		}

		By("Checking that Trunk VLAN range matches expected state")
		if len(Trunk) > 0 {
			portVlans, err := getPortAttribute(hostIface.Name, "trunks")
			Expect(err).NotTo(HaveOccurred())
			Expect(portVlans).To(Equal(Trunk))
		}

		By("Checking that port external-id:contIface contains reference to container interface name")
		externalIDContIface, err := getPortAttribute(hostIface.Name, "external-ids:contIface")
		Expect(err).NotTo(HaveOccurred())
		Expect(externalIDContIface).To(Equal(contIface.Name))

		By("Checking that port external-id:contNetns contains reference to container namespace path")
		externalIDContNetns, err := getPortAttribute(hostIface.Name, "external-ids:contNetns")
		Expect(err).NotTo(HaveOccurred())
		Expect(externalIDContNetns).To(Equal("\"" + targetNs.Path() + "\""))

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
				Expect(contLink.Attrs().MTU).To(Equal(mtu))
			} else {
				Expect(contLink.Attrs().MTU).To(Equal(defaultMTU))
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
		brPorts, err = listBridgePorts(bridgeName)
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
			}`, bridgeName, vlanID)
			It("should successfully complete ADD and DEL commands", func() {
				testAddDel(conf, true, false, "")
			})
		})
		Context("without a VLAN ID set on port", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s"
			}`, bridgeName)
			It("should successfully complete ADD and DEL commands", func() {
				testAddDel(conf, false, false, "")
			})
		})
		Context("with specific VLAN ID ranges set (via both range and id) for the port", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
				"trunk": [ {"minID": 10, "maxID": 12}, {"id": 15}, {"minID": 17, "maxID": 18}  ]
			}`, bridgeName)
			It("should successfully complete ADD and DEL commands", func() {
				testAddDel(conf, false, false, "[10, 11, 12, 15, 17, 18]")
			})
		})
		Context("with specific IPAM set for container interface", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
				"ipam": {
					"type": "host-local",
					"ranges": [[ {"subnet": "10.1.2.0/24", "gateway": "10.1.2.1"} ]],
					"dataDir": "/tmp/ovs-cni/conf"
				}
			}`, bridgeName)
			It("should successfully complete ADD and DEL commands", func() {
				testIPAM(conf, false, "10.1.2", "")
			})
		})
		Context("with dual stack ip addresses set for container interface", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
				"ipam": {
					"type": "host-local",
					"ranges": [[ {"subnet": "10.1.2.0/24", "gateway": "10.1.2.1"} ], [{"subnet": "3ffe:ffff:0:1ff::/64", "rangeStart": "3ffe:ffff:0:1ff::10", "rangeEnd": "3ffe:ffff:0:1ff::20"}]],
					"dataDir": "/tmp/ovs-cni/conf"
				}
			}`, bridgeName)
			It("should successfully complete ADD and DEL commands", func() {
				testIPAM(conf, true, "10.1.2", "3ffe:ffff:0:1ff")
			})
		})
		Context("with MTU set on port", func() {
			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"bridge": "%s",
				"mtu": %d
			}`, bridgeName, mtu)
			It("should successfully complete ADD and DEL commands", func() {
				testAddDel(conf, false, true, "")
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
				}`, bridgeName, vlanID)

				const IFNAME = "eth0"

				By("Creating two temporary target namespace to simulate two containers")
				targetNsOne, err := testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					Expect(targetNsOne.Close()).To(Succeed())
					Expect(testutils.UnmountNS(targetNsOne)).To(Succeed())
				}()
				targetNsTwo, err := testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					Expect(targetNsTwo.Close()).To(Succeed())
					Expect(testutils.UnmountNS(targetNsTwo)).To(Succeed())
				}()

				By("Checking that both namespaces have different mac addresses on eth0")
				resultOne := attach(targetNsOne, conf, IFNAME, "", "")
				contOneIface := resultOne.Interfaces[0]

				resultTwo := attach(targetNsTwo, conf, IFNAME, "", "")
				contTwoIface := resultTwo.Interfaces[1]

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
				}`, bridgeName, vlanID)

				const IFNAME = "eth0"

				By("Creating temporary target namespace to simulate a container")
				targetNs, err := testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					Expect(targetNs.Close()).To(Succeed())
					Expect(testutils.UnmountNS(targetNs)).To(Succeed())
				}()

				By("Checking that the mac address on eth0 equals to the requested one")
				mac := "0a:00:00:00:00:80"
				result := attach(targetNs, conf, IFNAME, mac, "")
				contIface := result.Interfaces[1]

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
				"bridge": "%s"}`, bridgeName)

				targetNs, err := testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					Expect(targetNs.Close()).To(Succeed())
					Expect(testutils.UnmountNS(targetNs)).To(Succeed())
				}()

				OvnPort := "test-port"
				result := attach(targetNs, conf, IFNAME, "", OvnPort)
				hostIface := result.Interfaces[0]
				output, err := exec.Command("ovs-vsctl", "--column=external_ids", "find", "Interface", fmt.Sprintf("name=%s", hostIface.Name)).CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output[:len(output)-1])).To(Equal(ovsOutput))
			})
		})
		Context("specify trunk with multiple ranges", func() {
			trunks := `[ {"minID": 10, "maxID": 12}, {"minID": 19, "maxID": 20} ]`
			It("testSplitVlanIds method should return with specifed values in the range", func() {
				testSplitVlanIds(trunks, []uint{10, 11, 12, 19, 20}, nil, false)
			})
		})
		Context("specify trunk with multiple ids", func() {
			trunks := `[ {"id": 15}, {"id": 19}, {"id": 40} ]`
			It("testSplitVlanIds method should return with specifed id values", func() {
				testSplitVlanIds(trunks, []uint{15, 19, 40}, nil, false)
			})
		})
		Context("specify trunk with minID/maxID same value and duplicate values", func() {
			trunks := `[ {"minID": 10, "maxID": 14}, {"id": 11}, {"minID": 13, "maxID": 13} ]`
			It("testSplitVlanIds method should return without duplicate trunk values", func() {
				testSplitVlanIds(trunks, []uint{10, 11, 12, 13, 14}, nil, false)
			})
		})
		Context("specify trunk with negative value", func() {
			trunks := `[ {"id": 15}, {"id": 15}, {"id": -20} ]`
			It("testSplitVlanIds method should throw appropriate error", func() {
				testSplitVlanIds(trunks, nil, errors.New("incorrect trunk id parameter"), true)
			})
		})
		Context("specify trunk with minID greater than maxID", func() {
			trunks := `[ {"minID": 10, "maxID": 12}, {"minID": 11, "maxID": 5} ]`
			It("testSplitVlanIds method should throw appropriate error", func() {
				testSplitVlanIds(trunks, nil, errors.New("minID is greater than maxID in trunk parameter"), false)
			})
		})
		Context("specify trunk with maxID greater than 4096", func() {
			trunks := `[ {"minID": 10, "maxID": 12}, {"minID": 1, "maxID": 5000} ]`
			It("testSplitVlanIds method should throw appropriate error", func() {
				testSplitVlanIds(trunks, nil, errors.New("incorrect trunk maxID parameter"), false)
			})
		})

		Context("purge ports with failed interfaces", func() {
			const IFNAME = "eth0"

			conf := fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "ovs",
				"OvnPort": "test-port",
				"bridge": "%s"}`, bridgeName)

			It("DEL removes ports without network namespace", func() {
				firstTargetNs, err := testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					Expect(firstTargetNs.Close()).To(Succeed())
					Expect(testutils.UnmountNS(firstTargetNs)).To(Succeed())
				}()

				secondTargetNs, err := testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					Expect(secondTargetNs.Close()).To(Succeed())
					Expect(testutils.UnmountNS(secondTargetNs)).To(Succeed())
				}()

				// Create two ports for two separate target namespaces.
				firstResult := attach(firstTargetNs, conf, IFNAME, "", "test-port-1")
				secondResult := attach(secondTargetNs, conf, IFNAME, "", "test-port-2")

				// Remove the host interface of the first port. This makes the
				// port faulty. Our test should remove the interfaces of this
				// port, but not the interfaces of the second.
				firstHostIface := firstResult.Interfaces[0]
				err = ip.DelLinkByName(firstHostIface.Name)
				Expect(err).NotTo(HaveOccurred())

				// It takes a short while for OVS to notice that we removed the
				// interface. Sometimes the test is faster. We therefore wait
				// up to 1 second for the interface to fail.
				waitForIfaceError(firstHostIface.Name, 10, 100*time.Millisecond)

				secondHostIface := secondResult.Interfaces[0]

				args := &skel.CmdArgs{
					ContainerID: "dummy",
					IfName:      IFNAME,
					StdinData:   []byte(conf),
				}
				err = cmdDelWithArgs(args, func() error {
					return CmdDel(args)
				})
				Expect(err).NotTo(HaveOccurred())

				output, err := exec.Command("ovs-vsctl", "--column=name", "find", "Interface", fmt.Sprintf("name=%s", firstHostIface.Name)).CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(Equal(""), "Faulty OVS interface should have been removed")

				output, err = exec.Command("ovs-vsctl", "--column=name", "find", "Port", fmt.Sprintf("name=%s", firstHostIface.Name)).CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(Equal(""), "Port with faulty OVS interface should have been removed")

				output, err = exec.Command("ovs-vsctl", "--column=name", "find", "Interface", fmt.Sprintf("name=%s", secondHostIface.Name)).CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(
					ContainSubstring(secondHostIface.Name), "Healthy OVS interface should have been kept")

				output, err = exec.Command("ovs-vsctl", "--column=name", "find", "Port", fmt.Sprintf("name=%s", secondHostIface.Name)).CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(
					ContainSubstring(secondHostIface.Name), "OVS port with healthy interface should have been kept")
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

func cmdAddWithArgs(args *skel.CmdArgs, f func() error) (cnitypes.Result, []byte, error) {
	return testutils.CmdAdd(args.Netns, args.ContainerID, args.IfName, args.StdinData, f)
}

func cmdDelWithArgs(args *skel.CmdArgs, f func() error) error {
	return testutils.CmdDel(args.Netns, args.ContainerID, args.IfName, f)
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

func waitForIfaceError(iface string, tries int, delay time.Duration) {
	for i := 0; i < tries; i++ {
		output, err := exec.Command("ovs-vsctl", "--column=error", "find", "Interface", fmt.Sprintf("name=%s", iface)).CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		if strings.Contains(string(output), fmt.Sprintf("could not open network device %s", iface)) {
			return
		}
		time.Sleep(delay)
	}
	Fail(fmt.Sprintf("%s failed to reach error status after %d tries", iface, tries))
}
