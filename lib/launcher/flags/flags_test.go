package flags_test

import (
	"testing"

	"github.com/headlesslab/wand/lib/launcher/flags"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestCheck(t *testing.T) {
	g := setup(t)

	flags.Headless.Check()

	g.Panic(func() {
		flags.Flag("headless=new").Check()
	})
}

func TestNormalizeFlag(t *testing.T) {
	g := setup(t)

	g.Eq(flags.Flag("--headless").NormalizeFlag(), flags.Headless)
	g.Eq(flags.Flag("-headless").NormalizeFlag(), flags.Headless)
	g.Eq(flags.Headless.NormalizeFlag(), flags.Headless)
}
