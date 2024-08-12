package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	err := rootCmd.Execute()
	if err == nil {
		return
	}

	if !errors.Is(err, LintFailure) {
		if Debug {
			// Enabling debug output will print stacktraces
			fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
	}

	os.Exit(1)
}
