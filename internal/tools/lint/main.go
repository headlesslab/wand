// Package main ...
package main

import (
	"github.com/headlesslab/wand/internal/devutil"
)

func main() {
	devutil.UseNode(true)

	devutil.Exec("npx -ys -- cspell@6.31.1 --no-progress **")

	devutil.Exec("npx -ys -- eslint@8.41.0 --ext=.js,.html --fix --ignore-path=.gitignore .")

	devutil.Exec("npx -ys -- prettier@2.8.8 --loglevel=error --write --ignore-path=.gitignore .")

	devutil.Exec("go run github.com/ysmood/golangci-lint@latest")

	lintMustPrefix()

	checkGitClean()
}

func checkGitClean() {
	out := devutil.ExecLine(false, "git status --porcelain")
	if out != "" {
		panic("Please run \"go generate\" on local and git commit the changes:\n" + out)
	}
}
