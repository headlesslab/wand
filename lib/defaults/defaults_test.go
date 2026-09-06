package defaults

import (
	"testing"
	"time"

	"github.com/ysmood/got"
)

func TestBasic(t *testing.T) {
	g := got.T(t)

	Show = true
	Devtools = true
	URL = "test"
	Monitor = "test"

	ResetWith("")
	parse("")
	g.False(Show)
	g.False(Devtools)
	g.Eq("", Monitor)
	g.Eq("", URL)

	parse("show,devtools,trace,slow=2s,port=8080,dir=tmp," +
		"url=http://test.com,cdp,monitor,bin=/path/to/chrome," +
		"proxy=localhost:8080,",
	)

	g.True(Show)
	g.True(Devtools)
	g.True(Trace)
	g.Eq(2*time.Second, Slow)
	g.Eq("8080", Port)
	g.Eq("/path/to/chrome", Bin)
	g.Eq("tmp", Dir)
	g.Eq("http://test.com", URL)
	g.NotNil(CDP.Println)
	g.Eq(":0", Monitor)
	g.Eq("localhost:8080", Proxy)

	parse("monitor=:1234")
	g.Eq(":1234", Monitor)

	g.Panic(func() {
		parse("a")
	})

	g.Eq(try(func() { parse("slow=1") }), "invalid value for \"slow\": time: missing unit in duration \"1\" (learn format from https://golang.org/pkg/time/#ParseDuration)")
}

func try(fn func()) (err interface{}) {
	defer func() {
		err = recover()
	}()

	fn()

	return err
}

func TestParseFlag(t *testing.T) {
	g := got.T(t)

	Reset()

	parseFlag([]string{"-wand"})
	g.False(Show)

	parseFlag([]string{"-wand=show"})
	g.True(Show)

	Reset()

	parseFlag([]string{"-wand", "show"})
	g.True(Show)
}
