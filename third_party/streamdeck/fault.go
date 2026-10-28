//go:build streamdeck_fault
// +build streamdeck_fault

package streamdeck

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// A build with -tags streamdeck_fault fails writes to a real deck on purpose,
// to show what the retry, reopen and disconnect paths do:
//
//	STREAMDECK_FAULT_EVERY=n  every nth write starts a run of failures
//	STREAMDECK_FAULT_RUN=m    that many writes fail in a row (default 1)
//	STREAMDECK_FAULT_GONE=d   after duration d every write and every reopen
//	                          fails, as if the deck had been unplugged
var fault struct {
	sync.Mutex
	every, run      int
	writes, failing int
	failed          map[byte]time.Time // keys whose image failed, by key
	gone            time.Time
}

var errInjected = errors.New("injected write failure")

func init() {
	fault.every, _ = strconv.Atoi(os.Getenv("STREAMDECK_FAULT_EVERY"))
	fault.run = 1
	fault.failed = map[byte]time.Time{}
	if n, err := strconv.Atoi(os.Getenv("STREAMDECK_FAULT_RUN")); err == nil {
		fault.run = n
	}
	if d, err := time.ParseDuration(os.Getenv("STREAMDECK_FAULT_GONE")); err == nil {
		fault.gone = time.Now().Add(d)
	}

	wrapOpen = func(open func() (hidHandle, error)) func() (hidHandle, error) {
		return func() (hidHandle, error) {
			if gone() {
				return nil, errors.New("injected: device is gone")
			}
			h, err := open()
			if err != nil {
				return nil, err
			}
			return faultyHandle{h}, nil
		}
	}
}

func gone() bool {
	return !fault.gone.IsZero() && time.Now().After(fault.gone)
}

type faultyHandle struct {
	hidHandle
}

func (h faultyHandle) Write(b []byte) (int, error) {
	fault.Lock()
	fault.writes++
	if fault.every > 0 && fault.writes%fault.every == 0 {
		fault.failing = fault.run
	}
	fail := fault.failing > 0 || gone()
	if fault.failing > 0 {
		fault.failing--
	}
	// b is an image page; with the 8-byte headers of the MK.2 and the XL,
	// b[2] is the key and b[6] the page.
	key, firstPage := b[2], b[6] == 0
	failedAt, redraw := fault.failed[key]
	if fail {
		fault.failed[key] = time.Now()
	} else if redraw && firstPage {
		delete(fault.failed, key)
	}
	fault.Unlock()

	if fail {
		os.Stderr.WriteString("streamdeck: injecting a write failure\n") //nolint:errcheck
		return 0, errInjected
	}
	if redraw && firstPage {
		fmt.Fprintf(os.Stderr, "streamdeck: key %d drawn again %s after its last injected failure\n",
			key, time.Since(failedAt).Round(time.Millisecond))
	}
	return h.hidHandle.Write(b)
}
