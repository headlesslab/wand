// Package flags ...
package flags

import "strings"

// Flag name of a command line argument of the browser, also known as command line flag or switch.
// List of available flags: https://peter.sh/experiments/chromium-command-line-switches
type Flag string

// TODO: we should automatically generate all the flags here.
const (
	// UserDataDir https://chromium.googlesource.com/chromium/src/+/master/docs/user_data_dir.md
	UserDataDir Flag = "user-data-dir"

	// Headless mode. Whether to run browser in headless mode. A mode without visible UI.
	Headless Flag = "headless"

	// App flag.
	App Flag = "app"

	// RemoteDebuggingPort flag.
	RemoteDebuggingPort Flag = "remote-debugging-port"

	// NoSandbox flag.
	NoSandbox Flag = "no-sandbox"

	// ProxyServer flag.
	ProxyServer Flag = "proxy-server"

	// WindowSize flag.
	WindowSize Flag = "window-size"

	// WindowPosition flag.
	WindowPosition Flag = "window-position"

	// WorkingDir flag.
	WorkingDir Flag = "wand-working-dir"

	// Env flag.
	Env Flag = "wand-env"

	// XVFB flag.
	XVFB Flag = "wand-xvfb"

	// ProfileDir flag.
	ProfileDir = "profile-directory"

	// Preferences flag.
	Preferences Flag = "wand-preferences"

	// Leakless flag.
	Leakless Flag = "wand-leakless"

	// Bin is the browser executable file path. If it's empty, Browser resolution searches for a System browser,
	// then takes the Managed browser, cached or downloaded.
	Bin Flag = "wand-bin"

	// Download is whether Browser resolution may download the Managed browser as its last resort:
	// "0" for no, anything else or no flag for yes.
	Download Flag = "wand-download"

	// KeepUserDataDir flag.
	KeepUserDataDir Flag = "wand-keep-user-data-dir"

	// Arguments for the command. Such as
	//     chrome-bin http://a.com http://b.com
	// The "http://a.com" and "http://b.com" are the arguments.
	Arguments Flag = ""
)

// Check if the flag name is valid.
func (f Flag) Check() {
	if strings.Contains(string(f), "=") {
		panic("flag name should not contain '='")
	}
}

// NormalizeFlag normalize the flag name, remove the leading dash.
func (f Flag) NormalizeFlag() Flag {
	return Flag(strings.TrimLeft(string(f), "-"))
}
