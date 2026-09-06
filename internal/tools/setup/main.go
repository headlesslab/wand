// Package main is the setup step of go generate and of the generate Gate:
// the Go modules, the Node tools of internal/tools/package.json from their
// lockfile (Node must be on PATH; golangci-lint needs no install, go run
// builds it at the pinned version), and the .dockerignore mirrored from
// .gitignore.
package main

import (
	"log"

	"github.com/headlesslab/wand/internal/devutil"
	"github.com/headlesslab/wand/lib/utils"
)

func main() {
	log.Println("setup project...")

	devutil.Exec("go mod download")

	devutil.InstallNodeTools()

	genDockerIgnore()
}

func genDockerIgnore() {
	s, err := devutil.ReadString(".gitignore")
	utils.E(err)
	utils.E(utils.OutputFile(".dockerignore", s))
}
