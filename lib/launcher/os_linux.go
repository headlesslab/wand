//go:build linux

package launcher

import (
	"os/exec"
	"runtime"
	"syscall"
)

// parentDeathSignal is what the kernel sends the browser when the thread that
// started it is gone. The tether test of this package sets it to zero in its
// driver, to prove the Pipe tether on its own.
var parentDeathSignal = syscall.SIGKILL

// osStart starts the browser with the parent-death signal. The kernel sends
// it when the thread that created the process exits, not when the process
// does (PR_SET_PDEATHSIG), so the browser is started from a goroutine that
// locks its thread and holds it until the browser has exited; the wand
// process dying, however it dies, takes the thread and so the browser with
// it. The signal is not inherited by the browser's own children, which the
// process-group kill and the browser's own shutdown cover. cmd.SysProcAttr
// is the one osSetupCmd always sets.
func (l *Launcher) osStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr.Pdeathsig = parentDeathSignal

	started := make(chan error, 1)
	go func() {
		// Never unlocked: a goroutine that returns with its thread locked
		// ends the thread instead of handing it back to the scheduler.
		runtime.LockOSThread()

		err := cmd.Start()
		started <- err
		if err == nil {
			<-l.exit
		}
	}()

	return <-started
}
