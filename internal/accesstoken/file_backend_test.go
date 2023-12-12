package accesstoken_test

import (
	"io"
	gofs "io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/fs"
	"github.com/rwx-research/mint-cli/internal/mocks"
)

var _ = Describe("FileBackend", func() {
	var mockFS *mocks.FileSystem

	BeforeEach(func() {
		mockFS = new(mocks.FileSystem)
	})

	Describe("Get", func() {
		Context("when the access token file does not exist", func() {
			BeforeEach(func() {
				mockFS.MockOpen = func(name string) (fs.File, error) {
					Expect(name).To(Equal("some/dir/accesstoken"))
					return nil, gofs.ErrNotExist
				}
			})

			It("returns an empty token", func() {
				backend, err := accesstoken.NewFileBackend("some/dir", mockFS)
				Expect(err).NotTo(HaveOccurred())

				token, err := backend.Get()
				Expect(err).NotTo(HaveOccurred())
				Expect(token).To(Equal(""))
			})
		})

		Context("when the access token file is otherwise unable to be opened", func() {
			BeforeEach(func() {
				mockFS.MockOpen = func(name string) (fs.File, error) {
					Expect(name).To(Equal("some/dir/accesstoken"))
					return nil, gofs.ErrPermission
				}
			})

			It("returns an error", func() {
				backend, err := accesstoken.NewFileBackend("some/dir", mockFS)
				Expect(err).NotTo(HaveOccurred())

				token, err := backend.Get()
				Expect(err.Error()).To(ContainSubstring("unable to open"))
				Expect(err).To(MatchError(gofs.ErrPermission))
				Expect(token).To(Equal(""))
			})
		})

		Context("when the access token file is present and has contents", func() {
			BeforeEach(func() {
				mockFS.MockOpen = func(name string) (fs.File, error) {
					Expect(name).To(Equal("some/dir/accesstoken"))
					return mocks.NewFile("the-token"), nil
				}
			})

			It("returns the token", func() {
				backend, err := accesstoken.NewFileBackend("some/dir", mockFS)
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
				mockFS.MockMkdirAll = func(path string) error {
					Expect(path).To(Equal("some/dir"))
					return nil
				}
				mockFS.MockCreate = func(name string) (fs.File, error) {
					Expect(name).To(Equal("some/dir/accesstoken"))
					return nil, gofs.ErrInvalid
				}
			})

			It("returns an error", func() {
				backend, err := accesstoken.NewFileBackend("some/dir", mockFS)
				Expect(err).NotTo(HaveOccurred())

				err = backend.Set("the-token")
				Expect(err.Error()).To(ContainSubstring("unable to create"))
				Expect(err).To(MatchError(gofs.ErrInvalid))
			})
		})

		Context("when the file is created", func() {
			var file *mocks.File

			BeforeEach(func() {
				mockFS.MockMkdirAll = func(path string) error {
					Expect(path).To(Equal("some/dir"))
					return nil
				}
				mockFS.MockCreate = func(name string) (fs.File, error) {
					Expect(name).To(Equal("some/dir/accesstoken"))
					file = mocks.NewFile("")
					return file, nil
				}
			})

			It("writes the token to the file", func() {
				backend, err := accesstoken.NewFileBackend("some/dir", mockFS)
				Expect(err).NotTo(HaveOccurred())

				err = backend.Set("the-token")
				Expect(err).NotTo(HaveOccurred())

				bytes, err := io.ReadAll(file)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bytes)).To(Equal("the-token"))
			})
		})
	})
})
