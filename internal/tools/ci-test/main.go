// Package main A helper to run go test on CI with the right environment variables.
package main

import (
	"os"

	"github.com/headlesslab/wand/internal/devutil"
	"github.com/headlesslab/wand/lib/utils"
)

func main() {
	for k, v := range devutil.TestEnvs {
		err := os.Setenv(k, v)
		utils.E(err)
	}
	devutil.Exec("go test", os.Args[1:]...)
}
