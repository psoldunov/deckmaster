package streamdeck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

const (
	// 30 fps fade animation.
	fadeDelay = time.Second / 30

	// A write or feature report that fails is tried ioAttempts times on the
	// open handle, ioRetryDelay apart, then as often again on a reopened one.
	// macOS rejects the odd transfer to a Stream Deck MK.2 with
	// kIOReturnError (0xE00002BC), about once a minute under a busy deck and
	// with or without other transfers in flight; the next attempt goes through.
	ioAttempts   = 3
	ioRetryDelay = 20 * time.Millisecond
)

// ErrDisconnected is returned once the device has no open handle: it kept
// failing and could not be opened again, which is what an unplugged deck
// looks like, or it was closed. Every later call returns it as well.
var ErrDisconnected = errors.New("stream deck disconnected")

// hidHandle is the part of an open HID device a Stream Deck uses. The tests
// stand in for it.
type hidHandle interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	SendFeatureReport(b []byte) (int, error)
	GetFeatureReport(b []byte) (int, error)
	Close() error
}

// Stream Deck Vendor & Product IDs.
//
//nolint:revive
const (
	VID_ELGATO              = 0x0fd9
	PID_STREAMDECK          = 0x0060
	PID_STREAMDECK_V2       = 0x006d
	PID_STREAMDECK_MK2      = 0x0080
	PID_STREAMDECK_MINI     = 0x0063
	PID_STREAMDECK_MINI_MK2 = 0x0090
	PID_STREAMDECK_XL       = 0x006c
)

// Firmware command IDs.
//
//nolint:revive
var (
	c_REV1_FIRMWARE   = []byte{0x04}
	c_REV1_RESET      = []byte{0x0b, 0x63}
	c_REV1_BRIGHTNESS = []byte{0x05, 0x55, 0xaa, 0xd1, 0x01}

	c_REV2_FIRMWARE   = []byte{0x05}
	c_REV2_RESET      = []byte{0x03, 0x02}
	c_REV2_BRIGHTNESS = []byte{0x03, 0x08}
)

// Device represents a single Stream Deck device.
type Device struct {
	ID     string
	Serial string

	Columns uint8
	Rows    uint8
	Keys    uint8
	Pixels  uint
	DPI     uint
	Padding uint

	featureReportSize   int
	firmwareOffset      int
	keyStateOffset      int
	translateKeyIndex   func(index, columns uint8) uint8
	imagePageSize       int
	imagePageHeaderSize int
	flipImage           func(image.Image) image.Image
	toImageFormat       func(image.Image) ([]byte, error)
	imagePageHeader     func(pageIndex int, keyIndex uint8, payloadLength int, lastPage bool) []byte

	getFirmwareCommand   []byte
	resetCommand         []byte
	setBrightnessCommand []byte

	keyState []byte

	// ioMutex guards device and serializes every write, feature report,
	// reopen and close, so the pages of an image go out back to back.
	// Overlapping transfers also make macOS reject them far more often.
	ioMutex *sync.Mutex
	device  hidHandle
	open    func() (hidHandle, error)

	// sleepMutex guards everything from here down.
	lastActionTime time.Time
	asleep         bool
	sleepCancel    context.CancelFunc
	sleepMutex     *sync.RWMutex
	fadeDuration   time.Duration

	brightness         uint8
	preSleepBrightness uint8
}

// Key holds the current status of a key on the device.
type Key struct {
	Index   uint8
	Pressed bool
}

// Devices returns all attached Stream Decks.
func Devices() ([]Device, error) {
	dd := []Device{}

	devs := hidEnumerate(VID_ELGATO, 0)
	for _, d := range devs {
		var dev Device

		switch {
		case d.VendorID == VID_ELGATO && d.ProductID == PID_STREAMDECK:
			dev = Device{
				ID:                   d.Path,
				Serial:               d.Serial,
				Columns:              5,
				Rows:                 3,
				Keys:                 15,
				Pixels:               72,
				DPI:                  124,
				Padding:              16,
				featureReportSize:    17,
				firmwareOffset:       5,
				keyStateOffset:       1,
				translateKeyIndex:    translateRightToLeft,
				imagePageSize:        7819,
				imagePageHeaderSize:  16,
				imagePageHeader:      rev1ImagePageHeader,
				flipImage:            flipHorizontally,
				toImageFormat:        toBMP,
				getFirmwareCommand:   c_REV1_FIRMWARE,
				resetCommand:         c_REV1_RESET,
				setBrightnessCommand: c_REV1_BRIGHTNESS,
			}
		case d.VendorID == VID_ELGATO && (d.ProductID == PID_STREAMDECK_MINI || d.ProductID == PID_STREAMDECK_MINI_MK2):
			dev = Device{
				ID:                   d.Path,
				Serial:               d.Serial,
				Columns:              3,
				Rows:                 2,
				Keys:                 6,
				Pixels:               80,
				DPI:                  138,
				Padding:              16,
				featureReportSize:    17,
				firmwareOffset:       5,
				keyStateOffset:       1,
				translateKeyIndex:    identity,
				imagePageSize:        1024,
				imagePageHeaderSize:  16,
				imagePageHeader:      miniImagePageHeader,
				flipImage:            rotateCounterclockwise,
				toImageFormat:        toBMP,
				getFirmwareCommand:   c_REV1_FIRMWARE,
				resetCommand:         c_REV1_RESET,
				setBrightnessCommand: c_REV1_BRIGHTNESS,
			}
		case d.VendorID == VID_ELGATO && (d.ProductID == PID_STREAMDECK_V2 || d.ProductID == PID_STREAMDECK_MK2):
			dev = Device{
				ID:                   d.Path,
				Serial:               d.Serial,
				Columns:              5,
				Rows:                 3,
				Keys:                 15,
				Pixels:               72,
				DPI:                  124,
				Padding:              16,
				featureReportSize:    32,
				firmwareOffset:       6,
				keyStateOffset:       4,
				translateKeyIndex:    identity,
				imagePageSize:        1024,
				imagePageHeaderSize:  8,
				imagePageHeader:      rev2ImagePageHeader,
				flipImage:            flipHorizontallyAndVertically,
				toImageFormat:        toJPEG,
				getFirmwareCommand:   c_REV2_FIRMWARE,
				resetCommand:         c_REV2_RESET,
				setBrightnessCommand: c_REV2_BRIGHTNESS,
			}
		case d.VendorID == VID_ELGATO && d.ProductID == PID_STREAMDECK_XL:
			dev = Device{
				ID:                   d.Path,
				Serial:               d.Serial,
				Columns:              8,
				Rows:                 4,
				Keys:                 32,
				Pixels:               96,
				DPI:                  166,
				Padding:              16,
				featureReportSize:    32,
				firmwareOffset:       6,
				keyStateOffset:       4,
				translateKeyIndex:    identity,
				imagePageSize:        1024,
				imagePageHeaderSize:  8,
				imagePageHeader:      rev2ImagePageHeader,
				flipImage:            flipHorizontallyAndVertically,
				toImageFormat:        toJPEG,
				getFirmwareCommand:   c_REV2_FIRMWARE,
				resetCommand:         c_REV2_RESET,
				setBrightnessCommand: c_REV2_BRIGHTNESS,
			}
		}

		if dev.ID != "" {
			dev.keyState = make([]byte, dev.Columns*dev.Rows)
			dev.ioMutex = &sync.Mutex{}
			dev.sleepMutex = &sync.RWMutex{}
			dev.open = wrapOpen(opener(d))
			dd = append(dd, dev)
		}
	}

	return dd, nil
}

// wrapOpen lets a build with -tags streamdeck_fault stand between a deck and
// its handles; see fault.go.
var wrapOpen = func(open func() (hidHandle, error)) func() (hidHandle, error) { return open }

// opener opens the device by the path it was enumerated under. A replugged
// deck comes back under a new path, so it cannot be reopened this way.
func opener(info hidDeviceInfo) func() (hidHandle, error) {
	return func() (hidHandle, error) {
		device, err := info.Open()
		if err != nil {
			return nil, err
		}
		return device, nil
	}
}

// Open the device for input/output. This must be called before trying to
// communicate with the device.
func (d *Device) Open() error {
	d.sleepMutex.Lock()
	d.lastActionTime = time.Now()
	d.sleepMutex.Unlock()

	d.ioMutex.Lock()
	defer d.ioMutex.Unlock()
	return d.reopen()
}

// Close the connection with the device.
func (d *Device) Close() error {
	d.cancelSleepTimer()

	d.ioMutex.Lock()
	defer d.ioMutex.Unlock()
	if d.device == nil {
		return nil
	}
	err := d.device.Close()
	d.device = nil
	return err
}

// reopen closes the handle, if there is one, and opens the device again.
// The caller holds ioMutex. On failure the device is left closed.
func (d *Device) reopen() error {
	if d.device != nil {
		_ = d.device.Close()
		d.device = nil
	}
	device, err := d.open()
	if err != nil {
		return err
	}
	d.device = device
	return nil
}

// do runs op on the open handle under ioMutex, so no other write, feature
// report or reopen comes in between. An op that keeps failing is run again on
// a reopened handle; a device that cannot be reopened stays closed and the
// error wraps ErrDisconnected.
func (d *Device) do(op func(hidHandle) error) error {
	d.ioMutex.Lock()
	defer d.ioMutex.Unlock()

	if d.device == nil {
		return ErrDisconnected
	}
	err := d.attempt(op)
	if err == nil {
		return nil
	}
	if rerr := d.reopen(); rerr != nil {
		return fmt.Errorf("%w: %v, then reopening it failed: %v", ErrDisconnected, err, rerr)
	}
	return d.attempt(op)
}

// attempt runs op up to ioAttempts times on the open handle.
func (d *Device) attempt(op func(hidHandle) error) error {
	var err error
	for i := 0; i < ioAttempts; i++ {
		if i > 0 {
			time.Sleep(ioRetryDelay)
		}
		if err = op(d.device); err == nil {
			return nil
		}
	}
	return err
}

// FirmwareVersion returns the firmware version of the device.
func (d *Device) FirmwareVersion() (string, error) {
	result, err := d.getFeatureReport(d.getFirmwareCommand)
	if err != nil {
		return "", err
	}
	return string(result[d.firmwareOffset:]), nil
}

// Resets the Stream Deck, clears all button images and shows the standby image.
func (d *Device) Reset() error {
	return d.sendFeatureReport(d.resetCommand)
}

// Clears the Stream Deck, setting a black image on all buttons.
func (d *Device) Clear() error {
	img := image.NewRGBA(image.Rect(0, 0, int(d.Pixels), int(d.Pixels)))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{0, 0, 0, 255}), image.Point{}, draw.Src)
	for i := uint8(0); i < d.Keys; i++ {
		if err := d.SetImage(i, img); err != nil {
			return err
		}
	}

	return nil
}

// ReadKeys returns a channel, which it will use to emit key presses/releases.
// The channel is closed once the device is closed or gone.
func (d *Device) ReadKeys() (chan Key, error) {
	kch := make(chan Key)
	keyBuffer := make([]byte, d.keyStateOffset+len(d.keyState))
	go func() {
		defer close(kch)
		for {
			copy(d.keyState, keyBuffer[d.keyStateOffset:])

			if !d.read(keyBuffer) {
				return
			}

			// don't trigger a key event if the device is asleep, but wake it
			if d.Asleep() {
				_ = d.Wake()

				// reset state so no spurious key events get triggered
				for i := d.keyStateOffset; i < len(keyBuffer); i++ {
					keyBuffer[i] = 0
				}
				continue
			}

			d.sleepMutex.Lock()
			d.lastActionTime = time.Now()
			d.sleepMutex.Unlock()

			for i := d.keyStateOffset; i < len(keyBuffer); i++ {
				keyIndex := uint8(i - d.keyStateOffset)
				if keyBuffer[i] != d.keyState[keyIndex] {
					kch <- Key{
						Index:   d.translateKeyIndex(keyIndex, d.Columns),
						Pressed: keyBuffer[i] == 1,
					}
				}
			}
		}
	}()

	return kch, nil
}

// read waits for the next input report. It carries on across a reopen, which
// closes the handle under a blocked read, and returns false once the device
// is closed or gone.
func (d *Device) read(b []byte) bool {
	for {
		d.ioMutex.Lock()
		device := d.device
		d.ioMutex.Unlock()
		if device == nil {
			return false
		}

		if _, err := device.Read(b); err == nil {
			return true
		}

		// Locking waits out a reopen that is still under way.
		d.ioMutex.Lock()
		reopened := d.device != nil && d.device != device
		d.ioMutex.Unlock()
		if !reopened {
			return false
		}
	}
}

// Sleep puts the device asleep, waiting for a key event to wake it up.
func (d *Device) Sleep() error {
	d.sleepMutex.Lock()
	defer d.sleepMutex.Unlock()

	d.preSleepBrightness = d.brightness

	if err := d.fade(d.brightness, 0, d.fadeDuration); err != nil {
		return err
	}

	d.asleep = true
	return d.setBrightness(0)
}

// Wake wakes the device from sleep.
func (d *Device) Wake() error {
	d.sleepMutex.Lock()
	defer d.sleepMutex.Unlock()

	d.asleep = false
	if err := d.fade(0, d.preSleepBrightness, d.fadeDuration); err != nil {
		return err
	}

	d.lastActionTime = time.Now()
	return d.setBrightness(d.preSleepBrightness)
}

// Asleep returns true if the device is asleep.
func (d *Device) Asleep() bool {
	d.sleepMutex.RLock()
	defer d.sleepMutex.RUnlock()
	return d.asleep
}

func (d *Device) cancelSleepTimer() {
	if d.sleepCancel == nil {
		return
	}

	d.sleepCancel()
	d.sleepCancel = nil
}

// SetSleepFadeDuration sets the duration of the fading animation when the
// device is put to sleep or wakes up.
func (d *Device) SetSleepFadeDuration(t time.Duration) {
	d.fadeDuration = t
}

// SetSleepTimeout sets the time after which the device will sleep if no key
// events are received.
func (d *Device) SetSleepTimeout(t time.Duration) {
	d.cancelSleepTimer()
	if t == 0 {
		return
	}

	var ctx context.Context
	ctx, d.sleepCancel = context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case <-time.After(time.Second):
				d.sleepMutex.RLock()
				since := time.Since(d.lastActionTime)
				asleep := d.asleep
				d.sleepMutex.RUnlock()

				if !asleep && since >= t {
					_ = d.Sleep()
				}

			case <-ctx.Done():
				return
			}
		}
	}()
}

// Fade fades the brightness in or out.
func (d *Device) Fade(start uint8, end uint8, duration time.Duration) error {
	d.sleepMutex.Lock()
	defer d.sleepMutex.Unlock()
	return d.fade(start, end, duration)
}

// fade is Fade for a caller holding sleepMutex.
func (d *Device) fade(start uint8, end uint8, duration time.Duration) error {
	step := (float64(end) - float64(start)) / float64(duration/fadeDelay)
	if step == math.Inf(1) || step == math.Inf(-1) {
		return nil
	}

	for current := float64(start); ; current += step {
		if !((start < end && int8(current) < int8(end)) ||
			(start > end && int8(current) > int8(end))) {
			break
		}
		if err := d.setBrightness(uint8(current)); err != nil {
			return err
		}

		time.Sleep(fadeDelay)
	}
	return nil
}

// SetBrightness sets the background lighting brightness from 0 to 100 percent.
func (d *Device) SetBrightness(percent uint8) error {
	d.sleepMutex.Lock()
	defer d.sleepMutex.Unlock()
	return d.setBrightness(percent)
}

// setBrightness is SetBrightness for a caller holding sleepMutex.
func (d *Device) setBrightness(percent uint8) error {
	if percent > 100 {
		percent = 100
	}

	d.brightness = percent
	if d.asleep && percent > 0 {
		// if the device is asleep, remember the brightness, but don't set it
		d.preSleepBrightness = percent
		return nil
	}

	report := make([]byte, len(d.setBrightnessCommand)+1)
	copy(report, d.setBrightnessCommand)
	report[len(report)-1] = percent

	return d.sendFeatureReport(report)
}

// SetImage sets the image of a button on the Stream Deck. The provided image
// needs to be in the correct resolution for the device. The index starts with
// 0 being the top-left button.
func (d *Device) SetImage(index uint8, img image.Image) error {
	if img.Bounds().Dy() != int(d.Pixels) ||
		img.Bounds().Dx() != int(d.Pixels) {
		return fmt.Errorf("supplied image has wrong dimensions, expected %[1]dx%[1]d pixels", d.Pixels)
	}

	imageBytes, err := d.toImageFormat(d.flipImage(img))
	if err != nil {
		return fmt.Errorf("cannot convert image data: %v", err)
	}
	imageData := imageData{
		image:    imageBytes,
		pageSize: d.imagePageSize - d.imagePageHeaderSize,
	}
	key := d.translateKeyIndex(index, d.Columns)

	// A retry starts over from page 0.
	return d.do(func(device hidHandle) error {
		return d.writeImage(device, key, imageData)
	})
}

// writeImage sends every page of an image to the device.
func (d *Device) writeImage(device hidHandle, key uint8, imageData imageData) error {
	data := make([]byte, d.imagePageSize)

	var page int
	var lastPage bool
	for !lastPage {
		var payload []byte
		payload, lastPage = imageData.Page(page)
		header := d.imagePageHeader(page, key, len(payload), lastPage)

		copy(data, header)
		copy(data[len(header):], payload)

		_, err := device.Write(data)
		if err != nil {
			return fmt.Errorf("cannot write image page %d of %d (%d image bytes) %d bytes: %w",
				page, imageData.PageCount(), imageData.Length(), len(data), err)
		}

		page++
	}

	return nil
}

// getFeatureReport from the device without worries about the correct payload
// size.
func (d *Device) getFeatureReport(payload []byte) ([]byte, error) {
	b := make([]byte, d.featureReportSize)
	err := d.do(func(device hidHandle) error {
		copy(b, payload)
		_, err := device.GetFeatureReport(b)
		return err
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// sendFeatureReport to the device without worries about the correct payload
// size.
func (d *Device) sendFeatureReport(payload []byte) error {
	b := make([]byte, d.featureReportSize)
	copy(b, payload)
	return d.do(func(device hidHandle) error {
		_, err := device.SendFeatureReport(b)
		return err
	})
}

// translateRightToLeft translates the given key index from right-to-left to
// left-to-right, based on the given number of columns.
func translateRightToLeft(index, columns uint8) uint8 {
	keyCol := index % columns
	return (index - keyCol) + (columns - 1) - keyCol
}

// identity returns the given key index as it is.
func identity(index, _ uint8) uint8 {
	return index
}

// toRGBA converts an image.Image to an image.RGBA.
func toRGBA(img image.Image) *image.RGBA {
	switch img := img.(type) {
	case *image.RGBA:
		return img
	}
	out := image.NewRGBA(img.Bounds())
	draw.Copy(out, image.Pt(0, 0), img, img.Bounds(), draw.Src, nil)
	return out
}

// flipHorizontally returns the given image horizontally flipped.
func flipHorizontally(img image.Image) image.Image {
	flipped := image.NewRGBA(img.Bounds())
	draw.Copy(flipped, image.Point{}, img, img.Bounds(), draw.Src, nil)
	for y := 0; y < flipped.Bounds().Dy(); y++ {
		for x := 0; x < flipped.Bounds().Dx()/2; x++ {
			xx := flipped.Bounds().Max.X - x - 1
			c := flipped.RGBAAt(x, y)
			flipped.SetRGBA(x, y, flipped.RGBAAt(xx, y))
			flipped.SetRGBA(xx, y, c)
		}
	}
	return flipped
}

// flipHorizontallyAndVertically returns the given image horizontally and
// vertically flipped.
func flipHorizontallyAndVertically(img image.Image) image.Image {
	flipped := image.NewRGBA(img.Bounds())
	draw.Copy(flipped, image.Point{}, img, img.Bounds(), draw.Src, nil)
	for y := 0; y < flipped.Bounds().Dy()/2; y++ {
		yy := flipped.Bounds().Max.Y - y - 1
		for x := 0; x < flipped.Bounds().Dx(); x++ {
			xx := flipped.Bounds().Max.X - x - 1
			c := flipped.RGBAAt(x, y)
			flipped.SetRGBA(x, y, flipped.RGBAAt(xx, yy))
			flipped.SetRGBA(xx, yy, c)
		}
	}
	return flipped
}

// rotateCounterclockwise returns the given image rotated counterclockwise.
func rotateCounterclockwise(img image.Image) image.Image {
	flipped := image.NewRGBA(img.Bounds())
	draw.Copy(flipped, image.Point{}, img, img.Bounds(), draw.Src, nil)
	for y := 0; y < flipped.Bounds().Dy(); y++ {
		for x := y + 1; x < flipped.Bounds().Dx(); x++ {
			c := flipped.RGBAAt(x, y)
			flipped.SetRGBA(x, y, flipped.RGBAAt(y, x))
			flipped.SetRGBA(y, x, c)
		}
	}
	for y := 0; y < flipped.Bounds().Dy()/2; y++ {
		yy := flipped.Bounds().Max.Y - y - 1
		for x := 0; x < flipped.Bounds().Dx(); x++ {
			c := flipped.RGBAAt(x, y)
			flipped.SetRGBA(x, y, flipped.RGBAAt(x, yy))
			flipped.SetRGBA(x, yy, c)
		}
	}
	return flipped
}

// toBMP returns the raw bytes of the given image in BMP format.
func toBMP(img image.Image) ([]byte, error) {
	rgba := toRGBA(img)

	// this is a BMP file header followed by a BPM bitmap info header
	// find more information here: https://en.wikipedia.org/wiki/BMP_file_format
	header := []byte{
		0x42, 0x4d, 0xf6, 0x3c, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x36, 0x00, 0x00, 0x00, 0x28, 0x00,
		0x00, 0x00, 0x48, 0x00, 0x00, 0x00, 0x48, 0x00,
		0x00, 0x00, 0x01, 0x00, 0x18, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xc0, 0x3c, 0x00, 0x00, 0xc4, 0x0e,
		0x00, 0x00, 0xc4, 0x0e, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	buffer := make([]byte, len(header)+rgba.Bounds().Dx()*rgba.Bounds().Dy()*3)
	copy(buffer, header)

	i := len(header)
	for y := 0; y < rgba.Bounds().Dy(); y++ {
		for x := 0; x < rgba.Bounds().Dx(); x++ {
			c := rgba.RGBAAt(x, y)
			buffer[i] = c.B
			buffer[i+1] = c.G
			buffer[i+2] = c.R
			i += 3
		}
	}
	return buffer, nil
}

// toJPEG returns the raw bytes of the given image in JPEG format.
func toJPEG(img image.Image) ([]byte, error) {
	buffer := bytes.NewBuffer([]byte{})
	opts := jpeg.Options{
		Quality: 100,
	}
	err := jpeg.Encode(buffer, img, &opts)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), err
}

// rev1ImagePageHeader returns the image page header sequence used by the
// Stream Deck v1.
func rev1ImagePageHeader(pageIndex int, keyIndex uint8, payloadLength int, lastPage bool) []byte {
	var lastPageByte byte
	if lastPage {
		lastPageByte = 1
	}
	return []byte{
		0x02, 0x01,
		byte(pageIndex + 1), 0x00,
		lastPageByte,
		keyIndex + 1,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

// miniImagePageHeader returns the image page header sequence used by the
// Stream Deck Mini.
func miniImagePageHeader(pageIndex int, keyIndex uint8, payloadLength int, lastPage bool) []byte {
	var lastPageByte byte
	if lastPage {
		lastPageByte = 1
	}
	return []byte{
		0x02, 0x01,
		byte(pageIndex), 0x00,
		lastPageByte,
		keyIndex + 1,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

// rev2ImagePageHeader returns the image page header sequence used by Stream
// Deck XL and Stream Deck v2.
func rev2ImagePageHeader(pageIndex int, keyIndex uint8, payloadLength int, lastPage bool) []byte {
	var lastPageByte byte
	if lastPage {
		lastPageByte = 1
	}
	return []byte{
		0x02, 0x07, keyIndex, lastPageByte,
		byte(payloadLength), byte(payloadLength >> 8),
		byte(pageIndex), byte(pageIndex >> 8),
	}
}

// imageData allows to access raw image data in a byte array through pages of a
// given size.
type imageData struct {
	image    []byte
	pageSize int
}

// Page returns the page with the given index and an indication if this is the
// last page.
func (d imageData) Page(pageIndex int) ([]byte, bool) {
	offset := pageIndex * d.pageSize
	if offset >= len(d.image) {
		return []byte{}, true
	}

	length := d.pageLength(pageIndex)
	if offset+length > len(d.image) {
		length = len(d.image) - offset
	}

	return d.image[offset : offset+length], pageIndex == d.PageCount()-1
}

func (d imageData) pageLength(pageIndex int) int {
	remaining := len(d.image) - (pageIndex * d.pageSize)
	if remaining > d.pageSize {
		return d.pageSize
	}
	if remaining > 0 {
		return remaining
	}
	return 0
}

// PageCount returns the total number of pages.
func (d imageData) PageCount() int {
	count := len(d.image) / d.pageSize
	if len(d.image)%d.pageSize != 0 {
		return count + 1
	}
	return count
}

// Length of the raw image data in bytes.
func (d imageData) Length() int {
	return len(d.image)
}
