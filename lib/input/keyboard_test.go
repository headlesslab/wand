package input_test

import (
	"testing"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand/lib/input"
	"github.com/headlesslab/wand/lib/proto"
	"github.com/ysmood/got"
)

func TestKeyMap(t *testing.T) {
	g := got.T(t)

	k := input.Key('a')
	g.Eq(k.Info(), input.KeyInfo{
		Key:      "a",
		Code:     "KeyA",
		KeyCode:  65,
		Location: 0,
	})

	k = input.Key('A')
	g.Eq(k.Info(), input.KeyInfo{
		Key:      "A",
		Code:     "KeyA",
		KeyCode:  65,
		Location: 0,
	})
	g.True(k.Printable())

	k = input.Enter
	g.Eq(k.Info(), input.KeyInfo{
		Key:      "\r",
		Code:     "Enter",
		KeyCode:  13,
		Location: 0,
	})

	k = input.ShiftLeft
	g.Eq(k.Info(), input.KeyInfo /* len=4 */ {
		Key:      "Shift",
		Code:     "ShiftLeft",
		KeyCode:  16,
		Location: 1,
	})
	g.False(k.Printable())

	k = input.ShiftRight
	g.Eq(k.Info(), input.KeyInfo /* len=4 */ {
		Key:      "Shift",
		Code:     "ShiftRight",
		KeyCode:  16,
		Location: 2,
	})

	k, has := input.Digit1.Shift()
	g.True(has)
	g.Eq(k.Info().Key, "!")

	_, has = input.Enter.Shift()
	g.False(has)

	g.Panic(func() {
		input.Key('\n').Info()
	})
}

func TestKeyModifier(t *testing.T) {
	g := got.T(t)

	check := func(k input.Key, m int) {
		g.Helper()

		g.Eq(k.Modifier(), m)
	}

	check(input.KeyA, 0)
	check(input.AltLeft, 1)
	check(input.ControlLeft, 2)
	check(input.MetaLeft, 4)
	check(input.ShiftLeft, 8)
}

func TestKeyEncode(t *testing.T) {
	g := got.T(t)

	g.Eq(input.Key('a').Encode(proto.InputDispatchKeyEventTypeKeyDown, 0), &proto.InputDispatchKeyEvent{
		Type:                  "keyDown",
		Text:                  "a",
		UnmodifiedText:        "a",
		Code:                  "KeyA",
		Key:                   "a",
		WindowsVirtualKeyCode: 65,
		Location:              lazyjson.Int(0),
	})

	g.Eq(input.Key('a').Encode(proto.InputDispatchKeyEventTypeKeyUp, 0), &proto.InputDispatchKeyEvent{
		Type:                  "keyUp",
		Text:                  "a",
		UnmodifiedText:        "a",
		Code:                  "KeyA",
		Key:                   "a",
		WindowsVirtualKeyCode: 65,
		Location:              lazyjson.Int(0),
	})

	g.Eq(input.AltLeft.Encode(proto.InputDispatchKeyEventTypeKeyDown, 0), &proto.InputDispatchKeyEvent{
		Type:                  "rawKeyDown",
		Code:                  "AltLeft",
		Key:                   "Alt",
		WindowsVirtualKeyCode: 18,
		Location:              lazyjson.Int(1),
	})

	g.Eq(input.Numpad1.Encode(proto.InputDispatchKeyEventTypeKeyDown, 0), &proto.InputDispatchKeyEvent{
		Type:                  "keyDown",
		Code:                  "Numpad1",
		Key:                   "1",
		Text:                  "1",
		UnmodifiedText:        "1",
		WindowsVirtualKeyCode: 35,
		IsKeypad:              true,
	})
}

func TestMac(t *testing.T) {
	g := got.T(t)

	old := input.IsMac
	input.IsMac = true
	defer func() { input.IsMac = old }()

	zero := 0

	g.Eq(input.ArrowDown.Encode(proto.InputDispatchKeyEventTypeKeyDown, 0), &proto.InputDispatchKeyEvent{
		Type:                  "rawKeyDown",
		Code:                  "ArrowDown",
		Key:                   "ArrowDown",
		WindowsVirtualKeyCode: 40,
		AutoRepeat:            false,
		IsKeypad:              false,
		IsSystemKey:           false,
		Location:              &zero,
		Commands: []string{
			"moveDown",
		},
	})
}

// TestMultiByteKey is the test of rod #1220, a character of more than one
// byte is one key, so it is printable and goes out as text, and of what
// AddKey does with such a key: it is registered under its own rune, as an
// ASCII key is, with its shifted form. The assertions go through the rune
// rather than the returned Key, so that a second registration of the same
// key, under -count, finds the first.
func TestMultiByteKey(t *testing.T) {
	g := got.T(t)

	// Cyrillic: two bytes per character.
	k := input.AddKey("б", "Б", "KeyБ", 1041, 0)
	g.Eq(k.Info(), input.KeyInfo{
		Key:      "б",
		Code:     "KeyБ",
		KeyCode:  1041,
		Location: 0,
	})
	g.True(k.Printable())

	lower := input.Key('б')
	g.Eq(lower.Info(), k.Info())
	g.True(lower.Printable())

	upper, has := lower.Shift()
	g.True(has)
	g.Eq(upper, input.Key('Б'))
	g.Eq(upper.Info().Key, "Б")
	g.True(upper.Printable())

	g.Eq(lower.Encode(proto.InputDispatchKeyEventTypeKeyDown, 0), &proto.InputDispatchKeyEvent{
		Type:                  "keyDown",
		Text:                  "б",
		UnmodifiedText:        "б",
		Code:                  "KeyБ",
		Key:                   "б",
		WindowsVirtualKeyCode: 1041,
		Location:              lazyjson.Int(0),
	})
}
