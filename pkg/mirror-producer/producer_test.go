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
	"strings"

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

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

type MirrorNet040 struct {
	CNIVersion    string                 `json:"cniVersion"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Bridge        string                 `json:"bridge"`
	Mirrors       []*types.Mirror        `json:"mirrors"`
	RawPrevResult map[string]interface{} `json:"prevResult,omitempty"`
	PrevResult    types040.Result        `json:"-"`
}

type MirrorNetCurrent struct {
	CNIVersion    string                 `json:"cniVersion"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Bridge        string                 `json:"bridge"`
	Mirrors       []*types.Mirror        `json:"mirrors"`
	RawPrevResult map[string]interface{} `json:"prevResult,omitempty"`
	PrevResult    current.Result         `json:"-"`
}

type SelectPort string

const (
	SelectSrcPort SelectPort = "select_src_port"
	SelectDstPort SelectPort = "select_dst_port"
)

const bridgeName = "bridge-mir-prod"
const vlanID = 100
const IFNAME1 = "eth0"
const IFNAME2 = "eth1"

var _ = BeforeSuite(func() {
	output, err := exec.Command("ovs-vsctl", "show").CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Open vSwitch is not available, if you have it installed and running, try to run tests with `sudo -E`: %v", string(output[:]))
})

var _ = AfterSuite(func() {
	exec.Command("ovs-vsctl", "del-br", "--if-exists", bridgeName).Run()
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
			mirrorDb, err := getMirrorAttribute(mirror.Name, "name")
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
		portUUID := getPortUUIDFromResult(r)

		// if both 'select_src_port' and 'select_dst_port' contains only 'portUUID',
		// cmdDel will destroy the mirror, otherwise it will remove that specific uuid from that mirror.
		// However, cmdDel can remove a mirror only if also 'output_port' is empty!
		var removableMirrors []string

		By("Creating a list with all mirrors that should be removed by cmdDel")
		for _, mirror := range mirrors {
			// Obtaining 'select_*' ports of 'mirror'
			srcPorts, err := getMirrorSrcPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())
			dstPorts, err := getMirrorDstPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())
			// Obtaining 'output_port' of 'mirror'
			outputPorts, err := getMirrorOutputPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())

			if len(outputPorts) == 0 && onlyContainsOrEmpty(srcPorts, portUUID) && onlyContainsOrEmpty(dstPorts, portUUID) {
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
			if containsElement(removableMirrors, mirror.Name) {
				By(fmt.Sprintf("Checking that mirror %s is no longer in ovsdb", mirror.Name))
				exists, err := isMirrorExists(mirror.Name)
				Expect(err).NotTo(HaveOccurred())
				// mirror must be removed by cmdDel
				Expect(exists).To(Equal(false))
			} else {
				if mirror.Ingress {
					By(fmt.Sprintf("Checking that mirror %s doesn't have portUUID %s in its 'select_src_port'", mirror.Name, portUUID))
					srcPorts, err := getMirrorSrcPorts(mirror.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(srcPorts).NotTo(ContainElement(portUUID))
				}

				if mirror.Egress {
					By(fmt.Sprintf("Checking that mirror %s doesn't have portUUID %s in its 'select_dst_port'", mirror.Name, portUUID))
					dstPorts, err := getMirrorDstPorts(mirror.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(dstPorts).NotTo(ContainElement(portUUID))
				}
			}
		}
	}

	testAdd := func(conf string, mirrors []types.Mirror, pluginPrevResult *current.Result, ifName string, targetNs ns.NetNS) (string, cnitypes.Result) {
		confMirror, r, err := add(version, conf, pluginPrevResult, ifName, targetNs)

		Expect(err).NotTo(HaveOccurred())

		By("Checking mirror ports")
		checkPortsInMirrors(mirrors, r)

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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, targetNs)
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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, targetNs)
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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, targetNs)
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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				portUUID := getPortUUIDFromResult(prevResult1)

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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, targetNs)
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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				confMirror1, result1 := testAdd(conf, mirrors, prevResult1, IFNAME1, targetNs)
				confMirror2, result2 := testAdd(conf, mirrors, prevResult2, IFNAME2, targetNs)
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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				confMirror1, result1 := testAdd(conf, mirrors, prevResult1, IFNAME1, targetNs)
				confMirror2, result2 := testAdd(conf, mirrors, prevResult2, IFNAME2, targetNs)
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
			mirrorsJSONStr, err := toJSONString(mirrors)
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
				confMirror, result := testAdd(conf, mirrors, prevResult, IFNAME1, targetNs)
				testCheck(confMirror, result, IFNAME1, targetNs)

				By("create a consumer interface and add its port via 'ovs-vsctl' to fill mirror 'output_port'")
				r2 := createInterfaces(IFNAME2, targetNs)
				portUUID := getPortUUIDFromResult(r2)
				addOutputPortToMirror(portUUID, mirrors[0].Name)

				By("run DEL command of ovs-mirror-producer")
				testDel(confMirror, mirrors, result, IFNAME1, targetNs)

				By("check results: mirror still exists")
				exists, err := isMirrorExists(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(Equal(true))
				By("check results: 'select_src_port*' and 'select_dst_port' must be empty")
				srcPorts, err := getMirrorSrcPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(srcPorts).To(BeEmpty())
				dstPorts, err := getMirrorDstPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPorts).To(BeEmpty())
				By("check results: 'output_port' must be unchanged")
				outputs, err := getMirrorOutputPorts(mirrors[0].Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(outputs).To(ContainElement(portUUID))
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
	interfacesJSONStr, err := toJSONString(pluginPrevResult.Interfaces)
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

// function to get portUUID from a cnitypes.Result object
func getPortUUIDFromResult(r cnitypes.Result) string {
	resultMirror, err := current.GetResult(r)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(resultMirror.Interfaces)).To(Equal(2))

	// This plugin must return the same interfaces of the previous one in the chain (ovs-cni plugin),
	// because it doesn't modify interfaces, but it only updates ovsdb.
	By("Checking that result interfaces are equal to those returned by ovs-cni plugin")
	hostIface := resultMirror.Interfaces[0]
	contIface := resultMirror.Interfaces[1]
	Expect(resultMirror.Interfaces[0]).To(Equal(hostIface))
	Expect(resultMirror.Interfaces[1]).To(Equal(contIface))

	By("Getting port uuid for the hostIface")
	portUUID, err := getPortUUIDByName(hostIface.Name)
	Expect(err).NotTo(HaveOccurred())
	return portUUID
}

// function that extracts ports from results and check if every mirror contains those port UUIDs.
// Since it's not possibile to have mirrors without both ingress and egress,
// it's enough finding the port in either ingress or egress.
func checkPortsInMirrors(mirrors []types.Mirror, results ...cnitypes.Result) bool {
	// build an empty slice of port UUIDs
	var portUUIDs = make([]string, 0)
	for _, r := range results {
		portUUID := getPortUUIDFromResult(r)
		portUUIDs = append(portUUIDs, portUUID)
	}

	for _, mirror := range mirrors {
		By(fmt.Sprintf("Checking that mirror %s is in ovsdb", mirror.Name))
		mirrorNameOvsdb, err := getMirrorAttribute(mirror.Name, "name")
		Expect(err).NotTo(HaveOccurred())
		Expect(mirrorNameOvsdb).To(Equal(mirror.Name))

		if mirror.Ingress {
			By(fmt.Sprintf("Checking that mirror %s has all ports created by ovs-cni plugin in select_src_port column", mirror.Name))
			srcPorts, err := getMirrorSrcPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())
			for _, portUUID := range portUUIDs {
				Expect(srcPorts).To(ContainElement(portUUID))
			}
		}

		if mirror.Egress {
			By(fmt.Sprintf("Checking that mirror %s has all ports created by ovs-cni plugin in select_dst_port column", mirror.Name))
			dstPorts, err := getMirrorDstPorts(mirror.Name)
			Expect(err).NotTo(HaveOccurred())
			for _, portUUID := range portUUIDs {
				Expect(dstPorts).To(ContainElement(portUUID))
			}
		}
	}
	return true
}

// function to check if a mirror exists by its name
func isMirrorExists(name string) (bool, error) {
	output, err := exec.Command("ovs-vsctl", "find", "Mirror", "name="+name).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check if mirror exists: %v", string(output[:]))
	}
	return len(output) > 0, nil
}

// function to get a portUUID by its name
func getPortUUIDByName(name string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "get", "Port", name, "_uuid").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get port uuid by name: %v", string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}

// function to get a mirror attribute
func getMirrorAttribute(mirrorName, attributeName string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "get", "Mirror", mirrorName, attributeName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get mirror attribute: %v", string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}

// utility function to get either 'select_src_port' or 'select_dst_port of a mirror
func getMirrorPorts(mirrorName string, attributeName SelectPort) ([]string, error) {
	output, err := getMirrorAttribute(mirrorName, string(attributeName))
	if err != nil {
		return make([]string, 0), fmt.Errorf("failed to get mirror %s ports: %v", mirrorName, string(output[:]))
	}

	// convert into a string, then remove "[" and "]" characters
	stringOutput := output[1 : len(output)-1]

	if stringOutput == "" {
		// if "stringOutput" is an empty string,
		// simply returns a new empty []string (in this way len == 0)
		return make([]string, 0), nil
	}

	// split the string by ", " to get individual uuids in a []string
	outputLines := strings.Split(stringOutput, ", ")
	return outputLines, nil
}

// function to get 'select_src_port' of a mirror as a string slice
func getMirrorSrcPorts(mirrorName string) ([]string, error) {
	return getMirrorPorts(mirrorName, "select_src_port")
}

// function to get 'select_dst_port' of a mirror as a string slice
func getMirrorDstPorts(mirrorName string) ([]string, error) {
	return getMirrorPorts(mirrorName, "select_dst_port")
}

// function to get 'output_port' of a mirror as a string slice
func getMirrorOutputPorts(mirrorName string) ([]string, error) {
	output, err := exec.Command("ovs-vsctl", "get", "Mirror", mirrorName, "output_port").CombinedOutput()
	if err != nil {
		return make([]string, 0), fmt.Errorf("failed to get mirror %s output_port: %v", mirrorName, string(output[:]))
	}

	// convert into a string removing the "\n" character at the end
	stringOutput := string(output[0 : len(output)-1])

	// outport_port field behaviour is quite inconsistent, because:
	// - if in empty, it returns an empty slice "[]" with a "\n" character at the end,
	// - otherwise, it returns a string with a "\n" character at the end
	if stringOutput == "[]" {
		// if "stringOutput" is an empty string,
		// simply returns a new empty []string (in this way len == 0)
		return make([]string, 0), nil
	}
	return []string{stringOutput}, nil
}

// function to add portUUID as 'output_port' to a specific mirror via 'ovs-vsctl'
func addOutputPortToMirror(portUUID, mirrorName string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "set", "Mirror", mirrorName, "output_port="+portUUID).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to set output_port for mirror %s with UUID %s: %v", mirrorName, portUUID, string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}

// convert input into a JSON string
func toJSONString(input interface{}) (string, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// function to check if a list of strings contains only 'el' element or is empty
func onlyContainsOrEmpty(list []string, el string) bool {
	if len(list) > 1 {
		// because it has more elements, so 'el' cannot be the only one
		return false
	}
	if len(list) == 0 {
		// in empty
		return true
	}
	// otherwise check if the only element in 'list' is equals to 'el'
	return containsElement(list, el)
}

// function that returns true if a list of strings contains a string element
func containsElement(list []string, el string) bool {
	for _, l := range list {
		if l == el {
			return true
		}
	}
	return false
}
