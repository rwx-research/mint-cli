package accesstoken_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
)

var _ = Describe("MemoryBackend", func() {
	Describe("Get/Set", func() {
		It("sets and gets tokens", func() {
			backend, err := accesstoken.NewMemoryBackend()
			Expect(err).NotTo(HaveOccurred())

			token, err := backend.Get()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal(""))

			err = backend.Set("the-token")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal(""))

			token, err = backend.Get()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal("the-token"))
		})
	})
})
