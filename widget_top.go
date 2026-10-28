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

// TopWidget is a widget displaying the current CPU/MEM/GPU usage as a bar.
type TopWidget struct {
	*BaseWidget

	mode      string
	color     color.Color
	fillColor color.Color

	// sysfs directory of the GPU the gpu and vram modes read, found on the
	// first update
	gpuDevice string

	lastValue float64
}

// NewTopWidget returns a new TopWidget.
func NewTopWidget(bw *BaseWidget, opts WidgetConfig) *TopWidget {
	bw.setInterval(time.Duration(opts.Interval)*time.Millisecond, time.Second/2)

	var mode string
	_ = ConfigValue(opts.Config["mode"], &mode)
	var color, fillColor color.Color
	_ = ConfigValue(opts.Config["color"], &color)
	_ = ConfigValue(opts.Config["fillColor"], &fillColor)

	return &TopWidget{
		BaseWidget: bw,
		mode:       mode,
		color:      color,
		fillColor:  fillColor,
	}
}

// Repaint makes the widget draw again on the next update, even if the value
// has not changed since it last drew.
func (w *TopWidget) Repaint() {
	w.BaseWidget.Repaint()
	w.lastValue = math.NaN()
}

// gpuUsage returns how busy the GPU is in gpu mode, or how much of its VRAM is
// in use in vram mode, as a percentage.
func (w *TopWidget) gpuUsage() (float64, error) {
	if w.gpuDevice == "" {
		device, err := findAMDGPU()
		if err != nil {
			return 0, err
		}
		w.gpuDevice = device
	}

	if w.mode == "gpu" {
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
		return "", errors.New("no amdgpu card found in /sys/class/drm")
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
	var value float64
	var label string

	switch w.mode {
	case "cpu":
		cpuUsage, err := cpu.Percent(0, false)
		if err != nil {
			return fmt.Errorf("can't retrieve CPU usage: %s", err)
		}

		value = cpuUsage[0]
		label = "CPU"

	case "memory":
		memory, err := mem.VirtualMemory()
		if err != nil {
			return fmt.Errorf("can't retrieve memory usage: %s", err)
		}
		value = memory.UsedPercent
		label = "MEM"

	case "gpu", "vram":
		usage, err := w.gpuUsage()
		if err != nil {
			// A GPU that fails to answer keeps its last frame rather than
			// taking the daemon down, and is asked again after the interval.
			fmt.Fprintf(os.Stderr, "Can't retrieve GPU usage: %s\n", err)
			w.lastUpdate = time.Now()
			return nil
		}
		value = usage
		label = strings.ToUpper(w.mode)

	default:
		return fmt.Errorf("unknown widget mode: %s", w.mode)
	}

	if w.lastValue == value {
		// Nothing to draw, so wait out the interval before asking again.
		w.lastUpdate = time.Now()
		return nil
	}
	w.lastValue = value

	if w.color == nil {
		w.color = DefaultColor
	}
	if w.fillColor == nil {
		w.fillColor = color.RGBA{166, 155, 182, 255}
	}

	size := int(w.dev.Pixels)
	margin := size / 18
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	draw.Draw(img,
		image.Rect(12, 6, size-12, size-18),
		&image.Uniform{w.color},
		image.Point{}, draw.Src)
	draw.Draw(img,
		image.Rect(13, 7, size-14, size-20),
		&image.Uniform{color.RGBA{0, 0, 0, 255}},
		image.Point{}, draw.Src)
	draw.Draw(img,
		image.Rect(14, 7+int(float64(size-26)*(1-value/100)), size-15, size-21),
		&image.Uniform{w.fillColor},
		image.Point{}, draw.Src)

	// draw percentage
	bounds := img.Bounds()
	bounds.Min.Y = 6
	bounds.Max.Y -= 18

	drawString(img,
		bounds,
		ttfFont,
		strconv.FormatInt(int64(value), 10),
		w.dev.DPI,
		13,
		w.color,
		image.Pt(-1, -1))

	// draw description
	bounds = img.Bounds()
	bounds.Min.Y = size - 16
	bounds.Max.Y -= margin

	drawString(img,
		bounds,
		ttfFont,
		"% "+label,
		w.dev.DPI,
		-1,
		w.color,
		image.Pt(-1, -1))

	return w.render(w.dev, img)
}
