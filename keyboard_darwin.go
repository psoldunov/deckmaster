//go:build darwin
// +build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit

#import <AppKit/AppKit.h>
#include <ApplicationServices/ApplicationServices.h>

// postKey posts a key down or up event for a macOS virtual key code, carrying
// the modifiers held down.
static bool postKey(CGKeyCode code, bool down, CGEventFlags flags) {
	CGEventRef event = CGEventCreateKeyboardEvent(NULL, code, down);
	if (event == NULL) {
		return false;
	}
	CGEventSetFlags(event, flags);
	CGEventPost(kCGHIDEventTap, event);
	CFRelease(event);
	return true;
}

// postSystemKey posts a media key down or up. macOS delivers media keys as
// NX_SYSDEFINED events of subtype 8 instead of key codes.
static bool postSystemKey(int key, bool down, CGEventFlags flags) {
	@autoreleasepool {
		NSInteger state = down ? 0xa : 0xb;
		NSEvent *event = [NSEvent otherEventWithType:NSEventTypeSystemDefined
		                                    location:NSZeroPoint
		                               modifierFlags:flags | (state << 8)
		                                   timestamp:0
		                                windowNumber:0
		                                     context:nil
		                                     subtype:8
		                                       data1:(key << 16) | (state << 8)
		                                       data2:-1];
		CGEventRef cgEvent = [event CGEvent];
		if (cgEvent == NULL) {
			return false;
		}
		CGEventPost(kCGHIDEventTap, cgEvent);
		return true;
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
)

var errNoAccessibility = errors.New("macOS drops emulated key events until deckmaster, or the terminal app that starts it, " +
	"is allowed in System Settings → Privacy & Security → Accessibility")

// accessibilityTrusted reports whether macOS lets deckmaster post key events.
func accessibilityTrusted() bool {
	return C.AXIsProcessTrusted() != 0
}

// cgKeyboard emulates key presses by posting CoreGraphics events for the evdev
// key codes the Linux build hands to uinput.
type cgKeyboard struct {
	mu sync.Mutex
	// modifiers held down with KeyDown, set on every event posted after it
	flags C.CGEventFlags
}

// KeyPress presses a key and releases it.
func (k *cgKeyboard) KeyPress(key int) error {
	if err := k.KeyDown(key); err != nil {
		return err
	}
	return k.KeyUp(key)
}

// KeyDown presses a key until KeyUp releases it.
func (k *cgKeyboard) KeyDown(key int) error {
	return k.post(key, true)
}

// KeyUp releases a key.
func (k *cgKeyboard) KeyUp(key int) error {
	return k.post(key, false)
}

// Close does nothing: there is no device to release.
func (k *cgKeyboard) Close() error {
	return nil
}

func (k *cgKeyboard) post(key int, down bool) error {
	code, isKey := macKeycodes[key]
	system, isSystem := macSystemKeys[key]
	if !isKey && !isSystem {
		return fmt.Errorf("keycode %d has no macOS equivalent", key)
	}
	if !accessibilityTrusted() {
		return errNoAccessibility
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if isSystem {
		if !C.postSystemKey(C.int(system), C.bool(down), k.flags) {
			return fmt.Errorf("can't create a system event for keycode %d", key)
		}
		return nil
	}

	if flag := C.CGEventFlags(macModifiers[code]); flag != 0 {
		if down {
			k.flags |= flag
		} else {
			k.flags &^= flag
		}
	}
	if !C.postKey(C.CGKeyCode(code), C.bool(down), k.flags) {
		return fmt.Errorf("can't create a key event for keycode %d", key)
	}
	return nil
}
