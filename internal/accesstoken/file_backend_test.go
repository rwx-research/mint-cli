package accesstoken_test

import (
	"io"
	"io/fs"
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
)

var _ = Describe("FileBackend", func() {
	var tmp string

	BeforeEach(func() {
		var err error
		tmp, err = os.MkdirTemp(os.TempDir(), "file-backend")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(tmp)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Get", func() {
		Context("when the access token file does not exist", func() {
			It("returns an empty token", func() {
				backend, err := accesstoken.NewFileBackend(tmp)
				Expect(err).NotTo(HaveOccurred())

				token, err := backend.Get()
				Expect(err).NotTo(HaveOccurred())
				Expect(token).To(Equal(""))
			})
		})

		Context("when the access token file is otherwise unable to be opened", func() {
			BeforeEach(func() {
				Expect(os.Chmod(tmp, 0o000)).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				backend, err := accesstoken.NewFileBackend(tmp)
				Expect(err).NotTo(HaveOccurred())

				token, err := backend.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to open"))
				Expect(err).To(MatchError(fs.ErrPermission))
				Expect(token).To(Equal(""))
			})
		})

		Context("when the access token file is present and has contents", func() {
			BeforeEach(func() {
				err := os.WriteFile(path.Join(tmp, "accesstoken"), []byte("the-token"), 0o644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the token", func() {
				backend, err := accesstoken.NewFileBackend(tmp)
				Expect(err).NotTo(HaveOccurred())

				token, err := backend.Get()
				Expect(err).NotTo(HaveOccurred())
				Expect(token).To(Equal("the-token"))
			})
		})

		Context("when the access token file includes leading or trailing whitespace", func() {
			BeforeEach(func() {
				err := os.WriteFile(path.Join(tmp, "accesstoken"), []byte("\n  \t  the-token\t  \n \n"), 0o644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the token with surrounding whitespace trimmed", func() {
				backend, err := accesstoken.NewFileBackend(tmp)
				Expect(err).NotTo(HaveOccurred())

				token, err := backend.Get()
				Expect(err).NotTo(HaveOccurred())
				Expect(token).To(Equal("the-token"))
			})
		})
	})

	Describe("Set", func() {
		Context("when creating the file errors", func() {
			BeforeEach(func() {
				Expect(os.Chmod(tmp, 0o400)).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				backend, err := accesstoken.NewFileBackend(tmp)
				Expect(err).NotTo(HaveOccurred())

				err = backend.Set("the-token")
				Expect(err.Error()).To(ContainSubstring("permission denied"))
				Expect(err).To(MatchError(fs.ErrPermission))
			})
		})

		Context("when the file is created", func() {
			It("writes the token to the file", func() {
				backend, err := accesstoken.NewFileBackend(tmp)
				Expect(err).NotTo(HaveOccurred())

				err = backend.Set("the-token")
				Expect(err).NotTo(HaveOccurred())

				file, err := os.Open(path.Join(tmp, "accesstoken"))
				Expect(err).NotTo(HaveOccurred())

				bytes, err := io.ReadAll(file)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bytes)).To(Equal("the-token"))
			})
		})
	})
})
