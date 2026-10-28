package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/muesli/streamdeck"
)

// failingWidget fails to draw the way BaseWidget.render does: it asks to be
// drawn again and returns a drawError.
type failingWidget struct {
	*BaseWidget
	err     error
	updates int
}

func (w *failingWidget) Update() error {
	w.updates++
	if w.err == nil {
		w.lastUpdate = time.Now()
		return nil
	}
	w.Repaint()
	return w.err
}

func newFailingWidget(key uint8, err error) *failingWidget {
	return &failingWidget{BaseWidget: &BaseWidget{key: key}, err: err}
}

// shutdownRequested reports whether something asked deckmaster to exit.
func shutdownRequested(t *testing.T) bool {
	t.Helper()
	select {
	case err := <-shutdown:
		t.Logf("shutdown: %v", err)
		return true
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestFailedDrawIsLoggedAndRetried(t *testing.T) {
	broken := newFailingWidget(1, drawError{key: 1, err: errors.New("IOHIDDeviceSetReport failed")})
	fine := newFailingWidget(2, nil)
	d := &Deck{Widgets: []Widget{broken, fine}}

	d.updateWidgets()
	if shutdownRequested(t) {
		t.Fatal("a failed draw stopped deckmaster")
	}
	if fine.updates != 1 {
		t.Errorf("the key after the failing one was drawn %d times, want 1", fine.updates)
	}

	broken.err = nil
	d.updateWidgets()
	if broken.updates != 2 {
		t.Errorf("the failed key was drawn %d times, want 2", broken.updates)
	}
	d.updateWidgets()
	if broken.updates != 2 || fine.updates != 1 {
		t.Errorf("keys without an interval drew again: %d, %d", broken.updates, fine.updates)
	}
}

func TestGoneDeckStopsDeckmaster(t *testing.T) {
	gone := fmt.Errorf("%w: write failed", streamdeck.ErrDisconnected)
	d := &Deck{Widgets: []Widget{newFailingWidget(0, drawError{key: 0, err: gone})}}

	d.updateWidgets()
	if !shutdownRequested(t) {
		t.Fatal("a gone deck did not stop deckmaster")
	}
}

func TestWidgetErrorStillStopsDeckmaster(t *testing.T) {
	d := &Deck{Widgets: []Widget{newFailingWidget(0, errors.New("top widget needs a mode"))}}

	d.updateWidgets()
	if !shutdownRequested(t) {
		t.Fatal("a widget error no longer stops deckmaster")
	}
}
