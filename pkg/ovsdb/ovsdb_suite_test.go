package ovsdb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestOvsdb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ovsdb Suite")
}
