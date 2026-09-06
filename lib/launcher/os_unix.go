//go:build !windows

package launcher

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/headlesslab/wand/lib/launcher/flags"
)

// remoteDebuggingPipe is the browser flag of the Pipe tether. Launch passes
// it only with descriptors 3 and 4 open, since Chrome 113 and later aborts
// without them; it is no flags.Flag on purpose, so that FormatArgs never
// lists it for a command built without the descriptors.
const remoteDebuggingPipe = "--remote-debugging-pipe"

// xvfbTether puts the Pipe tether back on descriptors 3 and 4 under xvfb-run,
// which closes descriptor 3 for the command it runs: the tether also travels
// as descriptors 5 and 6, and this shell line moves them into place before
// it execs the browser.
const xvfbTether = `exec "$0" "$@" 3<&5 4>&6 5<&- 6>&-`

// newPipe is os.Pipe; a test replaces it to make the Pipe tether fail.
var newPipe = os.Pipe

// guard is the Orphan guard's hold on a launched browser: wand's two ends of
// the Pipe tether, open and never spoken on until the browser exits. The
// kernel closes them with the wand process, however it dies, and a Chromium
// of 89 or later exits on the EOF that gives it.
type guard struct {
	held []*os.File
}

func (g *guard) release() {
	for _, f := range g.held {
		_ = f.Close()
	}
	g.held = nil
}

func killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func (l *Launcher) osSetupCmd(cmd *exec.Cmd) {
	if flags, has := l.GetFlags(flags.XVFB); has {
		var command []string
		// flags must append before cmd.Args
		command = append(command, flags...)
		command = append(command, cmd.Args...)

		*cmd = *exec.Command("xvfb-run", command...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// startGuarded starts the browser under the Orphan guard: the Pipe tether on
// descriptors 3 and 4, and on Linux the parent-death signal.
func (l *Launcher) startGuarded(cmd *exec.Cmd) error {
	browserEnds, err := l.tether(cmd)
	if err != nil {
		return err
	}

	err = l.osStart(cmd)

	// The browser holds its ends now, or never will.
	for _, f := range browserEnds {
		_ = f.Close()
	}
	if err != nil {
		l.guard.release()
	}
	return err
}

// tether opens the Pipe tether: the browser reads descriptor 3 and writes
// descriptor 4, wand keeps the other end of each pipe. It returns the
// browser's ends.
func (l *Launcher) tether(cmd *exec.Cmd) ([]*os.File, error) {
	browserRead, wandWrite, err := newPipe()
	if err != nil {
		return nil, err
	}

	wandRead, browserWrite, err := newPipe()
	if err != nil {
		_ = browserRead.Close()
		_ = wandWrite.Close()
		return nil, err
	}

	l.guard.held = []*os.File{wandWrite, wandRead}
	browserEnds := []*os.File{browserRead, browserWrite}

	cmd.Args = append(cmd.Args, remoteDebuggingPipe)
	cmd.ExtraFiles = browserEnds

	if xvfb, has := l.GetFlags(flags.XVFB); has {
		// cmd.Args is xvfb-run, its flags, then the browser and its
		// arguments; the shell goes in front of the browser.
		browser := 1 + len(xvfb)
		args := make([]string, 0, len(cmd.Args)+3)
		args = append(args, cmd.Args[:browser]...)
		args = append(args, "sh", "-c", xvfbTether)
		args = append(args, cmd.Args[browser:]...)
		cmd.Args = args
		cmd.ExtraFiles = []*os.File{browserRead, browserWrite, browserRead, browserWrite}
	}

	return browserEnds, nil
}
