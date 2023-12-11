package main_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mint "github.com/rwx-research/mint-cli/cmd/mint"
)

var _ = Describe("ParseInitParameters", func() {
	It("should parse init parameters", func() {
		parsed, err := mint.ParseInitParameters([]string{"a=b", "c=d"})
		Expect(err).To(BeNil())
		Expect(parsed).To(Equal(map[string]string{"a": "b", "c": "d"}))
	})

	It("should parse init parameter with equals signs", func() {
		parsed, err := mint.ParseInitParameters([]string{"a=b=c=d"})
		Expect(err).To(BeNil())
		Expect(parsed).To(Equal(map[string]string{"a": "b=c=d"}))
	})

	It("should error if init parameter is not equals-delimited", func() {
		parsed, err := mint.ParseInitParameters([]string{"a"})
		Expect(parsed).To(BeNil())
		Expect(err).To(MatchError("unable to parse \"a\""))
	})
})
