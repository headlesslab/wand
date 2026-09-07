// Package input ...
package input

import (
	"unicode/utf8"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand/lib/proto"
)

// Modifier values.
const (
	ModifierAlt     = 1
	ModifierControl = 2
	ModifierMeta    = 4
	ModifierShift   = 8
)

// Key symbol.
type Key rune

// keyMap for key description.
var keyMap = map[Key]KeyInfo{}

// keyMapShifted for shifted key description.
var keyMapShifted = map[Key]KeyInfo{}

var keyShiftedMap = map[Key]Key{}

// AddKey to KeyMap. A key of one character, of however many bytes, is its
// rune, so that Key('б') finds it as Key('a') does; one of a name, such as
// "Shift", or of a character already registered gets a code of its own,
// keyCode + 256 × (location + 1), which is a rune's value too: the
// generated map registers no character beyond ASCII, so none of its codes
// meets one.
func AddKey(key string, shiftedKey string, code string, keyCode int, location int) Key {
	if r, ok := singleRune(key); ok {
		if _, has := keyMap[r]; !has {
			keyMap[r] = KeyInfo{key, code, keyCode, location}

			if rs, ok := singleRune(shiftedKey); ok {
				keyMapShifted[rs] = KeyInfo{shiftedKey, code, keyCode, location}
				keyShiftedMap[r] = rs
			}
			return r
		}
	}

	k := Key(keyCode + (location+1)*256)
	keyMap[k] = KeyInfo{key, code, keyCode, location}

	return k
}

// singleRune is the one character s holds, when it holds exactly one. The
// Snapshot counted bytes (rod #1220), so a character of more than one byte
// was neither a rune key nor printable.
func singleRune(s string) (Key, bool) {
	r, n := utf8.DecodeRuneInString(s)

	return Key(r), n > 0 && n == len(s)
}

// Info of the key.
func (k Key) Info() KeyInfo {
	if k, has := keyMap[k]; has {
		return k
	}
	if k, has := keyMapShifted[k]; has {
		return k
	}

	panic("key not defined")
}

// KeyInfo of a key
// https://developer.mozilla.org/en-US/docs/Web/API/KeyboardEvent
type KeyInfo struct {
	// Here's the value for Shift key on the keyboard

	Key      string // Shift
	Code     string // ShiftLeft
	KeyCode  int    // 16
	Location int    // 1
}

// Shift returns the shifted key, such as shifted "1" is "!".
func (k Key) Shift() (Key, bool) {
	s, has := keyShiftedMap[k]
	return s, has
}

// Printable returns true if the key is printable: its Key is one character.
func (k Key) Printable() bool {
	_, ok := singleRune(k.Info().Key)

	return ok
}

// Modifier returns the modifier value of the key.
func (k Key) Modifier() int {
	switch k.Info().KeyCode {
	case 18:
		return ModifierAlt
	case 17:
		return ModifierControl
	case 91, 92:
		return ModifierMeta
	case 16:
		return ModifierShift
	}
	return 0
}

// Encode general key event.
func (k Key) Encode(t proto.InputDispatchKeyEventType, modifiers int) *proto.InputDispatchKeyEvent {
	tp := t
	if t == proto.InputDispatchKeyEventTypeKeyDown && !k.Printable() {
		tp = proto.InputDispatchKeyEventTypeRawKeyDown
	}

	info := k.Info()
	l := lazyjson.Int(info.Location)
	keypad := false
	if info.Location == 3 {
		l = nil
		keypad = true
	}

	txt := ""
	if k.Printable() {
		txt = info.Key
	}

	var cmd []string
	if IsMac {
		cmd = macCommands[info.Key]
	}

	e := &proto.InputDispatchKeyEvent{
		Type:                  tp,
		WindowsVirtualKeyCode: info.KeyCode,
		Code:                  info.Code,
		Key:                   info.Key,
		Text:                  txt,
		UnmodifiedText:        txt,
		Location:              l,
		IsKeypad:              keypad,
		Modifiers:             modifiers,
		Commands:              cmd,
	}

	return e
}
