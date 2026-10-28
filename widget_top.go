package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// TopWidget is a widget displaying the current CPU/MEM/GPU usage as bars.
// `mode` holds one mode per bar, split on `;`, and so does `fillColor`: two
// modes share the key as two columns.
type TopWidget struct {
	*BaseWidget

	modes      []string
	color      color.Color
	fillColors []color.Color

	// sysfs directory of the GPU the gpu and vram modes read, found on the
	// first update
	gpuDevice string
	// whether the missing GPU has been reported
	noGPUReported bool

	lastValues []float64
}

// defaultFillColor fills a bar that has no fillColor of its own.
var defaultFillColor = color.RGBA{166, 155, 182, 255}

// NewTopWidget returns a new TopWidget.
func NewTopWidget(bw *BaseWidget, opts WidgetConfig) *TopWidget {
	bw.setInterval(time.Duration(opts.Interval)*time.Millisecond, time.Second/2)

	var modes []string
	_ = ConfigValue(opts.Config["mode"], &modes)
	var textColor color.Color
	_ = ConfigValue(opts.Config["color"], &textColor)
	var fillColors []color.Color
	_ = ConfigValue(opts.Config["fillColor"], &fillColors)

	if textColor == nil {
		textColor = DefaultColor
	}
	for len(fillColors) < len(modes) {
		fillColors = append(fillColors, defaultFillColor)
	}

	w := &TopWidget{
		BaseWidget: bw,
		modes:      modes,
		color:      textColor,
		fillColors: fillColors,
		lastValues: make([]float64, len(modes)),
	}
	w.Repaint()
	return w
}

// Repaint makes the widget draw again on the next update, even if no value
// has changed since it last drew.
func (w *TopWidget) Repaint() {
	w.BaseWidget.Repaint()
	for i := range w.lastValues {
		w.lastValues[i] = math.NaN()
	}
}

// usage returns the current value of a mode as a percentage, and the label
// its bar carries.
func (w *TopWidget) usage(mode string) (float64, string, error) {
	switch mode {
	case "cpu":
		cpuUsage, err := cpu.Percent(0, false)
		if err != nil {
			return 0, "", fmt.Errorf("can't retrieve CPU usage: %s", err)
		}
		return cpuUsage[0], "CPU", nil

	case "memory":
		memory, err := mem.VirtualMemory()
		if err != nil {
			return 0, "", fmt.Errorf("can't retrieve memory usage: %s", err)
		}
		return memory.UsedPercent, "MEM", nil

	case "gpu", "vram":
		usage, err := w.gpuUsage(mode)
		return usage, strings.ToUpper(mode), err

	default:
		return 0, "", fmt.Errorf("unknown widget mode: %s", mode)
	}
}

// gpuUsage returns how busy the GPU is in gpu mode, or how much of its VRAM is
// in use in vram mode, as a percentage.
func (w *TopWidget) gpuUsage(mode string) (float64, error) {
	if w.gpuDevice == "" {
		device, err := findAMDGPU()
		if err != nil {
			return 0, err
		}
		w.gpuDevice = device
	}

	if mode == "gpu" {
		busy, err := readSysfsUint(filepath.Join(w.gpuDevice, "gpu_busy_percent"))
		return float64(busy), err
	}

	used, err := readSysfsUint(filepath.Join(w.gpuDevice, "mem_info_vram_used"))
	if err != nil {
		return 0, err
	}
	total, err := readSysfsUint(filepath.Join(w.gpuDevice, "mem_info_vram_total"))
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, fmt.Errorf("%s reports no VRAM", w.gpuDevice)
	}
	return float64(used) * 100 / float64(total), nil
}

// errNoGPU is returned for the gpu and vram modes on a machine without an
// amdgpu card.
var errNoGPU = errors.New("no amdgpu card found in /sys/class/drm")

// findAMDGPU returns the sysfs device directory of the amdgpu card with the
// most VRAM, which is the discrete one when an integrated GPU sits beside it.
func findAMDGPU() (string, error) {
	totals, err := filepath.Glob("/sys/class/drm/card*/device/mem_info_vram_total")
	if err != nil {
		return "", err
	}

	var device string
	var most uint64
	for _, path := range totals {
		total, err := readSysfsUint(path)
		if err != nil || total <= most {
			continue
		}
		device, most = filepath.Dir(path), total
	}
	if device == "" {
		return "", errNoGPU
	}
	return device, nil
}

// readSysfsUint reads a sysfs attribute that holds one unsigned number.
func readSysfsUint(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

// Update renders the widget.
func (w *TopWidget) Update() error {
	if len(w.modes) == 0 {
		return fmt.Errorf("top widget needs a mode")
	}

	values := make([]float64, len(w.modes))
	labels := make([]string, len(w.modes))
	for i, mode := range w.modes {
		value, label, err := w.usage(mode)
		switch {
		case errors.Is(err, errNoGPU):
			// A machine without an amdgpu card, a Mac among them, draws the
			// bar empty.
			if !w.noGPUReported {
				fmt.Fprintf(os.Stderr, "Can't retrieve GPU usage: %s\n", err)
				w.noGPUReported = true
			}
			value = math.Inf(-1)

		case err != nil:
			if mode != "gpu" && mode != "vram" {
				return err
			}
			// A GPU that fails to answer keeps its last frame rather than
			// taking the daemon down, and is asked again after the interval.
			fmt.Fprintf(os.Stderr, "Can't retrieve GPU usage: %s\n", err)
			w.lastUpdate = time.Now()
			return nil
		}
		values[i], labels[i] = value, label
	}

	if equalValues(values, w.lastValues) {
		// Nothing to draw, so wait out the interval before asking again.
		w.lastUpdate = time.Now()
		return nil
	}
	w.lastValues = values

	size := int(w.dev.Pixels)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i, frame := range meterColumns(size, len(values)) {
		m := meter{
			fill:      values[i],
			value:     strconv.FormatInt(int64(values[i]), 10),
			label:     labels[i],
			color:     w.color,
			fillColor: w.fillColors[i],
		}
		if math.IsInf(values[i], -1) {
			m.value = "–"
		}
		if len(values) == 1 {
			m.label = "% " + m.label
		}
		m.draw(img, frame, w.dev.DPI)
	}

	return w.render(w.dev, img)
}

// equalValues reports whether two readings hold the same values. NaN, which
// Repaint leaves behind, equals nothing.
func equalValues(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// meter is one bar on a key: a frame filled from the bottom up to `fill`
// percent, with `value` inside it and `label` underneath.
type meter struct {
	fill      float64
	value     string
	label     string
	color     color.Color
	fillColor color.Color
}

// meterColumns splits a key of `size` pixels into `count` columns of equal
// width, one meter each.
func meterColumns(size, count int) []image.Rectangle {
	frames := make([]image.Rectangle, count)
	for i := range frames {
		frames[i] = image.Rect(size*i/count, 0, size*(i+1)/count, size)
	}
	return frames
}

// draw paints the meter into `frame`, a column of img. A full-width frame
// keeps upstream's single bar: 48 px wide with the value at 13 pt. A narrower
// frame gets a slimmer bar and a smaller value.
func (m meter) draw(img *image.RGBA, frame image.Rectangle, dpi uint) {
	width, size := frame.Dx(), frame.Dy()
	pad, valueSize := width/6, 13.0
	if width < size {
		pad, valueSize = width/7, 10
	}

	bar := image.Rect(frame.Min.X+pad, frame.Min.Y+6, frame.Max.X-pad, frame.Max.Y-18)
	inner := bar.Inset(1)
	fill := inner.Inset(1)
	level := math.Max(0, math.Min(100, m.fill))
	fill.Min.Y = fill.Max.Y - int(math.Round(float64(fill.Dy())*level/100))

	draw.Draw(img, bar, &image.Uniform{m.color}, image.Point{}, draw.Src)
	draw.Draw(img, inner, &image.Uniform{color.RGBA{0, 0, 0, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, fill, &image.Uniform{m.fillColor}, image.Point{}, draw.Src)

	drawString(img, bar, ttfFont, m.value, dpi, valueSize, m.color, image.Pt(-1, -1))

	caption := image.Rect(frame.Min.X, frame.Max.Y-16, frame.Max.X, frame.Max.Y-size/18)
	drawString(img, caption, ttfFont, m.label, dpi, -1, m.color, image.Pt(-1, -1))
}
