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

package sriov

import (
	"net"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

func TestSriov(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SR-IOV Suite")
}

var _ = DescribeTable("PF VF information lookup",
	func(vfs []netlink.VfInfo, index int, valid bool) {
		pf := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "pf0", Vfs: vfs}}
		info, err := getVFInfo(pf, index)
		if valid {
			Expect(err).NotTo(HaveOccurred())
			Expect(info).To(Equal(vfs[index]))
		} else {
			Expect(err).To(MatchError(ContainSubstring("failed to get vf info from pf0")))
			Expect(info).To(Equal(netlink.VfInfo{}))
		}
	},
	Entry("missing VF metadata", nil, 0, false),
	Entry("empty VF metadata", []netlink.VfInfo{}, 0, false),
	Entry("negative index", []netlink.VfInfo{{ID: 0}}, -1, false),
	Entry("index equal to VF count", []netlink.VfInfo{{ID: 0}}, 1, false),
	Entry("index above VF count", []netlink.VfInfo{{ID: 0}}, 2, false),
	Entry("mismatched VF ID", []netlink.VfInfo{{ID: 1}}, 0, false),
	Entry("first VF", []netlink.VfInfo{{ID: 0, Mac: net.HardwareAddr{2, 0, 0, 0, 0, 1}}}, 0, true),
	Entry("last VF", []netlink.VfInfo{{ID: 0}, {ID: 1, Mac: net.HardwareAddr{2, 0, 0, 0, 0, 2}}}, 1, true),
)
