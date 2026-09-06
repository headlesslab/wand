// Package main ...
package main

import (
	"log"

	"github.com/headlesslab/wand/internal/devutil"
	"github.com/headlesslab/wand/lib/utils"
)

func main() {
	log.Println("setup project...")

	golangDeps()

	nodejsDeps()

	genDockerIgnore()
}

func golangDeps() {
	devutil.Exec("go mod download")
	devutil.Exec("go install mvdan.cc/gofumpt@latest")
}

func nodejsDeps() {
	devutil.UseNode(true)

	devutil.Exec("npm i -s eslint-plugin-html")
}

func genDockerIgnore() {
	s, err := devutil.ReadString(".gitignore")
	utils.E(err)
	utils.E(utils.OutputFile(".dockerignore", s))
}
