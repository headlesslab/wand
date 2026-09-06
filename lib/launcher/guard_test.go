package launcher

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The Orphan guard is proven with Go's helper-process pattern: a test
// re-executes this test binary as the driver (TestGuardDriver), the driver
// launches a browser and reports its PID, the test kills the driver hard and
// watches the browser. One test per mechanism lives in the file of its
// platform: the job object on Windows, the parent-death signal on Linux, the
// Pipe tether on macOS and on Linux with the signal disabled in the driver.

// guardDriverEnv configures the driver's launcher: "on" is launcher.New()
// as it comes, "off" is Leakless(false), "tether" (Linux) disables the
// parent-death signal so that the Pipe tether is proven on its own.
const guardDriverEnv = "WAND_TEST_GUARD"

// guardBound is how long a browser may take to be gone once its driver is;
// the mechanisms fire within a second, the bound covers a loaded runner.
const guardBound = 30 * time.Second

func TestGuardDriver(_ *testing.T) {
	mode := os.Getenv(guardDriverEnv)
	if mode == "" {
		return
	}

	l := New()
	if mode == "off" {
		l.Leakless(false)
	}
	configureDriver(mode)

	l.MustLaunch()
	_, _ = fmt.Fprintln(os.Stdout, "pid", l.PID())

	// Wait to be killed; a driver the test lost exits on its own, and takes a
	// guarded browser with it.
	time.Sleep(time.Minute)
	os.Exit(3)
}

// startDriver runs the driver in the given mode and returns the PID of the
// browser it launched, once the browser is up.
func startDriver(t *testing.T, mode string) (browserPID int, driver *exec.Cmd) {
	t.Helper()

	driver = exec.Command(os.Args[0], "-test.run=^TestGuardDriver$", "-test.timeout=2m")
	driver.Env = append(os.Environ(), guardDriverEnv+"="+mode)
	driver.Stderr = os.Stderr

	out, err := driver.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Start(); err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		if pid, ok := strings.CutPrefix(scanner.Text(), "pid "); ok {
			browserPID, err = strconv.Atoi(pid)
			if err != nil {
				t.Fatal(err)
			}
			return browserPID, driver
		}
	}

	_ = driver.Wait()
	t.Fatal("the driver reported no browser PID")
	return 0, nil
}

// killDriver kills the driver the way a crash or an OOM kill would, with no
// handler of its own running: SIGKILL on POSIX platforms, TerminateProcess on
// Windows, which is what os.Process.Kill does.
func killDriver(t *testing.T, driver *exec.Cmd) {
	t.Helper()

	if err := driver.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = driver.Wait()
}

// waitGone reports whether the process is gone within the bound.
func waitGone(pid int, bound time.Duration) bool {
	deadline := time.Now().Add(bound)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !processAlive(pid)
}

// TestGuardOff: with the guard off, the browser outlives a driver that dies
// hard, and is then cleaned up here with the launcher's own group kill.
func TestGuardOff(t *testing.T) {
	g := setup(t)

	pid, driver := startDriver(t, "off")
	killDriver(t, driver)

	g.False(waitGone(pid, 2*time.Second))

	killGroup(pid)
	g.True(waitGone(pid, guardBound))
}
