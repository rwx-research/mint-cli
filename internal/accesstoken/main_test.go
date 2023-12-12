package accesstoken_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
)

var _ = Describe("Main", func() {
	Describe("Get", func() {
		It("prefers the provided access token", func() {
			backend, err := accesstoken.NewMemoryBackend()
			Expect(err).NotTo(HaveOccurred())

			err = backend.Set("other-token")
			Expect(err).NotTo(HaveOccurred())

			token, err := accesstoken.Get(backend, "provided-token")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal("provided-token"))
		})

		It("falls back to the stored access token", func() {
			backend, err := accesstoken.NewMemoryBackend()
			Expect(err).NotTo(HaveOccurred())

			err = backend.Set("other-token")
			Expect(err).NotTo(HaveOccurred())

			token, err := accesstoken.Get(backend, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal("other-token"))
		})
	})

	Describe("Set", func() {
		It("stores the token in the backend", func() {
			backend, err := accesstoken.NewMemoryBackend()
			Expect(err).NotTo(HaveOccurred())

			err = accesstoken.Set(backend, "some-token")
			Expect(err).NotTo(HaveOccurred())

			token, err := backend.Get()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).To(Equal("some-token"))
		})
	})
})
