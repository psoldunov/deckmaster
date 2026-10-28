//go:build darwin
// +build darwin

package main

import (
	"errors"
	"time"
)

// errNoX11 is what window tracking answers on macOS, which has no X server, so
// the recentWindow widget stays empty there.
var errNoX11 = errors.New("there is no X server on macOS")

// Xorg stands in for the X11 session the Linux build tracks windows through.
type Xorg struct{}

// Connect fails on macOS.
func Connect(_ string) (*Xorg, error) {
	return nil, errNoX11
}

// Close does nothing on macOS.
func (x Xorg) Close() {}

// TrackWindows does nothing on macOS.
func (x *Xorg) TrackWindows(_ chan interface{}, _ time.Duration) {}

// RequestActivation fails on macOS.
func (x Xorg) RequestActivation(_ Window) error {
	return errNoX11
}

// CloseWindow fails on macOS.
func (x Xorg) CloseWindow(_ Window) error {
	return errNoX11
}
