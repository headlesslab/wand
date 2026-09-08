package utils_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestNoop(_ *testing.T) {
	utils.Noop()
}

func TestTestLog(t *testing.T) {
	g := setup(t)

	var res []interface{}
	lg := utils.Log(func(msg ...interface{}) { res = append(res, msg[0]) })
	lg.Println("ok")
	g.Eq(res[0], "ok")

	utils.LoggerQuiet.Println()

	utils.MultiLogger(lg, lg).Println("ok")
	g.Eq(res, []interface{}{"ok", "ok", "ok"})
}

func TestTestE(t *testing.T) {
	g := setup(t)

	utils.E(nil)

	g.Panic(func() {
		utils.E(errors.New("err"))
	})
}

func TestGenerateRandomString(t *testing.T) {
	g := setup(t)

	v := utils.RandString(10)
	raw, _ := hex.DecodeString(v)
	g.Len(raw, 10)
}

func TestMkdir(t *testing.T) {
	g := setup(t)

	p := filepath.Join(g.Testable.(*testing.T).TempDir(), "t")
	g.E(utils.Mkdir(p))
}

func TestAbsolutePaths(t *testing.T) {
	g := setup(t)

	p := utils.AbsolutePaths([]string{"utils.go"})
	g.Has(p[0], filepath.FromSlash("/utils.go"))
}

func TestOutputString(t *testing.T) {
	g := setup(t)

	p := "tmp/" + g.RandStr(16)

	_ = utils.OutputFile(p, p)

	s, err := os.ReadFile(p)
	if err != nil {
		panic(err)
	}

	g.Eq(string(s), p)
}

func TestOutputBytes(t *testing.T) {
	g := setup(t)

	p := "tmp/" + g.RandStr(16)

	_ = utils.OutputFile(p, []byte("test"))

	s, err := os.ReadFile(p)
	if err != nil {
		panic(err)
	}

	g.Eq(string(s), "test")
}

func TestOutputStream(t *testing.T) {
	g := setup(t)

	p := "tmp/" + g.RandStr(16)
	b := bytes.NewBufferString("test")

	_ = utils.OutputFile(p, b)

	s, err := os.ReadFile(p)
	if err != nil {
		panic(err)
	}

	g.Eq("test", string(s))
}

func TestOutputJSONErr(t *testing.T) {
	g := setup(t)

	p := "tmp/" + g.RandStr(16)

	g.Panic(func() {
		_ = utils.OutputFile(p, make(chan struct{}))
	})
}

func TestSleep(_ *testing.T) {
	utils.Sleep(0.01)
}

func TestAll(t *testing.T) {
	g := setup(t)

	c := g.Count(3)
	utils.All(c, c, c)()
}

func TestPause(_ *testing.T) {
	go utils.Pause()
}

func TestMustToJSON(t *testing.T) {
	g := setup(t)

	g.Eq(utils.Dump("a", 10), `"a" 10`)
	g.Eq(`{"a":1}`, utils.MustToJSON(map[string]int{"a": 1}))
}

func TestFileExists(t *testing.T) {
	g := setup(t)

	g.Eq(false, utils.FileExists("."))
	g.Eq(true, utils.FileExists("utils.go"))
	g.Eq(false, utils.FileExists(g.RandStr(16)))
}

func TestFormatCLIArgs(t *testing.T) {
	g := setup(t)

	g.Eq(utils.FormatCLIArgs([]string{"ab c", "abc"}), `"ab c" abc`)
}

func TestIdleCounter(t *testing.T) {
	g := setup(t)

	utils.All(func() {
		ct := utils.NewIdleCounter(100 * time.Millisecond)

		// The clock starts before the goroutine that sleeps does, so the
		// wait is held to the 300 ms of that sleep plus the counter's own
		// 100 ms of idle after the last Done whatever the scheduler makes of
		// the two goroutines: a start taken after the goroutine had begun its
		// sleep read 393 ms in the Gate. Neither a sleep nor a timer ends
		// early, so the lower bound is exact; the upper bounds below are
		// wide, a hosted runner under -race has taken 10 ms to wake a
		// goroutine.
		start := time.Now()

		ct.Add()
		go func() {
			ct.Add()
			time.Sleep(300 * time.Millisecond)
			ct.Done()
			ct.Done()
		}()

		ctx := g.Context()

		ct.Wait(ctx)
		d := time.Since(start)
		g.Gte(d, 400*time.Millisecond)
		g.Lt(d, 700*time.Millisecond)

		g.Panic(func() {
			ct.Done()
		})

		ctx.Cancel()
		ct.Wait(ctx)
	}, func() {
		ct := utils.NewIdleCounter(100 * time.Millisecond)
		start := time.Now()
		ct.Wait(g.Context())
		g.Lt(time.Since(start), 400*time.Millisecond)
	}, func() {
		// A counter with no idle duration has nothing to wait for: Wait resets
		// the timer to zero and comes back as soon as the scheduler hands the
		// goroutine back. So the only thing a clock can say here is that it
		// came back at all — a timer that never fired would leave Wait blocked
		// until this test's context is cancelled, which is the harness killing
		// the run rather than a failure to read. The bound is what turns that
		// hang into a failure, and it is wide because what it measures is the
		// machine: at 100 ms it went red on a loaded darwin/arm64 Nightly,
		// where waking the goroutine took 114 ms (#99).
		ct := utils.NewIdleCounter(0)
		start := time.Now()
		ct.Wait(g.Context())
		g.Lt(time.Since(start), time.Second)
	})()
}

func TestCropImage(t *testing.T) {
	g := setup(t)

	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))

	g.Err(utils.CropImage(nil, 0, 0, 0, 0, 0))

	bin := bytes.NewBuffer(nil)
	g.E(png.Encode(bin, img))
	g.E(utils.CropImage(bin.Bytes(), 0, 10, 10, 30, 30))

	bin = bytes.NewBuffer(nil)
	g.E(jpeg.Encode(bin, img, &jpeg.Options{Quality: 80}))
	g.E(utils.CropImage(bin.Bytes(), 0, 10, 10, 30, 30))
}
