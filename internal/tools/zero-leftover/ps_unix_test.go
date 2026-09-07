//go:build !windows

package main

import "testing"

func TestParsePS(t *testing.T) {
	g := setup(t)

	out := "    1 systemd\n" +
		" 2345 chrome\n" +
		"2346 chrome_crashpad_handler\n" +
		"  777 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome\n" +
		"  778 Google Chrome Helper (Renderer)  \n" +
		"abc no-pid\n" +
		"\n"

	g.Eq(parsePS(out), []process{
		{1, "systemd"},
		{2345, "chrome"},
		{2346, "chrome_crashpad_handler"},
		{777, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		{778, "Google Chrome Helper (Renderer)"},
	})
	g.Eq(parsePS(""), []process{})
}
