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

const bridgeName = "bridge-mir-cons"
const vlanID = 100
const IFNAME1 = "eth0"
const IFNAME2 = "eth1"
const ovsPortOwner = "ovs-cni.network.kubevirt.io"

var _ = BeforeSuite(func() {
	output, err := exec.Command("ovs-vsctl", "show").CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Open vSwitch is not available, if you have it installed and running, try to run tests with `sudo -E`: %v", string(output[:]))
})

var _ = AfterSuite(func() {
	exec.Command("ovs-vsctl", "del-br", "--if-exists", bridgeName).Run()
})

var _ = Describe("CNI mirror-consumer 0.3.0", func() { testFunc("0.3.0") })
var _ = Describe("CNI mirror-consumer 0.3.1", func() { testFunc("0.3.1") })
var _ = Describe("CNI mirror-consumer 0.4.0", func() { testFunc("0.4.0") })
var _ = Describe("CNI mirror-consumer 1.0.0", func() { testFunc("1.0.0") })

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
			ContainerID: "dummy-mir-cons",
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
			ContainerID: "dummy-mir-cons",
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
			ContainerID: "dummy-mir-cons",
			Netns:       targetNs.Path(),
			IfName:      ifName,
			StdinData:   []byte(conf),
		}

		// get portUUID from result
		portUUID := GetPortUUIDFromResult(r)

		// if 'output_port' contains only portUUID and both 'select_src_port' and 'select_dst_port' are empty,
		// cmdDel will destroy the mirror, otherwise it will remove the portUUID from 'output_port'
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

			if OnlyContainsOrEmpty(outputPorts, portUUID) && len(srcPorts) == 0 && len(dstPorts) == 0 {
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
				By(fmt.Sprintf("Checking that mirror %s doesn't have portUUID %s in its 'output_port'", mirror.Name, portUUID))
				outputPorts, err := GetMirrorOutputPorts(mirror.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(outputPorts).NotTo(ContainElement(portUUID))
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
		Context("as consumer (output_port in ovsdb)", func() {
			mirrors := []types.Mirror{
				{
					Name: "mirror-cons",
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-consumer",
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

				By("run ovs-mirror-consumer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)
			})
		})
	})

	Context("adding host port to multiple mirrors", func() {
		Context("as consumer (output_port in ovsdb)", func() {
			mirrors := []types.Mirror{
				{
					Name: "mir-cons1",
				},
				{
					Name: "mir-cons2",
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-consumer",
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

				By("run ovs-mirror-consumer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)
			})
		})
	})

	Context("adding multiple ports to a single mirror", func() {
		Context("as consumer (output_port in ovsdb)", func() {
			mirrors := []types.Mirror{
				{
					Name: "mir-cons1",
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-consumer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("shouldn't complete ADD, because you cannot override the configuration of an existing mirror", func() {
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces/ports using ovs-cni plugin")
				prevResult1 := createInterfaces(IFNAME1, targetNs)
				prevResult2 := createInterfaces(IFNAME2, targetNs)

				By("run ovs-mirror-consumer ADD command for the first port")
				_, _ = testAdd(conf, mirrors, prevResult1, IFNAME1, false, targetNs)

				By("run ovs-mirror-consumer ADD command for the second port expecting an error")
				_, _, err := add(version, conf, prevResult2, IFNAME2, targetNs)
				Expect(err).To(HaveOccurred())

				By("verify the error message")
				portUUID2 := GetPortUUIDFromResult(prevResult2)
				errorMessage := fmt.Sprintf("cannot attach port %s to mirror %s "+
					"because there is already another port. Error:", portUUID2, mirrors[0].Name)
				Expect(err.Error()).To(ContainSubstring(errorMessage))
			})
		})
	})

	Context("adding a mirror with both producer and consumer configuration", func() {
		Context("('output_port', 'select_src_port' and 'select_dst_port' defined with valid portUUIDs)", func() {
			mirrors := []types.Mirror{
				{
					Name: "mirror-cons",
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-consumer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("shouldn't be removed after calling cmdDel by this plugin, because it contains 'select_src_port' and 'select_dst_port' configured by a producer", func() {
				// This is very important:
				// cmdDel of both mirror-consumer and mirror-consumer plugins is able to cleanup a mirror
				// without a useful configuration (all traffic outputs and inputs are undefined).
				// However, they can remove a mirror only if both
				// 'output_port', 'select_src_port' and 'select_dst_port' are empty.
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces using ovs-cni plugin")
				prevResult := createInterfaces(IFNAME1, targetNs)

				By("run ovs-mirror-consumer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)

				By("create a producer interface and add its port via 'ovs-vsctl' to fill both 'select_src_port' and 'select_dst_port'")
				r2 := createInterfaces(IFNAME2, targetNs)
				portUUID := GetPortUUIDFromResult(r2)
				AddSelectPortToMirror(portUUID, mirrors[0].Name, true, true)

				By("run DEL command of ovs-mirror-consumer")
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)

				By("check results: mirror still exists")
				exists, err := IsMirrorExists(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(Equal(true))
				By("check results: 'select_src_port' and 'select_dst_port' must be unchanged")
				srcPorts, err := GetMirrorSrcPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(srcPorts).To(ContainElement(portUUID))
				dstPorts, err := GetMirrorDstPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPorts).To(ContainElement(portUUID))
				By("check results: 'output_port' must be empty")
				outputs, err := GetMirrorOutputPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(outputs).To(BeEmpty())
			})
		})
	})

	Context("adding multiple mirrors with both producer and consumer configuration", func() {
		Context("('output_port' and either 'select_src_port' or 'select_dst_port' defined with valid portUUIDs)", func() {
			mirrors := []types.Mirror{
				{
					Name: "mirror-cons1",
				},
				{
					Name: "mirror-cons2",
				},
			}
			mirrorsJSONStr, err := ToJSONString(mirrors)
			Expect(err).NotTo(HaveOccurred())

			conf := fmt.Sprintf(`{
				"cniVersion": "%s",
				"name": "mynet",
				"type": "ovs-mirror-consumer",
				"bridge": "%s",
				"mirrors": %s
			}`, version, bridgeName, mirrorsJSONStr)

			It("shouldn't be removed after calling cmdDel by this plugin, because it contains either 'select_src_port' or 'select_dst_port' configured by a producer", func() {
				// This is very important:
				// cmdDel of both mirror-consumer and mirror-consumer plugins is able to cleanup a mirror
				// without a useful configuration (all traffic outputs and inputs are undefined).
				// However, they can remove a mirror only if both
				// 'output_port', 'select_src_port' and 'select_dst_port' are empty.
				targetNs := newNS()
				defer func() {
					closeNS(targetNs)
				}()

				By("create interfaces using ovs-cni plugin")
				prevResult := createInterfaces(IFNAME1, targetNs)

				By("run ovs-mirror-consumer passing prevResult")
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, false, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)

				By("create a producer interface and get its portUUID")
				r2 := createInterfaces(IFNAME2, targetNs)
				portUUID := GetPortUUIDFromResult(r2)
				By(fmt.Sprintf("update mirror %s adding portUUID as 'select_src_port'", mirrors[0].Name))
				AddSelectPortToMirror(portUUID, mirrors[0].Name, true, false)
				By(fmt.Sprintf("update mirror %s adding portUUID as 'select_dst_port'", mirrors[1].Name))
				AddSelectPortToMirror(portUUID, mirrors[1].Name, false, true)

				By("run DEL command of ovs-mirror-consumer")
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)

				By("check results: mirror still exists")
				exists, err := IsMirrorExists(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(Equal(true))
				By("check results: 'select_src_port' and 'select_dst_port' must be unchanged")
				srcPorts, err := GetMirrorSrcPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(srcPorts).To(ContainElement(portUUID))
				dstPorts, err := GetMirrorDstPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPorts).To(BeEmpty())
				By("check results: 'output_port' must be empty")
				outputs, err := GetMirrorOutputPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(outputs).To(BeEmpty())

				By("check results: mirror still exists")
				exists, err = IsMirrorExists(mirrors[1].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(Equal(true))
				By("check results: 'select_src_port' and 'select_dst_port' must be unchanged")
				srcPorts, err = GetMirrorSrcPorts(mirrors[1].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(srcPorts).To(BeEmpty())
				dstPorts, err = GetMirrorDstPorts(mirrors[1].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPorts).To(ContainElement(portUUID))
				By("check results: 'output_port' must be empty")
				outputs, err = GetMirrorOutputPorts(mirrors[1].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(outputs).To(BeEmpty())
			})
		})
	})

	Context("when there are empty mirrors in ovsdb", func() {
		Context("that are owned by ovs-cni,", func() {
			Context("creating a new mirror", func() {
				mirrors := []types.Mirror{
					{
						Name: "mirror-cons",
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-consumer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirCons1", "emptyMirCons2"}

				It("should remove those that are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("manually create empty mirrors owned by ovs-cni")
					CreateEmptyMirrors(bridgeName, emptyMirrors, ovsPortOwner)

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-consumer passing prevResult")
					confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, true, targetNs)

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
						Name: "mirror-cons",
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-consumer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirCons1", "emptyMirCons2"}

				It("should remove those that are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-consumer passing prevResult")
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
						Name: "mirror-cons",
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-consumer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirCons1", "emptyMirCons2"}

				It("should NOT remove those are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("manually create an empty mirror WITHOUT specifying an owner")
					CreateEmptyMirrors(bridgeName, emptyMirrors, "")

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-consumer passing prevResult")
					confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, true, targetNs)

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
						Name: "mirror-cons",
					},
				}
				mirrorsJSONStr, err := ToJSONString(mirrors)
				Expect(err).NotTo(HaveOccurred())

				conf := fmt.Sprintf(`{
					"cniVersion": "%s",
					"name": "mynet",
					"type": "ovs-mirror-consumer",
					"bridge": "%s",
					"mirrors": %s
				}`, version, bridgeName, mirrorsJSONStr)

				emptyMirrors := []string{"emptyMirCons1", "emptyMirCons2"}

				It("should NOT remove those are in the current bridge", func() {
					targetNs := newNS()
					defer func() {
						closeNS(targetNs)
					}()

					By("create interfaces using ovs-cni plugin")
					prevResult := createInterfaces(IFNAME1, targetNs)

					By("run ovs-mirror-consumer passing prevResult")
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
	By("Building prevResult to pass it as input to mirror-consumer plugin")
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
		ContainerID: "dummy-mir-cons",
		Netns:       targetNs.Path(),
		IfName:      ifName,
		StdinData:   []byte(confMirror),
	}

	By("Calling ADD command for mirror-consumer plugin")
	r, _, err := cmdAddWithArgs(argsMirror, func() error {
		return CmdAdd(argsMirror)
	})

	return confMirror, r, err
}
