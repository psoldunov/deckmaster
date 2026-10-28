// Command deckstress drives a Stream Deck the way a busy deck does: several
// keys redrawing at once while the brightness changes and fades, and the
// firmware is queried in between. It uses only the public API, so it builds
// against any version of the streamdeck package, and stops at the first
// error or after -duration.
//
//	go run ./tools/deckstress -duration 10m
//	go run ./tools/deckstress -serial    # the same work from one goroutine
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/muesli/streamdeck"
)

var (
	duration = flag.Duration("duration", 10*time.Minute, "how long to run")
	serial   = flag.Bool("serial", false, "run every operation from one goroutine")
	writers  = flag.Int("writers", 3, "goroutines writing key images")
	device   = flag.String("device", "", "serial number of the deck to use")
	block    = flag.Int("block", 6, "noise block size in pixels; larger blocks make smaller images")
	onlyImgs = flag.Bool("images-only", false, "write key images only, no brightness, fades or firmware queries")
	gap      = flag.Duration("gap", 0, "pause after every image")
)

// counters of successful operations, read by the progress report.
var images, brightness, fades, firmware uint64

func main() {
	flag.Parse()

	dev, err := openDevice()
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(2)
	}

	failed := make(chan error, 1)
	fail := func(op string, err error) {
		select {
		case failed <- fmt.Errorf("%s: %w", op, err):
		default:
		}
	}
	stop := make(chan struct{})

	keys, err := dev.ReadKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read keys:", err)
		os.Exit(2)
	}
	go func() {
		for range keys { //nolint:revive // drain key events
		}
	}()

	var wg sync.WaitGroup
	if *serial {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runSerial(dev, stop, fail)
		}()
	} else {
		runConcurrent(dev, stop, fail, &wg)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	start := time.Now()
	progress := time.NewTicker(10 * time.Second)
	defer progress.Stop()

	code := 0
loop:
	for {
		select {
		case err := <-failed:
			fmt.Printf("FAIL after %s: %v\n", time.Since(start).Round(time.Millisecond), err)
			code = 1
			break loop
		case <-progress.C:
			report(start)
		case <-time.After(time.Until(start.Add(*duration))):
			fmt.Printf("PASS: %s without an error\n", *duration)
			break loop
		case <-sigs:
			fmt.Println("interrupted")
			code = 130
			break loop
		}
	}
	close(stop)
	wg.Wait()
	report(start)

	_ = dev.Reset()
	_ = dev.Close()
	os.Exit(code)
}

func openDevice() (*streamdeck.Device, error) {
	devs, err := streamdeck.Devices()
	if err != nil {
		return nil, err
	}
	for i := range devs {
		if *device != "" && devs[i].Serial != *device {
			continue
		}
		dev := &devs[i]
		if err := dev.Open(); err != nil {
			return nil, err
		}
		ver, err := dev.FirmwareVersion()
		if err != nil {
			return nil, fmt.Errorf("firmware version: %w", err)
		}
		fmt.Printf("deck %s, firmware %s, %d keys, serial=%v writers=%d block=%d images-only=%v gap=%s\n",
			dev.Serial, strings.TrimRight(ver, " \x00"), dev.Keys, *serial, *writers, *block, *onlyImgs, *gap)
		if err := dev.Reset(); err != nil {
			return nil, fmt.Errorf("reset: %w", err)
		}
		return dev, nil
	}
	return nil, fmt.Errorf("no Stream Deck found")
}

func report(start time.Time) {
	fmt.Printf("%6s images=%d brightness=%d fades=%d firmware=%d\n",
		time.Since(start).Round(time.Second),
		atomic.LoadUint64(&images), atomic.LoadUint64(&brightness),
		atomic.LoadUint64(&fades), atomic.LoadUint64(&firmware))
}

func runConcurrent(dev *streamdeck.Device, stop <-chan struct{}, fail func(string, error), wg *sync.WaitGroup) {
	every := func(op string, pause time.Duration, f func(r *rand.Rand) error) {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r := rand.New(rand.NewSource(seed)) //nolint:gosec // test patterns, not secrets
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := f(r); err != nil {
					fail(op, err)
					return
				}
				time.Sleep(pause)
			}
		}(time.Now().UnixNano() + int64(len(op)))
	}

	for i := 0; i < *writers; i++ {
		every("image", 0, func(r *rand.Rand) error {
			return setImage(dev, r)
		})
	}
	if *onlyImgs {
		return
	}
	every("brightness", 20*time.Millisecond, func(r *rand.Rand) error {
		if err := dev.SetBrightness(30 + below(r, 70)); err != nil {
			return err
		}
		atomic.AddUint64(&brightness, 1)
		return nil
	})
	every("fade", 100*time.Millisecond, func(*rand.Rand) error {
		if err := dev.Fade(90, 30, 250*time.Millisecond); err != nil {
			return err
		}
		if err := dev.Fade(30, 90, 250*time.Millisecond); err != nil {
			return err
		}
		atomic.AddUint64(&fades, 2)
		return nil
	})
	every("firmware", 50*time.Millisecond, func(*rand.Rand) error {
		if _, err := dev.FirmwareVersion(); err != nil {
			return err
		}
		atomic.AddUint64(&firmware, 1)
		return nil
	})
}

// runSerial does the concurrent run's work in turn from one goroutine, at
// least as fast: the control for whether overlap is what fails.
func runSerial(dev *streamdeck.Device, stop <-chan struct{}, fail func(string, error)) {
	r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // test patterns, not secrets
	for round := 0; ; round++ {
		select {
		case <-stop:
			return
		default:
		}
		for i := 0; i < *writers; i++ {
			if err := setImage(dev, r); err != nil {
				fail("image", err)
				return
			}
		}
		if *onlyImgs {
			continue
		}
		if err := dev.SetBrightness(30 + below(r, 70)); err != nil {
			fail("brightness", err)
			return
		}
		atomic.AddUint64(&brightness, 1)
		if round%8 == 0 {
			if err := dev.Fade(90, 30, 250*time.Millisecond); err != nil {
				fail("fade", err)
				return
			}
			atomic.AddUint64(&fades, 1)
		}
		if _, err := dev.FirmwareVersion(); err != nil {
			fail("firmware", err)
			return
		}
		atomic.AddUint64(&firmware, 1)
	}
}

func setImage(dev *streamdeck.Device, r *rand.Rand) error {
	key := below(r, int(dev.Keys))
	if err := dev.SetImage(key, noise(int(dev.Pixels), r)); err != nil {
		return err
	}
	atomic.AddUint64(&images, 1)
	time.Sleep(*gap)
	return nil
}

// noise draws coloured blocks, which come out of the JPEG encoder at a few
// KB: an image of several pages, like an icon with a label.
func noise(size int, r *rand.Rand) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	block := *block
	for y := 0; y < size; y += block {
		for x := 0; x < size; x += block {
			c := color.RGBA{below(r, 256), below(r, 256), below(r, 256), 255}
			draw.Draw(img, image.Rect(x, y, x+block, y+block), image.NewUniform(c), image.Point{}, draw.Src)
		}
	}
	return img
}

// below returns a random number from 0 up to n, which is at most 256.
func below(r *rand.Rand, n int) uint8 {
	return uint8(r.Intn(n)) //nolint:gosec // n is at most 256
}
