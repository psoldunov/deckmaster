package main

import (
	"image"

	"github.com/muesli/streamdeck"
)

// ActiveWindowChangedEvent gets emitted when the active window changes.
type ActiveWindowChangedEvent struct {
	Window Window
}

// WindowClosedEvent gets emitted when a window gets closed.
type WindowClosedEvent struct {
	Window Window
}

// Window describes an X11 window.
type Window struct {
	ID    uint32
	Class string
	Name  string
	Icon  image.Image
}

func handleActiveWindowChanged(dev *streamdeck.Device, event ActiveWindowChangedEvent) {
	verbosef("Active window changed to %s (%d, %s)",
		event.Window.Class, event.Window.ID, event.Window.Name)

	// remove dupes
	i := 0
	for _, rw := range recentWindows {
		if rw.ID == event.Window.ID {
			continue
		}

		recentWindows[i] = rw
		i++
	}
	recentWindows = recentWindows[:i]

	keys := int(dev.Keys)
	recentWindows = append([]Window{event.Window}, recentWindows...)
	if len(recentWindows) > keys {
		recentWindows = recentWindows[0:keys]
	}
	deck.updateWidgets()
}

func handleWindowClosed(event WindowClosedEvent) {
	i := 0
	for _, rw := range recentWindows {
		if rw.ID == event.Window.ID {
			continue
		}

		recentWindows[i] = rw
		i++
	}
	recentWindows = recentWindows[:i]
	deck.updateWidgets()
}
