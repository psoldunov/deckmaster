package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	// warnFillColor and critFillColor fill the bar once a sensor passes its
	// warning or critical temperature.
	warnFillColor = color.RGBA{246, 116, 0, 255}
	critFillColor = color.RGBA{218, 68, 83, 255}
)

const (
	// defaultCritTemp is the full bar for a sensor that reports no critical
	// temperature of its own.
	defaultCritTemp = 100.0
	// maxPlausibleTemp bounds the thresholds a sensor reports; NVMe drives
	// report 65261.85 °C for sensors without one.
	maxPlausibleTemp = 150.0
)

// TempWidget shows one hwmon temperature sensor as a bar that fills towards
// the sensor's critical temperature.
type TempWidget struct {
	*BaseWidget

	label string
	// the hwmon chip (its `name`), the sensor on it (its `temp*_label`, or
	// empty for an unlabelled one) and, for a chip with several instances, a
	// part of the chip's resolved sysfs device path
	chip, sensor, device string
	// warning and critical temperatures in °C from the config, or zero to
	// take the sensor's own
	warn, crit float64

	color     color.Color
	fillColor color.Color

	// the sensor's temp*_input and its thresholds, found on the first update
	input                string
	inputWarn, inputCrit float64

	lastValue float64
	lastError string
}

// NewTempWidget returns a new TempWidget.
func NewTempWidget(bw *BaseWidget, opts WidgetConfig) *TempWidget {
	bw.setInterval(time.Duration(opts.Interval)*time.Millisecond, time.Second)

	w := &TempWidget{BaseWidget: bw}
	_ = ConfigValue(opts.Config["label"], &w.label)
	_ = ConfigValue(opts.Config["chip"], &w.chip)
	_ = ConfigValue(opts.Config["sensor"], &w.sensor)
	_ = ConfigValue(opts.Config["device"], &w.device)
	_ = ConfigValue(opts.Config["warn"], &w.warn)
	_ = ConfigValue(opts.Config["crit"], &w.crit)
	_ = ConfigValue(opts.Config["color"], &w.color)
	_ = ConfigValue(opts.Config["fillColor"], &w.fillColor)

	if w.label == "" {
		w.label = w.sensor
	}
	if w.color == nil {
		w.color = DefaultColor
	}
	if w.fillColor == nil {
		w.fillColor = defaultFillColor
	}

	w.Repaint()
	return w
}

// Repaint makes the widget draw again on the next update, even if the
// temperature has not changed since it last drew.
func (w *TempWidget) Repaint() {
	w.BaseWidget.Repaint()
	w.lastValue = math.NaN()
}

// Update renders the widget.
func (w *TempWidget) Update() error {
	temp, err := w.read()
	if err != nil {
		// A sensor that is missing or fails to answer shows a dash rather
		// than taking the daemon down, and is looked up again after the
		// interval, since hwmon numbers change when a driver reloads.
		w.input = ""
		if msg := err.Error(); msg != w.lastError {
			fmt.Fprintf(os.Stderr, "Can't retrieve temperature: %s\n", msg)
			w.lastError = msg
		}
		temp = math.Inf(-1)
	} else {
		w.lastError = ""
	}

	if temp == w.lastValue {
		// Nothing to draw, so wait out the interval before asking again.
		w.lastUpdate = time.Now()
		return nil
	}

	m := meter{
		value:     "–",
		label:     w.label,
		color:     w.color,
		fillColor: w.fillColor,
	}
	if !math.IsInf(temp, -1) {
		warn, crit := w.thresholds()
		m.fill = temp * 100 / crit
		m.value = strconv.Itoa(int(math.Round(temp))) + "°"
		switch {
		case temp >= crit:
			m.fillColor = critFillColor
		case warn > 0 && temp >= warn:
			m.fillColor = warnFillColor
		}
	}

	size := int(w.dev.Pixels)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	m.draw(img, img.Bounds(), w.dev.DPI)

	// A value that did not reach the deck is drawn again next time.
	if err := w.render(w.dev, img); err != nil {
		return err
	}
	w.lastValue = temp
	return nil
}

// read returns the sensor's temperature in °C, finding the sensor first if
// it has not been found yet.
func (w *TempWidget) read() (float64, error) {
	if w.input == "" {
		input, err := findTempInput(w.chip, w.sensor, w.device)
		if err != nil {
			return 0, err
		}
		w.input = input
		w.inputWarn = readTempThreshold(input, "_max")
		w.inputCrit = readTempThreshold(input, "_crit")
	}

	millidegrees, err := readSysfsInt(w.input)
	if err != nil {
		return 0, err
	}
	return float64(millidegrees) / 1000, nil
}

// thresholds returns the warning and critical temperatures: the config's,
// else the sensor's, else no warning and a critical temperature of 100 °C.
func (w *TempWidget) thresholds() (float64, float64) {
	warn, crit := w.warn, w.crit
	if warn <= 0 {
		warn = w.inputWarn
	}
	if crit <= 0 {
		crit = w.inputCrit
	}
	if crit <= 0 {
		crit = defaultCritTemp
	}
	if warn >= crit {
		warn = 0
	}
	return warn, crit
}

// findTempInput returns the temp*_input file of the hwmon sensor labelled
// `sensor` on the chip named `chip`, or of its unlabelled sensor when `sensor`
// is empty. A non-empty `device` must be part of the chip's resolved device
// path, which tells apart two chips of the same kind, such as two NVMe drives.
func findTempInput(chip, sensor, device string) (string, error) {
	dirs, err := filepath.Glob("/sys/class/hwmon/hwmon*")
	if err != nil {
		return "", err
	}

	for _, dir := range dirs {
		name, err := readSysfsString(filepath.Join(dir, "name"))
		if err != nil || name != chip {
			continue
		}
		if device != "" {
			path, err := filepath.EvalSymlinks(filepath.Join(dir, "device"))
			if err != nil || !strings.Contains(path, device) {
				continue
			}
		}

		inputs, err := filepath.Glob(filepath.Join(dir, "temp*_input"))
		if err != nil {
			return "", err
		}
		for _, input := range inputs {
			label, _ := readSysfsString(strings.TrimSuffix(input, "_input") + "_label")
			if label == sensor {
				return input, nil
			}
		}
	}

	if device != "" {
		return "", fmt.Errorf("no %s sensor %q on a device matching %q", chip, sensor, device)
	}
	return "", fmt.Errorf("no %s sensor %q", chip, sensor)
}

// readTempThreshold reads the threshold with `suffix` (_max, _crit) beside a
// temp*_input file in °C, or returns zero if the sensor has no plausible one.
func readTempThreshold(input, suffix string) float64 {
	millidegrees, err := readSysfsInt(strings.TrimSuffix(input, "_input") + suffix)
	if err != nil {
		return 0
	}
	temp := float64(millidegrees) / 1000
	if temp <= 0 || temp > maxPlausibleTemp {
		return 0
	}
	return temp
}

// readSysfsInt reads a sysfs attribute that holds one signed number.
func readSysfsInt(path string) (int64, error) {
	data, err := readSysfsString(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(data, 10, 64)
}

// readSysfsString reads a sysfs attribute without its trailing newline.
func readSysfsString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
