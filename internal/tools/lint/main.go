// Package main is the lint step of go generate and of the generate Gate: the
// Node linters that internal/tools/package.json pins, then golangci-lint at
// the version internal/devutil pins (its formatters rewrite, its linters
// report), then upstream's Must-prefix rule and a clean git tree, so that a
// generator whose output drifted from the committed files fails here (spec
// #33, section 13). Node must be on PATH; the setup tool installs the tools.
package main

import (
	"github.com/headlesslab/wand/internal/devutil"
)

func main() {
	devutil.NodeTool("cspell", "--no-progress", "**")

	// The html plugin lives beside the tools, not beside .eslintrc.yml.
	devutil.NodeTool("eslint", "--ext=.js,.html", "--fix", "--ignore-path=.gitignore",
		"--resolve-plugins-relative-to=internal/tools", ".")

	devutil.NodeTool("prettier", "--loglevel=error", "--write", "--ignore-path=.gitignore", ".")

	devutil.GoTool(devutil.GolangciLint, "fmt", "./...")

	devutil.GoTool(devutil.GolangciLint, "run", "./...")

	lintMustPrefix()

	checkGitClean()
}

func checkGitClean() {
	out := devutil.ExecLine(false, "git status --porcelain")
	if out != "" {
		panic("Please run \"go generate\" on local and git commit the changes:\n" + out)
	}
}
