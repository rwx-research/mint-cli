package messages_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	messages "github.com/rwx-research/mint-cli/internal/messages"
)

var _ = Describe("FormatUserMessage", func() {
	It("builds a string based on the available data", func() {
		Expect(messages.FormatUserMessage("message", "", []messages.StackEntry{}, "")).To(Equal("message"))
		Expect(messages.FormatUserMessage("message", "frame", []messages.StackEntry{}, "")).To(Equal("message\nframe"))
		Expect(messages.FormatUserMessage("message", "frame", []messages.StackEntry{}, "advice")).To(Equal("message\nframe\nadvice"))

		stackTrace := []messages.StackEntry{
			{
					FileName: "mint1.yml",
					Line:     22,
					Column:   11,
					Name:     "*alias",
			},
			{
					FileName: "mint1.yml",
					Line:     5,
					Column:   22,
			},
		}
		Expect(messages.FormatUserMessage("message", "frame", stackTrace, "advice")).To(Equal(`message
frame
  at mint1.yml:5:22
  at *alias (mint1.yml:22:11)
advice`))
	})
})
