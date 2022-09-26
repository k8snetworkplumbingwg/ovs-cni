package testhelpers

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	types040 "github.com/containernetworking/cni/pkg/types/040"
	current "github.com/containernetworking/cni/pkg/types/100"

	ginko "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

// MirrorNet040 struct that represent the network configuration for CNI spec 0.4.0
type MirrorNet040 struct {
	CNIVersion    string                 `json:"cniVersion"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Bridge        string                 `json:"bridge"`
	Mirrors       []*types.Mirror        `json:"mirrors"`
	RawPrevResult map[string]interface{} `json:"prevResult,omitempty"`
	PrevResult    types040.Result        `json:"-"`
}

// MirrorNetCurrent struct that represent the network configuration for CNI spec 1.0.0
type MirrorNetCurrent struct {
	CNIVersion    string                 `json:"cniVersion"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Bridge        string                 `json:"bridge"`
	Mirrors       []*types.Mirror        `json:"mirrors"`
	RawPrevResult map[string]interface{} `json:"prevResult,omitempty"`
	PrevResult    current.Result         `json:"-"`
}

// SelectPort type that represent the kind of select_*_port
type SelectPort string

const (
	// SelectSrcPort const with value "select_src_port"
	// (ports on which arriving packets are selected for mirroring)
	SelectSrcPort SelectPort = "select_src_port"
	// SelectDstPort const with value "select_dst_port"
	// (ports on which departing packets are selected for mirroring)
	SelectDstPort SelectPort = "select_dst_port"
)

// GetPortUUIDFromResult gets portUUID from a cnitypes.Result object
func GetPortUUIDFromResult(r cnitypes.Result) string {
	resultMirror, err := current.GetResult(r)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(len(resultMirror.Interfaces)).To(gomega.Equal(2))

	// Both mirror-producer and mirror-consumer must return the same interfaces of the previous one in the chain (ovs-cni plugin),
	// because they don't modify interfaces, but they only update ovsdb.
	ginko.By("Checking that result interfaces are equal to those returned by ovs-cni plugin")
	hostIface := resultMirror.Interfaces[0]
	contIface := resultMirror.Interfaces[1]
	gomega.Expect(resultMirror.Interfaces[0]).To(gomega.Equal(hostIface))
	gomega.Expect(resultMirror.Interfaces[1]).To(gomega.Equal(contIface))

	ginko.By("Getting port uuid for the hostIface")
	portUUID, err := GetPortUUIDByName(hostIface.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return portUUID
}

// CheckPortsInMirrors extracts ports from results and check if every mirror contains those port UUIDs.
// Since it's not possibile to have mirrors without both ingress and egress,
// it's enough finding the port in either ingress or egress.
// It also verify the mirror owner using 'external_ids' attribute. You can skip this check with hasExternalOwner=true.
func CheckPortsInMirrors(mirrors []types.Mirror, hasExternalOwner bool, ovsPortOwner string, results ...cnitypes.Result) bool {
	// build an empty slice of port UUIDs
	var portUUIDs = make([]string, 0)
	for _, r := range results {
		portUUID := GetPortUUIDFromResult(r)
		portUUIDs = append(portUUIDs, portUUID)
	}

	for _, mirror := range mirrors {
		ginko.By(fmt.Sprintf("Checking that mirror %s is in ovsdb", mirror.Name))
		mirrorNameOvsdb, err := GetMirrorAttribute(mirror.Name, "name")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(mirrorNameOvsdb).To(gomega.Equal(mirror.Name))

		if !hasExternalOwner {
			mirrorExternalIdsOvsdb, err := GetMirrorAttribute(mirror.Name, "external_ids")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(mirrorExternalIdsOvsdb).To(gomega.ContainSubstring("owner=" + ovsPortOwner))
		}

		if mirror.Ingress {
			ginko.By(fmt.Sprintf("Checking that mirror %s has all ports created by ovs-cni plugin in select_src_port column", mirror.Name))
			srcPorts, err := GetMirrorSrcPorts(mirror.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, portUUID := range portUUIDs {
				gomega.Expect(srcPorts).To(gomega.ContainElement(portUUID))
			}
		}

		if mirror.Egress {
			ginko.By(fmt.Sprintf("Checking that mirror %s has all ports created by ovs-cni plugin in select_dst_port column", mirror.Name))
			dstPorts, err := GetMirrorDstPorts(mirror.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, portUUID := range portUUIDs {
				gomega.Expect(dstPorts).To(gomega.ContainElement(portUUID))
			}
		}
	}
	return true
}

// IsMirrorExists checks if a mirror exists by its name
func IsMirrorExists(name string) (bool, error) {
	output, err := exec.Command("ovs-vsctl", "find", "Mirror", "name="+name).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check if mirror exists: %v", string(output[:]))
	}
	return len(output) > 0, nil
}

// GetPortUUIDByName gets a portUUID by its name
func GetPortUUIDByName(name string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "get", "Port", name, "_uuid").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get port uuid by name: %v", string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}

// GetMirrorAttribute gets a mirror attribute
func GetMirrorAttribute(mirrorName, attributeName string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "get", "Mirror", mirrorName, attributeName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get mirror attribute: %v", string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}

// GetMirrorPorts gets either 'select_src_port' or 'select_dst_port of a mirror
func GetMirrorPorts(mirrorName string, attributeName SelectPort) ([]string, error) {
	output, err := GetMirrorAttribute(mirrorName, string(attributeName))
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

// GetMirrorSrcPorts gets 'select_src_port' of a mirror as a string slice
func GetMirrorSrcPorts(mirrorName string) ([]string, error) {
	return GetMirrorPorts(mirrorName, "select_src_port")
}

// GetMirrorDstPorts gets 'select_dst_port' of a mirror as a string slice
func GetMirrorDstPorts(mirrorName string) ([]string, error) {
	return GetMirrorPorts(mirrorName, "select_dst_port")
}

// GetMirrorOutputPorts gets 'output_port' of a mirror as a string slice
func GetMirrorOutputPorts(mirrorName string) ([]string, error) {
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

// AddSelectPortToMirror adds portUUID to a specific mirror via 'ovs-vsctl' as 'select_*' based on ingress and egress values.
// ingress == true => adds portUUID as 'select_src_port'
// egress == true => adds portUUID as 'select_dst_port'
func AddSelectPortToMirror(portUUID, mirrorName string, ingress, egress bool) (bool, error) {
	if ingress {
		output, err := exec.Command("ovs-vsctl", "add", "Mirror", mirrorName, "select_src_port", portUUID).CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("failed to set 'select_src_port' for mirror %s with UUID %s: %v", mirrorName, portUUID, string(output[:]))
		}
	}

	if egress {
		output, err := exec.Command("ovs-vsctl", "add", "Mirror", mirrorName, "select_dst_port", portUUID).CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("failed to set 'select_dst_port' for mirror %s with UUID %s: %v", mirrorName, portUUID, string(output[:]))
		}
	}

	return true, nil
}

// AddOutputPortToMirror adds portUUID as 'output_port' to a specific mirror via 'ovs-vsctl'
func AddOutputPortToMirror(portUUID, mirrorName string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "set", "Mirror", mirrorName, "output_port="+portUUID).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to set output_port for mirror %s with UUID %s: %v", mirrorName, portUUID, string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}

// CreateEmptyMirrors creates multiple mirrors in a bridge with an optional owner in external_ids.
// If ovsPortOwner is an empty string, it will leave external_ids empty.
func CreateEmptyMirrors(bridgeName string, mirrorNames []string, ovsPortOwner string) {
	for _, mirrorName := range mirrorNames {
		_, err := createEmptyMirror(bridgeName, mirrorName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		if ovsPortOwner != "" {
			// manually add owner as external_ids
			_, err = addOwnerToMirror(ovsPortOwner, mirrorName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
	}
}

// CheckEmptyMirrorsExistence checks if all mirrors exist or not, based on 'exists' value.
func CheckEmptyMirrorsExistence(mirrorNames []string, exists bool) {
	for _, mirrorName := range mirrorNames {
		emptyMirExists, err := IsMirrorExists(mirrorName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(emptyMirExists).To(gomega.Equal(exists))
	}
}

// ToJSONString coverts input into a JSON string
func ToJSONString(input interface{}) (string, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// OnlyContainsOrEmpty checks if a list of strings contains only 'el' element or is empty
func OnlyContainsOrEmpty(list []string, el string) bool {
	if len(list) > 1 {
		// because it has more elements, so 'el' cannot be the only one
		return false
	}
	if len(list) == 0 {
		// in empty
		return true
	}
	// otherwise check if the only element in 'list' is equals to 'el'
	return ContainsElement(list, el)
}

// ContainsElement returns true if a list of strings contains a string element
func ContainsElement(list []string, el string) bool {
	for _, l := range list {
		if l == el {
			return true
		}
	}
	return false
}

// createEmptyMirror creates a new empty mirror with name = 'mirrorName' in bridge 'bridgeName'
func createEmptyMirror(bridgeName, mirrorName string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "--", "add", "Bridge", bridgeName, "mirrors", "@m", "--", "--id=@m", "create", "Mirror", "name="+mirrorName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create empty mirror %s in bridge %s: %v", bridgeName, mirrorName, string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}

// addOwnerToMirror adds an owner to an existing mirror as external_ids
func addOwnerToMirror(ovsPortOwner, mirrorName string) (string, error) {
	output, err := exec.Command("ovs-vsctl", "add", "Mirror", mirrorName, "external_ids", "owner="+ovsPortOwner).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to add owner %s to mirror %s: %v", ovsPortOwner, mirrorName, string(output[:]))
	}

	return strings.TrimSpace(string(output[:])), nil
}
