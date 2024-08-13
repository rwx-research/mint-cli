package memoryfs_test

import (
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	memoryfs "github.com/rwx-research/mint-cli/internal/memoryfs"
)

var _ = Describe("MemoryFS", func() {
	var mfs *memoryfs.MemoryFS

	BeforeEach(func() {
		mfs = memoryfs.NewFS()
	})

	Describe("Create", func() {
		It("returns a file opened for writing", func() {
			file, err := mfs.Create("file.txt")
			Expect(err).To(BeNil())

			n, err := file.Write([]byte("hello"))
			Expect(err).To(BeNil())
			Expect(n).To(Equal(5))
		})

		It("respects cwd", func() {
			_, err := mfs.Create("file.txt")
			Expect(err).To(BeNil())

			entries := mfs.Entries()
			Expect(len(entries)).To(Equal(2))
			Expect(entries["/file.txt"]).NotTo(BeNil())

			err = mfs.MkdirAll("/path/to/wd")
			Expect(err).To(BeNil())

			entries = mfs.Entries()
			Expect(entries["/path"]).NotTo(BeNil())
			Expect(entries["/path/to"]).NotTo(BeNil())
			Expect(entries["/path/to/wd"]).NotTo(BeNil())

			err = mfs.Chdir("/path/to/wd")
			Expect(err).To(BeNil())

			_, err = mfs.Create("file2.txt")
			Expect(err).To(BeNil())
			entries = mfs.Entries()
			Expect(entries["/path/to/wd/file2.txt"]).NotTo(BeNil())
			Expect(entries["/file2.txt"]).To(BeNil())
		})

		It("errors when file exists at path", func() {
			_, err := mfs.Create("file.txt")
			Expect(err).To(BeNil())

			_, err = mfs.Create("file.txt")
			Expect(err).To(MatchError(memoryfs.ErrExist))
		})

		It("errors when directory exists at path", func() {
			err := mfs.MkdirAll("path/to.txt")
			Expect(err).To(BeNil())

			_, err = mfs.Create("path/to.txt")
			Expect(err).To(MatchError(memoryfs.ErrExist))
		})
	})

	Describe("Open", func() {
		It("returns a file opened for writing and reading", func() {
			// Write initial file
			file, err := mfs.Create("file.txt")
			Expect(err).To(BeNil())

			n, err := file.Write([]byte("hello"))
			Expect(err).To(BeNil())
			Expect(n).To(Equal(5))

			err = file.Close()
			Expect(err).To(BeNil())

			// Open for writing
			file, err = mfs.Open("file.txt")
			Expect(err).To(BeNil())

			n, err = file.Write([]byte("hello, world!"))
			Expect(err).To(BeNil())
			Expect(n).To(Equal(13))

			err = file.Close()
			Expect(err).To(BeNil())

			// Open for reading
			file, err = mfs.Open("file.txt")
			Expect(err).To(BeNil())

			contents := readToString(file)
			Expect(contents).To(Equal("hello, world!"))
		})

		It("returns an error if directory doesn't exist", func() {
			_, err := mfs.Create("path/to/file.txt")
			Expect(err).To(MatchError("parent directory doesn't exist at \"path/to\""))
		})
	})

	Describe("ReadDir", func() {
		It("returns directory entries", func() {
			err := mfs.WriteFiles(map[string][]byte{
				"/file0.txt":         []byte(""),
				"/path/to/file1.txt": []byte(""),
				"/path/to/file2.txt": []byte(""),
			})
			Expect(err).To(BeNil())

			// Also create an empty directory
			err = mfs.MkdirAll("/path/to/subdir")
			Expect(err).To(BeNil())

			entries, err := mfs.ReadDir("/path/to")
			Expect(err).To(BeNil())

			Expect(entries).To(HaveLen(3))
			Expect(entries[0].Name()).To(Equal("file1.txt"))
			Expect(entries[0].IsDir()).To(BeFalse())
			Expect(entries[1].Name()).To(Equal("file2.txt"))
			Expect(entries[1].IsDir()).To(BeFalse())
			Expect(entries[2].Name()).To(Equal("subdir"))
			Expect(entries[2].IsDir()).To(BeTrue())
		})

		It("errors if directory doesn't exist", func() {
			_, err := mfs.ReadDir("/path/to")
			Expect(err).To(MatchError(memoryfs.ErrNotExist))
		})

		It("errors if path isn't a directory", func() {
			_, err := mfs.Create("file.txt")
			Expect(err).To(BeNil())

			_, err = mfs.ReadDir("file.txt")
			Expect(err).To(MatchError("path \"file.txt\" is not a directory"))
		})
	})

	Describe("MkdirAll", func() {
		It("creates missing entries", func() {
			err := mfs.MkdirAll("/path/to/dir1")
			Expect(err).To(BeNil())

			entries := mfs.Entries()
			Expect(entries).To(HaveKey("/path"))
			Expect(entries).To(HaveKey("/path/to"))
			Expect(entries).To(HaveKey("/path/to/dir1"))

			err = mfs.MkdirAll("/path/to/dir2/a")
			Expect(err).To(BeNil())

			entries = mfs.Entries()
			Expect(entries).To(HaveKey("/path/to/dir2"))
			Expect(entries).To(HaveKey("/path/to/dir2/a"))
		})

		It("errors if a regular file exists", func() {
			err := mfs.WriteFiles(map[string][]byte{
				"/regular-file": []byte(""),
			})
			Expect(err).To(BeNil())

			err = mfs.MkdirAll("/regular-file")
			Expect(err).To(MatchError("unable to create subdirectory of regular file at \"/regular-file\""))
		})
	})

	Describe("Exists", func() {
		It("returns true only when file or directory exists", func() {
			err := mfs.WriteFiles(map[string][]byte{
				"/path/to/file.txt": []byte(""),
			})
			Expect(err).To(BeNil())

			exists, err := mfs.Exists("path/to/file.txt")
			Expect(err).To(BeNil())
			Expect(exists).To(BeTrue())

			exists, err = mfs.Exists("/path/to/")
			Expect(err).To(BeNil())
			Expect(exists).To(BeTrue())

			exists, err = mfs.Exists("/path/away")
			Expect(err).To(BeNil())
			Expect(exists).To(BeFalse())
		})
	})

	Describe("Stat", func() {
		It("returns entry only when file or directory exists", func() {
			err := mfs.WriteFiles(map[string][]byte{
				"/path/to/file.txt": []byte(""),
			})
			Expect(err).To(BeNil())

			info, err := mfs.Stat("path/to/file.txt")
			Expect(err).To(BeNil())
			Expect(info.Name()).To(Equal("file.txt"))
			Expect(info.IsDir()).To(BeFalse())

			info, err = mfs.Stat("/path/to")
			Expect(err).To(BeNil())
			Expect(info.Name()).To(Equal("to"))
			Expect(info.IsDir()).To(BeTrue())

			_, err = mfs.Stat("/path/away")
			Expect(err).To(MatchError(memoryfs.ErrNotExist))
		})
	})
})

func readToString(r io.Reader) string {
	b, err := io.ReadAll(r)
	Expect(err).To(BeNil())

	return string(b)
}
