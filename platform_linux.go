//go:build linux
// +build linux

package main

import (
	"fmt"

	"github.com/bendahl/uinput"
	"github.com/godbus/dbus"
)

// pasteKeys pastes the clipboard: ctrl-v.
const pasteKeys = "29-47"

// newKeyboard creates a virtual keyboard through /dev/uinput.
func newKeyboard() (virtualKeyboard, error) {
	keyboard, err := uinput.CreateKeyboard("/dev/uinput", []byte("Deckmaster"))
	if err != nil {
		return nil, fmt.Errorf("can't create virtual input device (/dev/uinput): %w", err)
	}
	return keyboard, nil
}

// connectDBus connects to the session bus that dbus actions call.
func connectDBus() (*dbus.Conn, error) {
	return dbus.SessionBus()
}
