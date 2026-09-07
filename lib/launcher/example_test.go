package launcher_test

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/utils"
)

// This example has no output comment, so it is compiled but never run: it
// needs a System browser installed, which a machine may not have.
func Example_use_system_browser() {
	if path, exists := launcher.LookPath(); exists {
		l := launcher.New().Bin(path)
		defer l.Cleanup()

		browser := wand.New().ControlURL(l.MustLaunch()).MustConnect()
		defer browser.MustClose()
	}
}

// This example has no output comment, so it is compiled but never run: what it
// prints is the browser's own command line output, which is not the same twice.
func Example_print_browser_CLI_output() {
	l := launcher.New().Logger(os.Stdout)
	defer l.Cleanup()

	// Pipe the browser stderr and stdout to os.Stdout .
	browser := wand.New().ControlURL(l.MustLaunch()).MustConnect()
	defer browser.MustClose()
}

func Example_custom_launch() {
	// get the browser executable path
	path := launcher.NewBrowser().MustGet()

	l := launcher.New()
	defer l.Cleanup()

	// use the FormatArgs to construct args, this line is optional, you can construct the args manually
	args := l.FormatArgs()

	// A browser started this way has no Orphan guard: it outlives a wand
	// process that dies without killing it. launcher.New().Launch() adds the guard.
	cmd := exec.Command(path, args...)

	parser := launcher.NewURLParser()
	cmd.Stderr = parser
	utils.E(cmd.Start())

	// Reap the browser process once it has exited.
	defer func() { _ = cmd.Wait() }()

	u := launcher.MustResolveURL(<-parser.URL)

	browser := wand.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	fmt.Println(browser.MustVersion().ProtocolVersion)

	// Output: 1.3
}
