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
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types040 "github.com/containernetworking/cni/pkg/types/040"
	current "github.com/containernetworking/cni/pkg/types/100"
	cniversion "github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	plugin "github.com/k8snetworkplumbingwg/ovs-cni/pkg/plugin"

	. "github.com/k8snetworkplumbingwg/ovs-cni/pkg/testhelpers"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

const bridgeName = "bridge-mir-prod"
const vlanID = 100
const IFNAME1 = "eth0"
const IFNAME2 = "eth1"
const ovsPortOwner = "ovs-cni.network.kubevirt.io"

var _ = BeforeSuite(func() {
	output, err := exec.Command("ovs-vsctl", "show").CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Open vSwitch is not available, if you have it installed and running, try to run tests with `sudo -E`: %v", string(output[:]))
})

var _ = AfterSuite(func() {
	output, err := exec.Command("ovs-vsctl", "del-br", "--if-exists", bridgeName).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Cleanup of the bridge failed: %v", string(output[:]))
})

var _ = Describe("CNI mirror-producer 0.3.0", func() { testFunc("0.3.0") })
var _ = Describe("CNI mirror-producer 0.3.1", func() { testFunc("0.3.1") })
var _ = Describe("CNI mirror-producer 0.4.0", func() { testFunc("0.4.0") })
var _ = Describe("CNI mirror-producer 1.0.0", func() { testFunc("1.0.0") })

var testFunc = func(version string) {
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

	createInterfaces := func(ifName string, targetNs ns.NetNS) *current.Result {
		confplugin := fmt.Sprintf(`{
			"cniVersion": "%s",
			"name": "mynet",
			"type": "ovs",
			"bridge": "%s",
			"vlan": %d
		}`, version, bridgeName, vlanID)
		args := &skel.CmdArgs{
			ContainerID: "dummy-mir-prod",
			Netns:       targetNs.Path(),
			IfName:      ifName,
			StdinData:   []byte(confplugin),
		}

		By("Calling ADD command of ovs-cni plugin to create interfaces")
		resPlugin, _, err := cmdAddWithArgs(args, func() error {
			return plugin.CmdAdd(args)
		})
		Expect(err).NotTo(HaveOccurred())

		By("Checking that result of ADD command of ovs-cni plugin is in expected format")
		resultPlugin, err := current.GetResult(resPlugin)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(resultPlugin.Interfaces)).To(Equal(2))
		Expect(len(resultPlugin.IPs)).To(Equal(0))

		return resultPlugin
	}

	testCheck := func(conf string, r cnitypes.Result, ifName string, targetNs ns.NetNS) {
		if checkSupported, _ := cniversion.GreaterThanOrEqualTo(version, "0.4.0"); !checkSupported {
			return
		}

		args := &skel.CmdArgs{
			ContainerID: "dummy-mir-prod",
			Netns:       targetNs.Path(),
			IfName:      ifName,
			StdinData:   []byte(conf),
		}

		By("Calling CHECK command")
		moreThan100, err := cniversion.GreaterThanOrEqualTo(version, "1.0.0")
		Expect(err).NotTo(HaveOccurred())
		var confString []byte
		if moreThan100 {
			netconf := &MirrorNetCurrent{}
			err = json.Unmarshal([]byte(conf), &netconf)
			Expect(err).NotTo(HaveOccurred())
			result, err := current.GetResult(r)
			Expect(err).NotTo(HaveOccurred())
			netconf.PrevResult = *result

			var data bytes.Buffer
			err = result.PrintTo(&data)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			err = json.Unmarshal(data.Bytes(), &raw)
			Expect(err).NotTo(HaveOccurred())
			netconf.RawPrevResult = raw

			confString, err = json.Marshal(netconf)
			Expect(err).NotTo(HaveOccurred())
		} else {
			netconf := &MirrorNet040{}
			err = json.Unmarshal([]byte(conf), &netconf)
			Expect(err).NotTo(HaveOccurred())
			result, err := types040.GetResult(r)
			Expect(err).NotTo(HaveOccurred())
			netconf.PrevResult = *result

			var data bytes.Buffer
			err = result.PrintTo(&data)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			err = json.Unmarshal(data.Bytes(), &raw)
			Expect(err).NotTo(HaveOccurred())
			netconf.RawPrevResult = raw

			confString, err = json.Marshal(netconf)
			Expect(err).NotTo(HaveOccurred())
		}

		args.StdinData = confString

		err = cmdCheckWithArgs(args, func() error {
			return CmdCheck(args)
		})
		Expect(err).NotTo(HaveOccurred())
	}

	testDel := func(conf string, mirrors []types.Mirror, r cnitypes.Result, ifName string, targetNs ns.NetNS) {
		By("Checking that mirrors are still in ovsdb")
		for _, mirror := range mirrors {
			mirrorDb, err := GetMirrorAttribute(mirror.Name, "name")
			Expect(err).NotTo(HaveOccurred())
			Expect(mirrorDb).To(Equal(mirror.Name))
		}

		args := &skel.CmdArgs{
			ContainerID: "dummy-mir-prod",
			Netns:       targetNs.Path(),
			IfName:      ifName,
			StdinData:   []byte(conf),
		}

		// get portUUID from result
		portUUID := GetPortUUIDFromResult(r)

		// if both 'select_src_port' and 'select_dst_port' contains only 'portUUID',
		// cmdDel will destroy the mirror, otherwise it will remove that specific uuid from that mirror.
		// However, cmdDel can remove a mirror only if also 'output_port' is empty!
		var removableMirrors []string

		By("Creating a list with all mirrors that should be removed by cmdDel")
		for _, mirror := range mirrors {
			// Obtaining 'select_*' ports of 'mirror'
			srcPorts, err := GetMirrorSrcPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())
			dstPorts, err := GetMirrorDstPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())
			// Obtaining 'output_port' of 'mirror'
			outputPorts, err := GetMirrorOutputPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())

			if len(outputPorts) == 0 && OnlyContainsOrEmpty(srcPorts, portUUID) && OnlyContainsOrEmpty(dstPorts, portUUID) {
				// this mirror will be removed by cmdDel
				removableMirrors = append(removableMirrors, mirror.Name)
			}
		}

		By("Calling DEL command")
		err := cmdDelWithArgs(args, func() error {
			return CmdDel(args)
		})
		Expect(err).NotTo(HaveOccurred())

		By("Checking mirrors after DEL command")
		for _, mirror := range mirrors {
			if ContainsElement(removableMirrors, mirror.Name) {
				By(fmt.Sprintf("Checking that mirror %s is no longer in ovsdb", mirror.Name))
				exists, err := IsMirrorExists(mirror.Name)
				Expect(err).NotTo(HaveOccurred())
				// mirror must be removed by cmdDel
				Expect(exists).To(Equal(false))
			} else {
				if mirror.Ingress {
					By(fmt.Sprintf("Checking that mirror %s doesn't have portUUID %s in its 'select_src_port'", mirror.Name, portUUID))
					srcPorts, err := GetMirrorSrcPorts(mirror.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(srcPorts).NotTo(ContainElement(portUUID))
				}

				if mirror.Egress {
					By(fmt.Sprintf("Checking that mirror %s doesn't have portUUID %s in its 'select_dst_port'", mirror.Name, portUUID))
					dstPorts, err := GetMirrorDstPorts(mirror.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(dstPorts).NotTo(ContainElement(portUUID))
				}
			}
		}
	}

	testAdd := func(conf string, mirrors []types.Mirror, pluginPrevResult *current.Result, ifName string, hasExternalOwner bool, targetNs ns.NetNS) (string, cnitypes.Result) {
		confMirror, r, err := add(version, conf, pluginPrevResult, ifName, targetNs)

		Expect(err).NotTo(HaveOccurred())

		By("Checking mirror ports")
		CheckPortsInMirrors(mirrors, hasExternalOwner, ovsPortOwner, r)

		return confMirror, r
	}

	Context("adding host port to a mirror", func() {
		Context("as both ingress and egress (select_src_port and select_dst_port in ovsdb)", func() {
			mirrors := []types.Mirror{
				{
					Name:    "mirror-prod",
					Ingress: true,
					Egress:  true,
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("should successfully complete ADD, CHECK and DEL commands", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces using ovs-cni plugin")
				prevResult := createInterfaces(IFNAME1, targetNs)

				By("run ovs-mirror-producer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)
			})
		})

		Context("as ingress, but not egress (only select_src_port in ovsdb)", func() {
			mirrors := []types.Mirror{
				{
					Name:    "mirror-prod",
					Ingress: true,
					// Egress:  false (if omitted, 'Egress' is false by default)
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("should successfully complete ADD, CHECK and DEL commands", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces using ovs-cni plugin")
				prevResult := createInterfaces(IFNAME1, targetNs)

				By("run ovs-mirror-producer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)
			})
		})

		Context("as egress, but not ingress (only select_dst_port in ovsdb)", func() {
			mirrors := []types.Mirror{
				{
					Name:    "mirror-prod",
					Ingress: false, // (if omitted, 'Ingress' is false by default)
					Egress:  true,
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("should successfully complete ADD, CHECK and DEL commands", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces using ovs-cni plugin")
				prevResult := createInterfaces(IFNAME1, targetNs)

				By("run ovs-mirror-producer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)
			})
		})

		Context("without both ingress and egress (select_src_port and select_dst_port in ovsdb)", func() {
			mirrorName := "mir-prod1"
			mirrors := []types.Mirror{
				{
					Name:    mirrorName,
					Ingress: false,
					// Egress omitted, but false by default
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("should FAIL with ADD command", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces/ports using ovs-cni plugin")
				prevResult1 := createInterfaces(IFNAME1, targetNs)
				portUUID := GetPortUUIDFromResult(prevResult1)

				By("run ovs-mirror-producer ADD command")
				// call 'add' instead of 'testAdd' because we want the result of cmdAdd without additional check
				_, _, err := add(version, conf, prevResult1, IFNAME1, targetNs)
				Expect(err).To(HaveOccurred())

				By("verify the error message")
				errorMessage := fmt.Sprintf("cannot attach port %s to mirror %s: "+
					"a mirror producer must have either a ingress or an egress or both", portUUID, mirrorName)
				Expect(err.Error()).To(Equal(errorMessage))
			})
		})
	})

	Context("adding host port to multiple mirrors", func() {
		Context("with different ingress and egress configurations", func() {
			mirrors := []types.Mirror{
				{
					Name:    "mir-prod1",
					Ingress: true,
					Egress:  true,
				},
				{
					Name:    "mir-prod2",
					Ingress: false,
					Egress:  true,
				},
				{
					Name:    "mir-prod3",
					Ingress: true,
					Egress:  false,
				},
				{
					Name:    "mir-prod4",
					Ingress: true,
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("should successfully complete ADD, CHECK and DEL commands", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces using ovs-cni plugin")
				prevResult := createInterfaces(IFNAME1, targetNs)

				By("run ovs-mirror-producer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)
			})
		})
	})

	Context("adding multiple ports to a single mirror", func() {
		Context("as both ingress and egress (select_src_port and select_dst_port in ovsdb)", func() {
			mirrors := []types.Mirror{
				{
					Name:    "mir-prod1",
					Ingress: true,
					Egress:  true,
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("should successfully complete ADD, CHECK and DEL commands", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces/ports using ovs-cni plugin")
				prevResult1 := createInterfaces(IFNAME1, targetNs)
				prevResult2 := createInterfaces(IFNAME2, targetNs)

				By("run ovs-mirror-producer passing prevResult")
				confMirror1, result1 := testAdd(conf, mirrors, prevResult1, IFNAME1, false, targetNs)
				confMirror2, result2 := testAdd(conf, mirrors, prevResult2, IFNAME2, false, targetNs)
				testCheck(confMirror1, result1, IFNAME1, targetNs)
				testCheck(confMirror2, result2, IFNAME2, targetNs)

				By("run ovs-mirror-producer to delete mirrors")
				testDel(confMirror1, mirrors, result1, IFNAME1, targetNs)
				testDel(confMirror2, mirrors, result2, IFNAME2, targetNs)
			})
		})
	})

	Context("adding multiple ports to multiple mirrors", func() {
		Context("with different ingress and egress configurations", func() {
			mirrors := []types.Mirror{
				{
					Name:    "mir-prod1",
					Ingress: true,
					Egress:  true,
				},
				{
					Name:    "mir-prod2",
					Ingress: false,
					Egress:  true,
				},
				{
					Name:    "mir-prod3",
					Ingress: true,
					Egress:  false,
				},
				{
					Name:    "mir-prod4",
					Ingress: true,
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("should successfully complete ADD, CHECK and DEL commands", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces/ports using ovs-cni plugin")
				prevResult1 := createInterfaces(IFNAME1, targetNs)
				prevResult2 := createInterfaces(IFNAME2, targetNs)

				By("run ovs-mirror-producer passing prevResult")
				confMirror1, result1 := testAdd(conf, mirrors, prevResult1, IFNAME1, false, targetNs)
				confMirror2, result2 := testAdd(conf, mirrors, prevResult2, IFNAME2, false, targetNs)
				testCheck(confMirror1, result1, IFNAME1, targetNs)
				testCheck(confMirror2, result2, IFNAME2, targetNs)

				By("run ovs-mirror-producer to delete mirrors")
				testDel(confMirror1, mirrors, result1, IFNAME1, targetNs)
				testDel(confMirror2, mirrors, result2, IFNAME2, targetNs)
			})
		})
	})

	Context("adding a mirror with both producer and consumer configuration", func() {
		Context("('output_port', 'select_src_port' and 'select_dst_port' defined with valid portUUIDs)", func() {
			mirrors := []types.Mirror{
				{
					Name:    "mirror-prod",
					Ingress: true,
					Egress:  true,
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-producer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("shouldn't be removed after calling cmdDel by this plugin, because it contains 'output_port' configured by a consumer", func() {
				// This is very important:
				// cmdDel of both mirror-producer and mirror-consumer plugins is able to cleanup a mirror
				// without a useful configuration (all traffic outputs and inputs are undefined).
				// However, they can remove a mirror only if both
				// 'output_port', 'select_src_port' and 'select_dst_port' are empty.
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces using ovs-cni plugin")
				prevResult := createInterfaces(IFNAME1, targetNs)

				By("run ovs-mirror-producer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)

				By("create a consumer interface and add its port via 'ovs-vsctl' to fill mirror 'output_port'")
				r2 := createInterfaces(IFNAME2, targetNs)
				portUUID := GetPortUUIDFromResult(r2)
				_, err = AddOutputPortToMirror(portUUID, mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())

				By("run DEL command of ovs-mirror-producer")
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)

				By("check results: mirror still exists")
				exists, err := IsMirrorExists(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(Equal(true))
				By("check results: 'select_src_port*' and 'select_dst_port' must be empty")
				srcPorts, err := GetMirrorSrcPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(srcPorts).To(BeEmpty())
				dstPorts, err := GetMirrorDstPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPorts).To(BeEmpty())
				By("check results: 'output_port' must be unchanged")
				outputs, err := GetMirrorOutputPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(outputs).To(ContainElement(portUUID))
			})
		})
	})

	Context("when there are empty mirrors in ovsdb", func() {
		Context("that are owned by ovs-cni,", func() {
			Context("creating a new mirror", func() {
				mirrors := []types.Mirror{
					{
						Name:    "mirror-prod",
						Ingress: true,
						Egress:  true,
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-producer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirProd1", "emptyMirProd2"}

				It("should remove those that are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("manually create empty mirrors owned by ovs-cni")
					CreateEmptyMirrors(bridgeName, emptyMirrors, ovsPortOwner)

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-producer passing prevResult")
					confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)

					// 'cmdAdd' mirror function calls automatically cleanEmptyMirrors
					// to remove unused mirrors of the bridge

					By("mirrors should not exist anymore")
					CheckEmptyMirrorsExistence(emptyMirrors, false)

					testCheck(confMirror, result, IFNAME1, targetNs)
					testDel(confMirror, mirrors, result, IFNAME1, targetNs)
				})
			})

			Context("deleting a mirror", func() {
				mirrors := []types.Mirror{
					{
						Name:    "mirror-prod",
						Ingress: true,
						Egress:  true,
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-producer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirProd1", "emptyMirProd2"}

				It("should remove those that are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-producer passing prevResult")
					confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
					testCheck(confMirror, result, IFNAME1, targetNs)

					By("manually create empty mirrors owned by ovs-cni")
					CreateEmptyMirrors(bridgeName, emptyMirrors, ovsPortOwner)

					// 'cmdDel' mirror function calls automatically cleanEmptyMirrors
					// to remove unused mirrors of the bridge

					testDel(confMirror, mirrors, result, IFNAME1, targetNs)

					By("mirrors should not exist anymore")
					CheckEmptyMirrorsExistence(emptyMirrors, false)
				})
			})
		})

		Context("that are NOT owned by ovs-cni,", func() {
			Context("creating a new mirror", func() {
				mirrors := []types.Mirror{
					{
						Name:    "mirror-prod",
						Ingress: true,
						Egress:  true,
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-producer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirProd1", "emptyMirProd2"}

				It("should NOT remove those that are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("manually create an empty mirror WITHOUT specifying an owner")
					CreateEmptyMirrors(bridgeName, emptyMirrors, "")

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-producer passing prevResult")
					confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)

					// 'cmdAdd' mirror function calls automatically cleanEmptyMirrors
					// to remove unused mirrors of the bridge

					By("mirrors should still exists")
					CheckEmptyMirrorsExistence(emptyMirrors, true)

					testCheck(confMirror, result, IFNAME1, targetNs)
					testDel(confMirror, mirrors, result, IFNAME1, targetNs)
				})
			})

			Context("deleting a mirror", func() {
				mirrors := []types.Mirror{
					{
						Name:    "mirror-prod",
						Ingress: true,
						Egress:  true,
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-producer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirProd1", "emptyMirProd2"}

				It("should NOT remove those that are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-producer passing prevResult")
					confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
					testCheck(confMirror, result, IFNAME1, targetNs)

					By("manually create an empty mirror WITHOUT specifying an owner")
					CreateEmptyMirrors(bridgeName, emptyMirrors, "")

					// 'cmdDel' mirror function calls automatically cleanEmptyMirrors
					// to remove unused mirrors of the bridge

					testDel(confMirror, mirrors, result, IFNAME1, targetNs)

					By("mirrors should still exists")
					CheckEmptyMirrorsExistence(emptyMirrors, true)
				})
			})
		})
	})
}

func newNS() ns.NetNS {
	targetNs, err := testutils.NewNS()
	Expect(err).NotTo(HaveOccurred())
	return targetNs
}

func closeNS(targetNs ns.NetNS) {
	Expect(targetNs.Close()).To(Succeed())
	Expect(testutils.UnmountNS(targetNs)).To(Succeed())
}

func cmdAddWithArgs(args *skel.CmdArgs, f func() error) (cnitypes.Result, []byte, error) {
	return testutils.CmdAdd(args.Netns, args.ContainerID, args.IfName, args.StdinData, f)
}

func cmdCheckWithArgs(args *skel.CmdArgs, f func() error) error {
	return testutils.CmdCheck(args.Netns, args.ContainerID, args.IfName, args.StdinData, f)
}

func cmdDelWithArgs(args *skel.CmdArgs, f func() error) error {
	return testutils.CmdDel(args.Netns, args.ContainerID, args.IfName, f)
}

// function to call cmdAdd with the right input
func add(version string, conf string, pluginPrevResult *current.Result, ifName string, targetNs ns.NetNS) (string, cnitypes.Result, error) {
	By("Building prevResult to pass it as input to mirror-producer plugin")
	interfacesJSONStr, err := ToJSONString(pluginPrevResult.Interfaces)
	Expect(err).NotTo(HaveOccurred())

	prevResult := fmt.Sprintf(`{
		"cniVersion": "%s",
		"interfaces": %s
	}`, version, interfacesJSONStr)

	// add prevResult to conf (first we need to remove the last character "}"
	// and then concatenate the rest
	confMirror := conf[:len(conf)-1] + ", \"prevResult\": " + prevResult + "\n}"

	argsMirror := &skel.CmdArgs{
		ContainerID: "dummy-mir-prod",
		Netns:       targetNs.Path(),
		IfName:      ifName,
		StdinData:   []byte(confMirror),
	}

	By("Calling ADD command for mirror-producer plugin")
	r, _, err := cmdAddWithArgs(argsMirror, func() error {
		return CmdAdd(argsMirror)
	})

	return confMirror, r, err
}
