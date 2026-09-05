// This is the setup file for this test suite.

package main

import (
	"testing"

	"github.com/headlesslab/wand"
	"github.com/ysmood/got"
)

// test context.
type G struct {
	got.G

	browser *wand.Browser
}

// setup for tests.
var setup = func() func(t *testing.T) G {
	browser := wand.New().MustConnect()

	return func(t *testing.T) G {
		t.Parallel() // run each test concurrently

		return G{got.New(t), browser}
	}
}()

// a helper function to create an incognito page.
func (g G) page(url string) *wand.Page {
	page := g.browser.MustIncognito().MustPage(url)
	g.Cleanup(page.MustClose)
	return page
}
