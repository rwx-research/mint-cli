package main

import (
	"fmt"
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("Error: %s", err))
		os.Exit(1)
	}
}
