package ovsdb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("hasError", func() {
	It("should return false for empty error string", func() {
		row := map[string]interface{}{"error": ""}
		Expect(hasError(row)).To(BeFalse())
	})

	It("should return true for non-empty error string", func() {
		row := map[string]interface{}{"error": "could not open network device eth0 (No such device)"}
		Expect(hasError(row)).To(BeTrue())
	})

	It("should return false when error key is missing", func() {
		row := map[string]interface{}{"name": "test"}
		Expect(hasError(row)).To(BeFalse())
	})

	It("should return false for non-string error value", func() {
		row := map[string]interface{}{"error": 42}
		Expect(hasError(row)).To(BeFalse())
	})
})
