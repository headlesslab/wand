// Example run:
// go test -bench . ./lib/benchmark

package main_test

import (
	"path/filepath"
	"testing"

	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

func BenchmarkCleanup(b *testing.B) {
	u := got.New(b).Serve().Route("/", "", "page body").URL("/")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			launch := launcher.New().UserDataDir(filepath.Join("tmp", "cleanup", utils.RandString(8)))
			b.Cleanup(launch.Cleanup)

			url := launch.MustLaunch()

			browser := wand.New().ControlURL(url).MustConnect()
			b.Cleanup(browser.MustClose)

			browser.MustPage(u).MustClose()
		}
	})
}
