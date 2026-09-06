//go:build !windows

package launcher

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/headlesslab/wand/lib/launcher/flags"
)

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

// start the browser, with the Orphan guard when guarded: the Pipe tether on
// descriptors 3 and 4, and on Linux the parent-death signal.
func (l *Launcher) start(cmd *exec.Cmd, guarded bool) error {
	if !guarded {
		return cmd.Start()
	}

	browserEnds, err := l.tether(cmd)
	if err != nil {
		return err
	}

	err = l.startCmd(cmd)

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
// descriptor 4, wand keeps the other end of each pipe. The flag is passed
// only here, with both descriptors open, since Chrome 113 and later aborts
// without them; FormatArgs never lists it. It returns the browser's ends.
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
	cmd.ExtraFiles = []*os.File{browserRead, browserWrite}
	cmd.Args = append(cmd.Args, "--"+string(flags.RemoteDebuggingPipe))

	return cmd.ExtraFiles, nil
}
