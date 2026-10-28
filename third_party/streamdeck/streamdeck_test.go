package streamdeck

import (
	"errors"
	"image"
	"image/color"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errFake = errors.New("fake write failure")

// report is one transfer that reached the fake deck.
type report struct {
	feature bool
	data    []byte
}

// fakeDeck records what reaches it through every handle it hands out, and
// notices when two transfers overlap.
type fakeDeck struct {
	mu      sync.Mutex
	reports []report
	handles []*fakeHandle
	// failWrites is how many writes each new handle fails, -1 for all.
	failWrites int
	openErr    error

	busy    int32
	overlap int32
}

func (f *fakeDeck) open() (hidHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.openErr != nil {
		return nil, f.openErr
	}
	h := &fakeHandle{
		deck:       f,
		failWrites: f.failWrites,
		input:      make(chan []byte, 1),
		closed:     make(chan struct{}),
	}
	f.handles = append(f.handles, h)
	return h, nil
}

func (f *fakeDeck) transfer(feature bool, b []byte) {
	if atomic.AddInt32(&f.busy, 1) > 1 {
		atomic.StoreInt32(&f.overlap, 1)
	}
	time.Sleep(20 * time.Microsecond) // widen the window for an overlap
	f.mu.Lock()
	f.reports = append(f.reports, report{feature: feature, data: append([]byte(nil), b...)})
	f.mu.Unlock()
	atomic.AddInt32(&f.busy, -1)
}

func (f *fakeDeck) log() []report {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]report(nil), f.reports...)
}

type fakeHandle struct {
	deck       *fakeDeck
	failWrites int
	input      chan []byte
	closed     chan struct{}
	closeOnce  sync.Once
}

func (h *fakeHandle) Read(b []byte) (int, error) {
	select {
	case r := <-h.input:
		return copy(b, r), nil
	case <-h.closed:
		return 0, errors.New("closed")
	}
}

func (h *fakeHandle) Write(b []byte) (int, error) {
	h.deck.mu.Lock()
	fail := h.failWrites != 0
	if h.failWrites > 0 {
		h.failWrites--
	}
	h.deck.mu.Unlock()
	if fail {
		return 0, errFake
	}
	h.deck.transfer(false, b)
	return len(b), nil
}

func (h *fakeHandle) SendFeatureReport(b []byte) (int, error) {
	h.deck.transfer(true, b)
	return len(b), nil
}

func (h *fakeHandle) GetFeatureReport(b []byte) (int, error) {
	h.deck.transfer(true, b)
	return len(b), nil
}

func (h *fakeHandle) Close() error {
	h.closeOnce.Do(func() { close(h.closed) })
	return nil
}

func (h *fakeHandle) isClosed() bool {
	select {
	case <-h.closed:
		return true
	default:
		return false
	}
}

// newFakeMK2 returns an open Stream Deck MK.2 wired to a fake deck.
func newFakeMK2(t *testing.T, f *fakeDeck) *Device {
	t.Helper()
	enumerate := hidEnumerate
	hidEnumerate = func(uint16, uint16) []hidDeviceInfo {
		return []hidDeviceInfo{{Path: "fake", VendorID: VID_ELGATO, ProductID: PID_STREAMDECK_MK2, Serial: "FAKE"}}
	}
	t.Cleanup(func() { hidEnumerate = enumerate })

	devs, err := Devices()
	if err != nil || len(devs) != 1 {
		t.Fatalf("Devices() = %d devices, %v", len(devs), err)
	}
	dev := &devs[0]
	dev.open = f.open
	if err := dev.Open(); err != nil {
		t.Fatal(err)
	}
	return dev
}

// noise returns an image that encodes to several pages.
func noise(size int, seed int64) image.Image {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // test pattern
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255})
		}
	}
	return img
}

// page decodes an MK.2 image page header.
type page struct {
	key   uint8
	last  bool
	index int
}

func decodePage(b []byte) page {
	return page{key: b[2], last: b[3] == 1, index: int(b[6]) | int(b[7])<<8}
}

// checkContiguous fails unless every image in log went out page after page,
// with nothing in between, and returns how many complete images it saw.
func checkContiguous(t *testing.T, log []report) int {
	t.Helper()
	images := 0
	for i := 0; i < len(log); {
		if log[i].feature {
			i++
			continue
		}
		first := decodePage(log[i].data)
		if first.index != 0 {
			t.Fatalf("report %d: image for key %d starts at page %d", i, first.key, first.index)
		}
		for n := 0; ; n++ {
			if i >= len(log) || log[i].feature {
				t.Fatalf("report %d: image for key %d cut off after page %d", i, first.key, n-1)
			}
			p := decodePage(log[i].data)
			if p.key != first.key || p.index != n {
				t.Fatalf("report %d: page %d of key %d inside page %d of key %d", i, p.index, p.key, n, first.key)
			}
			i++
			if p.last {
				break
			}
		}
		images++
	}
	return images
}

func TestConcurrentImagesStayWhole(t *testing.T) {
	f := &fakeDeck{}
	dev := newFakeMK2(t, f)

	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				if err := dev.SetImage(uint8(w), noise(int(dev.Pixels), int64(w*100+i))); err != nil {
					t.Error(err)
					return
				}
			}
		}(w)
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if err := dev.SetBrightness(uint8(i)); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		if err := dev.Fade(100, 0, 100*time.Millisecond); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()

	if atomic.LoadInt32(&f.overlap) != 0 {
		t.Error("two transfers reached the device at once")
	}
	if images := checkContiguous(t, f.log()); images != 80 {
		t.Errorf("saw %d whole images, want 80", images)
	}
}

func TestFailedImageStartsOverFromFirstPage(t *testing.T) {
	f := &fakeDeck{}
	dev := newFakeMK2(t, f)
	img := noise(int(dev.Pixels), 1)
	if err := dev.SetImage(3, img); err != nil {
		t.Fatal(err)
	}
	pages := len(f.log())
	if pages < 2 {
		t.Fatalf("image went out in %d pages, want several", pages)
	}

	// Let page 0 through, then fail page 1 once.
	h := f.handles[0]
	f.mu.Lock()
	f.reports = nil
	f.mu.Unlock()
	dev.ioMutex.Lock()
	dev.device = &secondWriteFails{fakeHandle: h}
	dev.ioMutex.Unlock()

	if err := dev.SetImage(3, img); err != nil {
		t.Fatal(err)
	}
	log := f.log()
	if len(log) != 1+pages {
		t.Fatalf("got %d pages, want page 0 and then the %d pages again", len(log), pages)
	}
	checkContiguous(t, log[1:])
	if len(f.handles) != 1 {
		t.Errorf("a single failure reopened the device")
	}
}

// secondWriteFails fails its second write.
type secondWriteFails struct {
	*fakeHandle
	writes int
}

func (h *secondWriteFails) Write(b []byte) (int, error) {
	h.writes++
	if h.writes == 2 {
		return 0, errFake
	}
	return h.fakeHandle.Write(b)
}

func TestFailingHandleIsReopened(t *testing.T) {
	f := &fakeDeck{failWrites: -1}
	dev := newFakeMK2(t, f)
	f.mu.Lock()
	f.failWrites = 0 // the reopened handle works
	f.mu.Unlock()

	if err := dev.SetImage(0, noise(int(dev.Pixels), 2)); err != nil {
		t.Fatal(err)
	}
	if len(f.handles) != 2 {
		t.Fatalf("opened %d handles, want 2", len(f.handles))
	}
	if !f.handles[0].isClosed() {
		t.Error("the failing handle was left open")
	}
	if images := checkContiguous(t, f.log()); images != 1 {
		t.Errorf("saw %d whole images, want 1", images)
	}
}

func TestKeyStillFailingAfterReopen(t *testing.T) {
	f := &fakeDeck{failWrites: -1}
	dev := newFakeMK2(t, f)

	err := dev.SetImage(0, noise(int(dev.Pixels), 3))
	if !errors.Is(err, errFake) || errors.Is(err, ErrDisconnected) {
		t.Fatalf("SetImage() = %v, want the write error", err)
	}
	if len(f.handles) != 2 {
		t.Errorf("opened %d handles, want 2", len(f.handles))
	}
	// The device stays open and the next call tries again.
	f.mu.Lock()
	f.handles[1].failWrites = 0
	f.mu.Unlock()
	if err := dev.SetImage(0, noise(int(dev.Pixels), 3)); err != nil {
		t.Fatal(err)
	}
}

func TestGoneWhenReopenFails(t *testing.T) {
	f := &fakeDeck{failWrites: -1}
	dev := newFakeMK2(t, f)
	keys, err := dev.ReadKeys()
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.openErr = errors.New("no such device")
	f.mu.Unlock()

	if err := dev.SetImage(0, noise(int(dev.Pixels), 4)); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("SetImage() = %v, want ErrDisconnected", err)
	}
	if err := dev.SetBrightness(50); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("SetBrightness() = %v, want ErrDisconnected", err)
	}
	select {
	case _, ok := <-keys:
		if ok {
			t.Fatal("got a key from a gone device")
		}
	case <-time.After(time.Second):
		t.Fatal("key channel still open")
	}
}

func TestReadErrorClosesKeys(t *testing.T) {
	f := &fakeDeck{}
	dev := newFakeMK2(t, f)
	keys, err := dev.ReadKeys()
	if err != nil {
		t.Fatal(err)
	}
	// What an unplug looks like to the reader.
	_ = f.handles[0].Close()

	select {
	case _, ok := <-keys:
		if ok {
			t.Fatal("got a key from an unplugged device")
		}
	case <-time.After(time.Second):
		t.Fatal("key channel still open")
	}
}

func TestReaderFollowsReopen(t *testing.T) {
	f := &fakeDeck{failWrites: -1}
	dev := newFakeMK2(t, f)
	keys, err := dev.ReadKeys()
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.failWrites = 0
	f.mu.Unlock()

	if err := dev.SetImage(0, noise(int(dev.Pixels), 5)); err != nil {
		t.Fatal(err)
	}
	pressed := make([]byte, dev.keyStateOffset+int(dev.Keys))
	pressed[dev.keyStateOffset+7] = 1
	f.handles[1].input <- pressed

	select {
	case k, ok := <-keys:
		if !ok || k.Index != 7 || !k.Pressed {
			t.Fatalf("got %+v (open %v), want key 7 pressed", k, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("no key from the reopened device")
	}
}

func TestCloseEndsKeys(t *testing.T) {
	f := &fakeDeck{}
	dev := newFakeMK2(t, f)
	keys, err := dev.ReadKeys()
	if err != nil {
		t.Fatal(err)
	}
	if err := dev.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-keys; ok {
		t.Fatal("got a key after Close")
	}
	if err := dev.Reset(); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("Reset() after Close = %v, want ErrDisconnected", err)
	}
}

func TestClearDrawsEveryKeyOnce(t *testing.T) {
	f := &fakeDeck{}
	dev := newFakeMK2(t, f)
	if err := dev.Clear(); err != nil {
		t.Fatal(err)
	}
	seen := map[uint8]bool{}
	for _, r := range f.log() {
		p := decodePage(r.data)
		if p.index == 0 {
			if p.key >= dev.Keys {
				t.Errorf("drew key %d on a %d-key deck", p.key, dev.Keys)
			}
			seen[p.key] = true
		}
	}
	if len(seen) != int(dev.Keys) {
		t.Errorf("drew %d keys, want %d", len(seen), dev.Keys)
	}
}
