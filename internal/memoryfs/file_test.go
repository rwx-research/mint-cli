package memoryfs_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	memoryfs "github.com/rwx-research/mint-cli/internal/memoryfs"
)

var _ = Describe("MemFile", func() {
	It("writes, closes, reads, and overwrites", func() {
		mf := &memoryfs.MemFile{}
		Expect(mf.Bytes()).To(HaveLen(0))

		omf, err := mf.Open()
		Expect(err).To(BeNil())

		for i := 0; i < 1024; i++ {
			n, err := omf.Write([]byte("Hello World!\n"))
			Expect(err).To(BeNil())
			Expect(n).To(Equal(13))
		}

		// Doesn't update until closed
		Expect(len(mf.Bytes())).To(Equal(0))
		err = omf.Close()
		Expect(err).To(BeNil())

		contents := mf.Bytes()
		Expect(len(contents)).To(Equal(13312))
		Expect(contents[0:26]).To(Equal([]byte("Hello World!\nHello World!\n")))

		// Can read, overwrite, and truncate
		omf, err = mf.Open()
		Expect(err).To(BeNil())
		n, err := omf.Read(make([]byte, 6))
		Expect(err).To(BeNil())
		Expect(n).To(Equal(6))
		n, err = omf.Write([]byte("Bob!"))
		Expect(err).To(BeNil())
		Expect(n).To(Equal(4))

		err = omf.Close()
		Expect(err).To(BeNil())
		contents = mf.Bytes()
		Expect(len(contents)).To(Equal(10))
		Expect(contents).To(Equal([]byte("Hello Bob!")))
	})
})
