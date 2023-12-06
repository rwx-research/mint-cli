package integration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

type input struct {
	args []string
}

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

func mintCmd(input input) *exec.Cmd {
	const mintPath = "../mint"
	_, err := os.Stat(mintPath)
	Expect(err).ToNot(HaveOccurred(), "integration tests depend on a built mint binary at %s", mintPath)

	cmd := exec.Command(mintPath, input.args...)

	fmt.Fprintf(GinkgoWriter, "Executing command: %s\n with env %s\n", cmd.String(), cmd.Env)

	return cmd
}

func runMint(input input) result {
	cmd := mintCmd(input)
	var stdoutBuffer, stderrBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	err := cmd.Run()

	exitCode := 0

	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		Expect(ok).To(BeTrue(), "mint exited with an error that wasn't an ExitError")
		exitCode = exitErr.ExitCode()
	}

	return result{
		stdout:   strings.TrimSuffix(stdoutBuffer.String(), "\n"),
		stderr:   strings.TrimSuffix(stderrBuffer.String(), "\n"),
		exitCode: exitCode,
	}
}

var _ = Describe("mint run", func() {
	It("errors if an init parameter is specified without a flag", func() {
		input := input{
			args: []string{"run", "--access-token", "fake-for-test", "--file", "./hello-world.mint.yaml", "init=foo"},
		}

		result := runMint(input)

		Expect(result.exitCode).To(Equal(1))
		Expect(result.stderr).To(ContainSubstring("You have specified a task target with an equals sign: \"init=foo\"."))
		Expect(result.stderr).To(ContainSubstring("You may have meant to specify --init \"init=foo\"."))
	})
})
