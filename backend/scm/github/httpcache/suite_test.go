package httpcache_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHTTPCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GitHub HTTP Cache Suite")
}
