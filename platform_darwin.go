//go:build darwin
// +build darwin

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/godbus/dbus"
)

// pasteKeys pastes the clipboard: cmd-v.
const pasteKeys = "125-47"

// newKeyboard creates a keyboard that posts CoreGraphics events. macOS drops
// those events until the binary has Accessibility permission, which can be
// granted while deckmaster runs, so a missing grant is only reported here.
func newKeyboard() (virtualKeyboard, error) {
	if !accessibilityTrusted() {
		fmt.Fprintf(os.Stderr, "Emulating keyboard events needs Accessibility permission: %s\n", errNoAccessibility)
	}
	return &cgKeyboard{}, nil
}

// connectDBus fails on macOS, which has no session bus for dbus actions.
func connectDBus() (*dbus.Conn, error) {
	return nil, errors.New("dbus actions are not supported on macOS")
}
