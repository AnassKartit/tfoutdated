package main

import (
	"os"

	"github.com/anasskartit/tfoutdated/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
