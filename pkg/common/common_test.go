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

package common

import (
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

var _ = Describe("", func() {
	testSplitVlanIds := func(conf string, expTrunks []uint, expErr error, setUnmarshalErr bool) {
		var trunks []*types.Trunk
		err := json.Unmarshal([]byte(conf), &trunks)
		if setUnmarshalErr {
			Expect(err).To(HaveOccurred())
			return
		}
		Expect(err).NotTo(HaveOccurred())
		By("Calling testSplitVlanIds method")
		vlanIds, err := SplitVlanIds(trunks)
		if expErr != nil {
			By("Checking expected error is occurred")
			Expect(err).To(Equal(expErr))
		} else {
			By("Checking vlanIds are same as trunk vlans")
			Expect(vlanIds).To(Equal(expTrunks))
		}
	}

	Context("specify trunk with multiple ranges", func() {
		trunks := `[ {"minID": 10, "maxID": 12}, {"minID": 19, "maxID": 20} ]`
		It("testSplitVlanIds method should return with specified values in the range", func() {
			testSplitVlanIds(trunks, []uint{10, 11, 12, 19, 20}, nil, false)
		})
	})
	Context("specify trunk with multiple ids", func() {
		trunks := `[ {"id": 15}, {"id": 19}, {"id": 40} ]`
		It("testSplitVlanIds method should return with specified id values", func() {
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
})
