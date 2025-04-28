package cli_test

import (
	"github.com/goccy/go-yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/errors"
)

var _ = Describe("YamlDoc", func() {
	Context("TryReadStringAtPath", func() {
		It("returns an empty string when the path is not found", func() {
			contents := `
a:
  b: hello
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			Expect(doc.TryReadStringAtPath("$.a.c")).To(Equal(""))
		})

		It("returns a string value at the given path", func() {
			contents := `
a:
  b: hello
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			Expect(doc.ReadStringAtPath("$.a.b")).To(Equal("hello"))
		})

		It("returns false when tasks are not found", func() {
			contents := `
tasks-but-not-really:
  - key: task1
    tasks: [still, no]
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			Expect(doc.HasTasks()).To(BeFalse())
		})
	})
	Context("HasTasks", func() {
		It("returns true when tasks are not found", func() {
			contents := `
on:
  github:

tasks:
  - key: task1
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			Expect(doc.HasTasks()).To(BeTrue())
		})

		It("returns false when tasks are not found", func() {
			contents := `
tasks-but-not-really:
  - key: task1
    tasks: [still, no]
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			Expect(doc.HasTasks()).To(BeFalse())
		})
	})

	Context("InsertOrUpdateBase", func() {
		It("inserts missing base before tasks", func() {
			contents := `
on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  # comment
  os: linux
  tag: 1.0

tasks:
  - key: task1 # another line comment
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.InsertOrUpdateBase(cli.BaseLayerSpec{
				Os:   "linux",
				Tag:  "1.2",
				Arch: "x86_64",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(doc.String()).To(Equal(`on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  # comment
  os: linux
  tag: 1.2

tasks:
  - key: task1 # another line comment
  - key: task2
`))
		})

		It("updates existing base", func() {
			contents := `
on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

tasks:
  - key: task1 # another line comment
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.InsertOrUpdateBase(cli.BaseLayerSpec{
				Os:   "linux",
				Tag:  "1.2",
				Arch: "arm64",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(doc.String()).To(Equal(`on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  arch: arm64
  os: linux
  tag: 1.2

tasks:
  - key: task1 # another line comment
  - key: task2
`))
		})
	})

	Context("InsertBefore", func() {
		It("inserts a yaml object before the given path", func() {
			contents := `
on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

tasks:
  - key: task1 # another line comment
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.InsertBefore("$.tasks", map[string]any{
				"base": map[string]any{
					"os":   "linux",
					"tag":  1.2,
					"arch": "x86_64",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(doc.String()).To(Equal(`on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  arch: x86_64
  os: linux
  tag: 1.2

tasks:
  - key: task1 # another line comment
  - key: task2
`))
		})

		It("errors when the path is not found", func() {
			contents := `
tasks:
  - key: task1
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.MergeAtPath("$.base", map[string]any{
				"tag":  1.2,
				"arch": "x86_64",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to find path ( $.base ): node not found"))
			Expect(errors.Is(err, yaml.ErrNotFoundNode)).To(BeTrue())
		})
	})

	Context("MergeAtPath", func() {
		It("merges a yaml object at a specific path", func() {
			contents := `
on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  # comment
  os: linux

tasks:
  - key: task1 # another line comment
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.MergeAtPath("$.base", map[string]any{
				"tag":  1.2,
				"arch": "x86_64",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(doc.String()).To(Equal(`on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  # comment
  os: linux
  arch: x86_64
  tag: 1.2

tasks:
  - key: task1 # another line comment
  - key: task2
`))
		})

		It("errors when the path is not found", func() {
			contents := `
tasks:
  - key: task1
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.MergeAtPath("$.base", map[string]any{
				"tag":  1.2,
				"arch": "x86_64",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to find path ( $.base ): node not found"))
			Expect(errors.Is(err, yaml.ErrNotFoundNode)).To(BeTrue())
		})
	})

	Context("ReplaceAtPath", func() {
		It("replaces a yaml file at a specific path", func() {
			contents := `
on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  # comment
  os: linux
  tag: 1.0   # comment here
  arch: x86_64

tasks:
  - key: task1 # another line comment
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.ReplaceAtPath("$.base.tag", 1.2)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc.String()).To(Equal(`on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  # comment
  os: linux
  tag: 1.2
  arch: x86_64

tasks:
  - key: task1 # another line comment
  - key: task2
`))
		})

		It("errors when the path is not found", func() {
			contents := `
tasks:
  - key: task1
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.ReplaceAtPath("$.base.tag", 1.2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to find path ( $.base.tag ): node not found"))
			Expect(errors.Is(err, yaml.ErrNotFoundNode)).To(BeTrue())
		})
	})

	Context("SetAtPath", func() {
		It("sets and overwrites a yaml object at a specific path", func() {
			contents := `
on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  # comment
  old: true

tasks:
  - key: task1 # another line comment
  - key: task2
`

			doc, err := cli.ParseYamlDoc(contents)
			Expect(err).NotTo(HaveOccurred())

			err = doc.SetAtPath("$.base", map[string]any{
				"os":  "linux",
				"tag": 1.2,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(doc.String()).To(Equal(`on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tag: not it

base:
  os: linux
  tag: 1.2

tasks:
  - key: task1 # another line comment
  - key: task2
`))
		})
	})
})
