// Package main ...
package main

import (
	"github.com/headlesslab/wand/internal/devutil"
)

func main() {
	devutil.Exec("go run ./internal/tools/setup")

	devutil.Exec("go run ./internal/tools/lint")

	devutil.Exec("go test -coverprofile=coverage.out ./lib/launcher")
	devutil.Exec("go run github.com/ysmood/got/cmd/check-cov")

	devutil.Exec("go test -coverprofile=coverage.out")
	devutil.Exec("go run github.com/ysmood/got/cmd/check-cov")
}
