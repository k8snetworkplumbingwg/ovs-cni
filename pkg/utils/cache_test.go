// Copyright (c) 2024 CNI authors
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

package utils

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type testConf struct {
	Data string `json:"data"`
}

func writeToCacheDir(tmpDir, cacheDir, key string, data []byte) {
	d := filepath.Join(tmpDir, cacheDir)
	ExpectWithOffset(1, os.MkdirAll(d, 0700)).NotTo(HaveOccurred())
	ExpectWithOffset(1, os.WriteFile(filepath.Join(d, key), data, 0700)).NotTo(HaveOccurred())
}

var _ = Describe("Utils", func() {
	Context("Cache", func() {
		var (
			tmpDir string
			err    error
		)
		BeforeEach(func() {
			tmpDir, err = os.MkdirTemp("", "ovs-cni-cache-test*")
			rootDir = tmpDir
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			rootDir = ""
			Expect(os.RemoveAll(tmpDir)).NotTo(HaveOccurred())
		})
		It("should save data to the new cache path", func() {
			Expect(SaveCache("key1", testConf{Data: "test"})).NotTo(HaveOccurred())
			data, err := os.ReadFile(filepath.Join(tmpDir, "/var/lib/cni/ovs-cni/cache/key1"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal(`{"data":"test"}`))
		})
		It("should return data from the new cache dir", func() {
			origData := []byte(`{"data":"test"}`)
			writeToCacheDir(tmpDir, "/var/lib/cni/ovs-cni/cache", "key1", origData)
			data, err := ReadCache("key1")
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal([]byte(`{"data":"test"}`)))
		})
		It("should return data from the old cache dir", func() {
			origData := []byte(`{"data":"test"}`)
			writeToCacheDir(tmpDir, "/tmp/ovscache", "key1", origData)
			data, err := ReadCache("key1")
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal([]byte(`{"data":"test"}`)))
		})
		It("should return error if can't read data from new and old path", func() {
			data, err := ReadCache("key1")
			Expect(err).To(MatchError(ContainSubstring("not found")))
			Expect(data).To(BeNil())
		})
		It("should remove data from old and new path", func() {
			origData := []byte(`{"data":"test"}`)
			writeToCacheDir(tmpDir, "/var/lib/cni/ovs-cni/cache", "key1", origData)
			writeToCacheDir(tmpDir, "/tmp/ovscache", "key1", origData)
			Expect(CleanCache("key1")).NotTo(HaveOccurred())
			_, err := ReadCache("key1")
			Expect(err).To(MatchError(ContainSubstring("not found")))
		})
		It("should not return error when clean called for unknown key", func() {
			Expect(CleanCache("key1")).NotTo(HaveOccurred())
		})
	})
})
