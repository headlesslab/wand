// Package launcher for launching browser utils.
package launcher

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/headlesslab/wand/lib/defaults"
	"github.com/headlesslab/wand/lib/launcher/flags"
	"github.com/headlesslab/wand/lib/utils"
)

// DefaultUserDataDirPrefix ...
var DefaultUserDataDirPrefix = filepath.Join(os.TempDir(), "wand", "user-data")

// Launcher is a helper to launch browser binary smartly.
type Launcher struct {
	Flags map[flags.Flag][]string `json:"flags"`

	ctx       context.Context
	ctxCancel func()

	logger io.Writer

	browser *Browser
	parser  *URLParser
	pid     int
	exit    chan struct{}
	guard   guard

	// findSystem is Browser resolution's search for a System browser,
	// LookPath outside the tests.
	findSystem func() (string, bool)

	// tmpUserDataDir is whether flags.UserDataDir is the temporary directory
	// New made up, which a launch that fails removes again, rather than one
	// the caller or the -wand=dir flag named.
	tmpUserDataDir bool

	managed    bool
	serviceURL string

	isLaunched int32 // zero means not launched
}

// New returns the default arguments to start browser.
// Headless will be enabled by default.
// The Orphan guard ([Launcher.Leakless]) will be enabled by default.
// UserDataDir will use OS tmp dir by default, this folder will usually be cleaned up by the OS after reboot.
// The browser comes from Browser resolution at launch: an explicit path, then
// a System browser, then a Managed browser, downloaded only as a last resort;
// check [Launcher.Bin], [LookPath], [Launcher.Download] and [Launcher.Source]
// for more info.
func New() *Launcher {
	dir := defaults.Dir
	if dir == "" {
		dir = filepath.Join(DefaultUserDataDirPrefix, utils.RandString(8))
	}

	defaultFlags := map[flags.Flag][]string{
		flags.Bin:      {defaults.Bin},
		flags.Download: {downloadDefault()},
		flags.Leakless: nil,

		flags.UserDataDir: {dir},

		// use random port by default
		flags.RemoteDebuggingPort: {defaults.Port},

		// enable headless by default
		flags.Headless: nil,

		// to disable the init blank window
		"no-first-run":      nil,
		"no-startup-window": nil,

		// TODO: about the "site-per-process" see https://github.com/puppeteer/puppeteer/issues/2548
		"disable-features": {"site-per-process", "TranslateUI"},

		"disable-dev-shm-usage":                              nil,
		"disable-background-networking":                      nil,
		"disable-background-timer-throttling":                nil,
		"disable-backgrounding-occluded-windows":             nil,
		"disable-breakpad":                                   nil,
		"disable-client-side-phishing-detection":             nil,
		"disable-component-extensions-with-background-pages": nil,
		"disable-default-apps":                               nil,
		"disable-hang-monitor":                               nil,
		"disable-ipc-flooding-protection":                    nil,
		"disable-popup-blocking":                             nil,
		"disable-prompt-on-repost":                           nil,
		"disable-renderer-backgrounding":                     nil,
		"disable-sync":                                       nil,
		"disable-site-isolation-trials":                      nil,
		"enable-automation":                                  nil,
		"enable-features":                                    {"NetworkService", "NetworkServiceInProcess"},
		"force-color-profile":                                {"srgb"},
		"metrics-recording-only":                             nil,
		"use-mock-keychain":                                  nil,
	}

	if defaults.Show {
		delete(defaultFlags, flags.Headless)
	}
	if defaults.Devtools {
		defaultFlags["auto-open-devtools-for-tabs"] = nil
	}
	if inContainer {
		defaultFlags[flags.NoSandbox] = nil
	}
	if defaults.Proxy != "" {
		defaultFlags[flags.ProxyServer] = []string{defaults.Proxy}
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Launcher{
		ctx:            ctx,
		ctxCancel:      cancel,
		Flags:          defaultFlags,
		exit:           make(chan struct{}),
		browser:        NewBrowser(),
		findSystem:     LookPath,
		tmpUserDataDir: defaults.Dir == "",
		parser:         NewURLParser(),
		logger:         io.Discard,
	}
}

// NewUserMode is a preset to enable reusing current user data. Useful for automation of personal browser.
// If you see any error, it may because you can't launch debug port for existing browser, the solution is to
// completely close the running browser. Unfortunately, there's no API for wand to tell it automatically yet.
// The browser comes from the same Browser resolution as [New], so a System
// browser is preferred to a Managed one.
func NewUserMode() *Launcher {
	ctx, cancel := context.WithCancel(context.Background())

	return &Launcher{
		ctx:       ctx,
		ctxCancel: cancel,
		Flags: map[flags.Flag][]string{
			flags.RemoteDebuggingPort: {"37712"},
			"no-startup-window":       nil,
			flags.Bin:                 {defaults.Bin},
			flags.Download:            {downloadDefault()},
		},
		browser:    NewBrowser(),
		findSystem: LookPath,
		exit:       make(chan struct{}),
		parser:     NewURLParser(),
		logger:     io.Discard,
	}
}

// NewAppMode is a preset to run the browser like a native application.
// The u should be a URL.
func NewAppMode(u string) *Launcher {
	l := New()
	l.Set(flags.App, u).
		Set(flags.Env, "GOOGLE_API_KEY=no").
		Headless(false).
		Delete("no-startup-window").
		Delete("enable-automation")
	return l
}

// Context sets the context.
func (l *Launcher) Context(ctx context.Context) *Launcher {
	ctx, cancel := context.WithCancel(ctx)
	l.ctx = ctx
	l.parser.Context(ctx)
	l.ctxCancel = cancel
	return l
}

// Set a command line argument when launching the browser.
// Be careful the first argument is a flag name, it shouldn't contain values. The values the will be joined with comma.
// A flag can have multiple values. If no values are provided the flag will be a boolean flag.
// You can use the [Launcher.FormatArgs] to debug the final CLI arguments.
// List of available flags: https://peter.sh/experiments/chromium-command-line-switches
func (l *Launcher) Set(name flags.Flag, values ...string) *Launcher {
	name.Check()
	l.Flags[name.NormalizeFlag()] = values
	return l
}

// Get flag's first value.
func (l *Launcher) Get(name flags.Flag) string {
	if list, has := l.GetFlags(name); has {
		return list[0]
	}
	return ""
}

// Has flag or not.
func (l *Launcher) Has(name flags.Flag) bool {
	_, has := l.GetFlags(name)
	return has
}

// GetFlags from settings.
func (l *Launcher) GetFlags(name flags.Flag) ([]string, bool) {
	flag, has := l.Flags[name.NormalizeFlag()]
	return flag, has
}

// Append values to the flag.
func (l *Launcher) Append(name flags.Flag, values ...string) *Launcher {
	flags, has := l.GetFlags(name)
	if !has {
		flags = []string{}
	}
	return l.Set(name, append(flags, values...)...)
}

// Delete a flag.
func (l *Launcher) Delete(name flags.Flag) *Launcher {
	delete(l.Flags, name.NormalizeFlag())
	return l
}

// Bin of the browser binary to launch: the first step of Browser resolution,
// which then goes no further. It beats the -wand=bin= flag, which beats
// WAND_BROWSER_BIN; an empty path leaves the search to the steps below: a
// System browser from [LookPath], then the Managed browser, cached or
// downloaded.
func (l *Launcher) Bin(path string) *Launcher {
	return l.Set(flags.Bin, path)
}

// Download switch: whether Browser resolution may download the Managed
// browser when no explicit path, System browser or cached Managed browser is
// found. On by default; WAND_BROWSER_DOWNLOAD=0 switches it off for a whole
// process. With it off, a launch that finds nothing fails with [ErrNoBrowser].
// Like [Launcher.Bin] it is a flag, so it reaches a remote launcher too.
func (l *Launcher) Download(enable bool) *Launcher {
	if enable {
		return l.Set(flags.Download, "1")
	}

	return l.Set(flags.Download, "0")
}

// Source of the Managed browser, the last steps of Browser resolution, cached
// or downloaded: Chrome for Testing at the Target Chrome, the default, or
// Chromium trunk builds at the Companion Chromium. WAND_BROWSER_SOURCE sets
// the default.
func (l *Launcher) Source(source Source) *Launcher {
	l.browser.Source = source
	return l
}

// Binary of the Chrome for Testing archive the Managed browser comes from:
// the full browser, the default, or chrome-headless-shell.
// WAND_BROWSER_BINARY sets the default.
func (l *Launcher) Binary(binary Binary) *Launcher {
	l.browser.Binary = binary
	return l
}

// Version of Chrome for Testing the Managed browser is, the Target Chrome by
// default. It selects the Chrome for Testing source. A version other than
// the Target Chrome has no recorded hash, so its download is not verified.
func (l *Launcher) Version(version string) *Launcher {
	l.browser.Source = SourceChrome
	l.browser.Version = version
	return l
}

// Revision of the Chromium trunk build the Managed browser is, the Companion
// Chromium by default. It selects the Chromium source. A revision other than
// the Companion Chromium has no recorded hash, so its download is not
// verified.
func (l *Launcher) Revision(rev int) *Launcher {
	l.browser.Source = SourceChromium
	l.browser.Revision = rev
	return l
}

// Hosts to download the Managed browser from, as the URL templates
// [DefaultHosts] describes, for Download hosts of your own. WAND_BROWSER_HOSTS
// sets the default.
func (l *Launcher) Hosts(templates ...string) *Launcher {
	l.browser.Hosts = templates
	return l
}

// Headless switch. Whether to run browser in headless mode. A mode without visible UI.
func (l *Launcher) Headless(enable bool) *Launcher {
	if enable {
		return l.Set(flags.Headless)
	}
	return l.Delete(flags.Headless)
}

// HeadlessNew switch is the "--headless=new" switch: https://developer.chrome.com/docs/chromium/new-headless
func (l *Launcher) HeadlessNew(enable bool) *Launcher {
	if enable {
		return l.Set(flags.Headless, "new")
	}
	return l.Delete(flags.Headless)
}

// NoSandbox switch. Whether to run browser in no-sandbox mode.
// Linux users may face "running as root without --no-sandbox is not supported" in some Linux/Chrome combinations.
// This function helps switch mode easily.
// Be aware disabling sandbox is not trivial. Use at your own risk.
// Related doc: https://bugs.chromium.org/p/chromium/issues/detail?id=638180
func (l *Launcher) NoSandbox(enable bool) *Launcher {
	if enable {
		return l.Set(flags.NoSandbox)
	}
	return l.Delete(flags.NoSandbox)
}

// XVFB enables to run browser in by XVFB. Useful when you want to run headful mode on linux.
func (l *Launcher) XVFB(args ...string) *Launcher {
	return l.Set(flags.XVFB, args...)
}

// Preferences set chromium user preferences, such as set the default search engine or disable the pdf viewer.
// The pref is a json string, the doc is here
// https://src.chromium.org/viewvc/chrome/trunk/src/chrome/common/pref_names.cc
func (l *Launcher) Preferences(pref string) *Launcher {
	return l.Set(flags.Preferences, pref)
}

// AlwaysOpenPDFExternally switch.
// It will set chromium user preferences to enable the always_open_pdf_externally option.
func (l *Launcher) AlwaysOpenPDFExternally() *Launcher {
	return l.Set(flags.Preferences, `{"plugins":{"always_open_pdf_externally": true}}`)
}

// Leakless switch is the Orphan guard: whether the browser dies with the wand process, however
// that process dies, with no helper process and nothing written at launch. It is on in [New] and
// off in [NewUserMode]. On Windows the browser joins a job object that the kernel closes, and so
// kills, with the wand process; on Linux it is started with the parent-death signal SIGKILL; on
// every POSIX platform it is also started with --remote-debugging-pipe on descriptors 3 and 4
// that wand opens and never speaks on, so that a Chromium of 89 or later exits by itself when
// they close. CDP stays on the WebSocket [Launcher.Launch] returns. The pipe flag is added at
// launch, with its descriptors, and is not part of [Launcher.FormatArgs].
// When disabled, [Launcher.Kill] and [Launcher.Cleanup] still kill the browser's process group.
func (l *Launcher) Leakless(enable bool) *Launcher {
	if enable {
		return l.Set(flags.Leakless)
	}
	return l.Delete(flags.Leakless)
}

// Devtools switch to auto open devtools for each tab.
func (l *Launcher) Devtools(autoOpenForTabs bool) *Launcher {
	if autoOpenForTabs {
		return l.Set("auto-open-devtools-for-tabs")
	}
	return l.Delete("auto-open-devtools-for-tabs")
}

// IgnoreCerts configure the Chrome's ignore-certificate-errors-spki-list argument with the public keys.
func (l *Launcher) IgnoreCerts(pks []crypto.PublicKey) error {
	spkis := make([]string, 0, len(pks))

	for _, pk := range pks {
		spki, err := certSPKI(pk)
		if err != nil {
			return fmt.Errorf("certSPKI: %w", err)
		}
		spkis = append(spkis, string(spki))
	}

	l.Set("ignore-certificate-errors-spki-list", spkis...)

	return nil
}

// UserDataDir is where the browser will look for all of its state, such as cookie and cache.
// When set to empty, browser will use current OS home dir.
// Related doc: https://chromium.googlesource.com/chromium/src/+/master/docs/user_data_dir.md
func (l *Launcher) UserDataDir(dir string) *Launcher {
	l.tmpUserDataDir = false
	if dir == "" {
		l.Delete(flags.UserDataDir)
	} else {
		l.Set(flags.UserDataDir, dir)
	}
	return l
}

// ProfileDir is the browser profile the browser will use.
// When set to empty, the profile 'Default' is used.
// Related article: https://superuser.com/a/377195
func (l *Launcher) ProfileDir(dir string) *Launcher {
	if dir == "" {
		l.Delete(flags.ProfileDir)
	} else {
		l.Set(flags.ProfileDir, dir)
	}
	return l
}

// RemoteDebuggingPort to launch the browser. Zero for a random port. Zero is the default value.
// If it's not zero and the Launcher.Leakless is disabled, the launcher will try to reconnect to it first,
// if the reconnection fails it will launch a new browser.
func (l *Launcher) RemoteDebuggingPort(port int) *Launcher {
	return l.Set(flags.RemoteDebuggingPort, fmt.Sprintf("%d", port))
}

// Proxy for the browser.
func (l *Launcher) Proxy(host string) *Launcher {
	return l.Set(flags.ProxyServer, host)
}

// WindowSize for the browser.
func (l *Launcher) WindowSize(x, y int) *Launcher {
	return l.Set(flags.WindowSize, fmt.Sprintf("%d,%d", x, y))
}

// WindowPosition for the browser.
func (l *Launcher) WindowPosition(x, y int) *Launcher {
	return l.Set(flags.WindowPosition, fmt.Sprintf("%d,%d", x, y))
}

// WorkingDir to launch the browser process.
func (l *Launcher) WorkingDir(path string) *Launcher {
	return l.Set(flags.WorkingDir, path)
}

// Env to launch the browser process. The default value is [os.Environ]().
// Usually you use it to set the timezone env. Such as:
//
//	Env(append(os.Environ(), "TZ=Asia/Tokyo")...)
func (l *Launcher) Env(env ...string) *Launcher {
	return l.Set(flags.Env, env...)
}

// StartURL to launch.
func (l *Launcher) StartURL(u string) *Launcher {
	return l.Set("", u)
}

// FormatArgs returns the formatted arg list for cli.
func (l *Launcher) FormatArgs() []string {
	execArgs := []string{}
	for k, v := range l.Flags {
		if k == flags.Arguments {
			continue
		}

		if strings.HasPrefix(string(k), "wand-") {
			continue
		}

		// fix a bug of chrome, if path is not absolute chrome will hang
		if k == flags.UserDataDir {
			abs, err := filepath.Abs(v[0])
			utils.E(err)
			v[0] = abs
		}

		str := "--" + string(k)
		if v != nil {
			str += "=" + strings.Join(v, ",")
		}
		execArgs = append(execArgs, str)
	}

	execArgs = append(execArgs, l.Flags[flags.Arguments]...)
	sort.Strings(execArgs)
	return execArgs
}

// Logger to handle stdout and stderr from browser.
// For example, pipe all browser output to stdout:
//
//	launcher.New().Logger(os.Stdout)
func (l *Launcher) Logger(w io.Writer) *Launcher {
	l.logger = w
	return l
}

// MustLaunch is similar to Launch.
func (l *Launcher) MustLaunch() string {
	u, err := l.Launch()
	utils.E(err)
	return u
}

// Launch a standalone temp browser instance and returns the debug url.
// bin and profileDir are optional, set them to empty to use the default values.
// If you want to reuse sessions, such as cookies, set the [Launcher.UserDataDir] to the same location.
//
// Please note launcher can only be used once.
func (l *Launcher) Launch() (string, error) {
	if l.hasLaunched() {
		return "", ErrAlreadyLaunched
	}

	defer l.ctxCancel()

	bin, err := l.ResolveBin()
	if err != nil {
		return "", err
	}

	l.setupUserPreferences()

	// A guarded browser is this launcher's own; without the guard a browser
	// already listening on the port is reused.
	guarded := l.Has(flags.Leakless)
	if !guarded {
		port := l.Get(flags.RemoteDebuggingPort)
		u, err := ResolveURL(port)
		if err == nil {
			return u, nil
		}
	}

	cmd := exec.Command(bin, l.FormatArgs()...)
	l.setupCmd(cmd)

	if guarded {
		err = l.startGuarded(cmd)
	} else {
		err = cmd.Start()
	}
	if err != nil {
		// Nothing started; the temporary directory the preferences may have
		// been written into goes with the failure.
		l.Cleanup()

		return "", err
	}

	l.pid = cmd.Process.Pid

	go func() {
		_ = cmd.Wait()
		l.guard.release()
		close(l.exit)
	}()

	u, err := l.getURL()
	if err != nil {
		l.Kill()

		// A browser that started and gave no URL has written into the
		// temporary directory made up for it; a failed launch leaves none.
		if l.tmpUserDataDir {
			l.Cleanup()
		}

		return "", err
	}

	return ResolveURL(u)
}

func (l *Launcher) hasLaunched() bool {
	return !atomic.CompareAndSwapInt32(&l.isLaunched, 0, 1)
}

func (l *Launcher) setupUserPreferences() {
	userDir := l.Get(flags.UserDataDir)
	pref := l.Get(flags.Preferences)

	if userDir == "" || pref == "" {
		return
	}

	userDir, err := filepath.Abs(userDir)
	utils.E(err)

	profile := l.Get(flags.ProfileDir)
	if profile == "" {
		profile = "Default"
	}

	path := filepath.Join(userDir, profile, "Preferences")

	utils.E(utils.OutputFile(path, pref))
}

func (l *Launcher) setupCmd(cmd *exec.Cmd) {
	l.osSetupCmd(cmd)

	dir := l.Get(flags.WorkingDir)
	env, _ := l.GetFlags(flags.Env)
	cmd.Dir = dir
	cmd.Env = env

	cmd.Stdout = io.MultiWriter(l.logger, l.parser)
	cmd.Stderr = io.MultiWriter(l.logger, l.parser)
}

// ResolveBin runs Browser resolution (ADR-0005) and returns the browser
// binary [Launcher.Launch] would start, without starting it: the explicit
// path from [Launcher.Bin], the -wand=bin= flag or EnvBrowserBin, then a
// System browser from [LookPath], then the Managed browser already in the
// cache, then its download unless [Launcher.Download] switched that off. An
// explicit path is taken as it is; nothing is compared by version. When every
// step finds nothing the error, wrapping [ErrNoBrowser], lists them all.
// Launch runs it again, so a caller that wants the path for a message, or
// wants one download to serve many launches, calls it first and passes the
// result to [Launcher.Bin].
func (l *Launcher) ResolveBin() (string, error) {
	if bin := l.Get(flags.Bin); bin != "" {
		return bin, nil
	}

	if bin := os.Getenv(EnvBrowserBin); bin != "" {
		return bin, nil
	}

	if bin, has := l.findSystem(); has {
		return bin, nil
	}

	l.browser.Context = l.ctx

	tried := []string{
		"no path from Launcher.Bin(), -wand=bin= or " + EnvBrowserBin,
		"no System browser at the paths LookPath searches on " + runtime.GOOS,
	}

	if l.Get(flags.Download) != "0" {
		bin, err := l.browser.Get()
		if err != nil {
			tried = append(tried, "no usable Managed browser at "+l.browser.BinPath())

			return "", fmt.Errorf("%w: %s; %w", ErrNoBrowser, strings.Join(tried, "; "), err)
		}

		return bin, nil
	}

	err := l.browser.Validate()
	if err == nil {
		return l.browser.BinPath(), nil
	}

	tried = append(tried,
		fmt.Sprintf("no usable Managed browser at %s (%v)", l.browser.BinPath(), err),
		"download off ("+EnvBrowserDownload+"=0 or Launcher.Download(false))",
	)

	return "", fmt.Errorf("%w: %s", ErrNoBrowser, strings.Join(tried, "; "))
}

func (l *Launcher) getURL() (u string, err error) {
	select {
	case <-l.ctx.Done():
		err = l.ctx.Err()
	case u = <-l.parser.URL:
	case <-l.exit:
		err = l.parser.Err()
	}
	return
}

// PID returns the browser process pid.
func (l *Launcher) PID() int {
	return l.pid
}

// Kill the browser process.
func (l *Launcher) Kill() {
	// TODO: If kill too fast, the browser's children processes may not be ready.
	// Browser don't have an API to tell if the children processes are ready.
	utils.Sleep(1)

	if l.PID() == 0 { // avoid killing the current process
		return
	}

	// A browser that has exited is left alone: by now its pid may belong to
	// another process, a browser launched a moment ago among them.
	select {
	case <-l.exit:
		return
	default:
	}

	killGroup(l.PID())
	p, err := os.FindProcess(l.PID())
	if err == nil {
		_ = p.Kill()
	}
}

// Cleanup waits for the browser to exit and removes [flags.UserDataDir].
// Neither wait is unbounded: a browser still running cleanupBound after the
// call is killed, and the removal, which the helper processes of a browser
// can hold up for a moment (a crash handler on Windows for seconds), is
// retried for as long. A launcher that never launched has nothing to wait
// for; it removes the temporary directory New made up, in case a failed
// launch wrote the preferences into it, and leaves a directory the caller
// named alone.
func (l *Launcher) Cleanup() {
	if l.PID() == 0 {
		if l.tmpUserDataDir {
			removeDir(l.Get(flags.UserDataDir))
		}

		return
	}

	select {
	case <-l.exit:
	case <-time.After(cleanupBound):
		l.Kill()
		<-l.exit
	}

	removeDir(l.Get(flags.UserDataDir))
}

// cleanupBound is how long Cleanup waits for the browser to exit before
// killing it, and how long it keeps trying to remove the user data directory.
// A variable for the tests.
var cleanupBound = 10 * time.Second

// removeDir removes dir, retrying a failure every 100 ms until it succeeds or
// cleanupBound passes; the error is dropped, as it was before the retries.
func removeDir(dir string) {
	deadline := time.Now().Add(cleanupBound)

	for {
		err := os.RemoveAll(dir)
		if err == nil || time.Now().After(deadline) {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}
