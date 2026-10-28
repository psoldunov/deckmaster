//go:build darwin
// +build darwin

package main

import (
	"strconv"
	"testing"
)

func evdevCode(t *testing.T, name string) int {
	t.Helper()
	code, err := strconv.Atoi(formatKeycodes(name))
	if err != nil {
		t.Fatalf("%s is not a known key name", name)
	}
	return code
}

func TestMacKeycodes(t *testing.T) {
	for name, want := range map[string]uint16{
		"A":         0x00,
		"Num1":      0x12,
		"V":         0x09,
		"Enter":     0x24,
		"Leftctrl":  0x3B,
		"Leftmeta":  0x37,
		"Rightmeta": 0x36,
		"F1":        0x7A,
		"Up":        0x7E,
	} {
		if got, ok := macKeycodes[evdevCode(t, name)]; !ok || got != want {
			t.Errorf("%s: got 0x%02X (mapped: %t), want 0x%02X", name, got, ok, want)
		}
	}
}

func TestMacSystemKeys(t *testing.T) {
	for name, want := range map[string]int{
		"Volumeup":     0,
		"Volumedown":   1,
		"Mute":         7,
		"Playpause":    16,
		"Nextsong":     17,
		"Previoussong": 18,
	} {
		code := evdevCode(t, name)
		if got, ok := macSystemKeys[code]; !ok || got != want {
			t.Errorf("%s: got %d (mapped: %t), want %d", name, got, ok, want)
		}
		if _, ok := macKeycodes[code]; ok {
			t.Errorf("%s is mapped both as a key and as a system key", name)
		}
	}
}

func TestNoMacEquivalent(t *testing.T) {
	// KEY_MICMUTE has no key on a Mac.
	if err := (&cgKeyboard{}).KeyPress(248); err == nil {
		t.Error("keycode 248 posted an event, want an error")
	}
}

func TestPasteIsCommandV(t *testing.T) {
	if pasteKeys != strconv.Itoa(evdevCode(t, "Leftmeta"))+"-"+strconv.Itoa(evdevCode(t, "V")) {
		t.Errorf("pasteKeys = %q, want Leftmeta-V", pasteKeys)
	}
}
