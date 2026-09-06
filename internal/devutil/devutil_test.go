package devutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/headlesslab/wand/internal/devutil"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestSTemplate(t *testing.T) {
	g := setup(t)

	out := devutil.S(
		"{{.a}} {{.b}} {{.c.A}} {{d}}",
		"a", "<value>",
		"b", 10,
		"c", struct{ A string }{"ok"},
		"d", func() string {
			return "ok"
		},
	)
	g.Eq("<value> 10 ok ok", out)
}

func TestReadString(t *testing.T) {
	g := setup(t)

	p := filepath.Join(g.Testable.(*testing.T).TempDir(), "t")
	g.E(os.WriteFile(p, []byte("test"), 0o600))

	s, err := devutil.ReadString(p)
	g.E(err)
	g.Eq(s, "test")

	_, err = devutil.ReadString(filepath.Join(p, "missing"))
	g.Err(err)
}

func TestExec(t *testing.T) {
	g := setup(t)

	g.Has(devutil.Exec("go version"), "go version")
}

func TestExecErr(t *testing.T) {
	g := setup(t)

	g.Panic(func() {
		devutil.Exec("")
	})
	g.Panic(func() {
		devutil.Exec(g.RandStr(16))
	})
	g.Panic(func() {
		devutil.ExecLine(false, "", "")
	})
}

func TestExecLineReturnsStdoutOnly(t *testing.T) {
	g := setup(t)

	// go build -n writes the commands it would run to stderr and nothing to
	// stdout, so anything returned here leaked in from stderr.
	g.Eq(devutil.ExecLine(false, "go build -n ."), "")
}

func TestEscapeGoString(t *testing.T) {
	g := setup(t)

	g.Eq("`` + \"`\" + `test` + \"`\" + ``", devutil.EscapeGoString("`test`"))
}
