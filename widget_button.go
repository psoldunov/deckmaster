package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"time"
)

// ButtonWidget is a simple widget displaying an icon and/or label.
type ButtonWidget struct {
	*BaseWidget

	icon        image.Image
	iconPath    string
	iconCommand string
	label       string
	fontsize    float64
	color       color.Color
	flatten     bool
}

// NewButtonWidget returns a new ButtonWidget.
func NewButtonWidget(bw *BaseWidget, opts WidgetConfig) (*ButtonWidget, error) {
	var icon, iconCommand, label string
	_ = ConfigValue(opts.Config["icon"], &icon)
	_ = ConfigValue(opts.Config["iconCommand"], &iconCommand)
	_ = ConfigValue(opts.Config["label"], &label)

	// A button whose icon follows a command is repainted every second unless
	// the widget sets its own interval.
	var defaultInterval time.Duration
	if iconCommand != "" {
		defaultInterval = time.Second
	}
	bw.setInterval(time.Duration(opts.Interval)*time.Millisecond, defaultInterval)
	var fontsize float64
	_ = ConfigValue(opts.Config["fontsize"], &fontsize)
	var color color.Color
	_ = ConfigValue(opts.Config["color"], &color)
	var flatten bool
	_ = ConfigValue(opts.Config["flatten"], &flatten)

	if color == nil {
		color = DefaultColor
	}

	w := &ButtonWidget{
		BaseWidget:  bw,
		iconPath:    icon,
		iconCommand: iconCommand,
		label:       label,
		fontsize:    fontsize,
		color:       color,
		flatten:     flatten,
	}
	if icon != "" {
		if err := w.LoadImage(icon); err != nil {
			return nil, err
		}
	}

	return w, nil
}

// refreshIcon runs iconCommand and switches to the image file its output
// names. A failing command or an unreadable image keeps the current icon, so a
// missing media player cannot take the daemon down.
func (w *ButtonWidget) refreshIcon() {
	out, err := runCommand(w.iconCommand)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iconCommand failed: %s\n", err)
		return
	}

	path := strings.TrimSpace(out)
	if path == "" || path == w.iconPath {
		return
	}
	if err := w.LoadImage(path); err != nil {
		fmt.Fprintf(os.Stderr, "Can't load icon %s: %s\n", path, err)
		return
	}
	w.iconPath = path
}

// LoadImage loads an image from disk.
func (w *ButtonWidget) LoadImage(path string) error {
	path, err := expandPath(w.base, path)
	if err != nil {
		return err
	}
	icon, err := loadImage(path)
	if err != nil {
		return err
	}

	w.SetImage(icon)
	return nil
}

// SetImage updates the widget's icon.
func (w *ButtonWidget) SetImage(img image.Image) {
	w.icon = img
	if w.flatten {
		w.icon = flattenImage(w.icon, w.color)
	}
}

// Update renders the widget.
func (w *ButtonWidget) Update() error {
	if w.iconCommand != "" {
		w.refreshIcon()
	}

	size := int(w.dev.Pixels)
	margin := size / 18
	height := size - (margin * 2)
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	if w.label != "" {
		iconsize := int((float64(height) / 3.0) * 2.0)
		bounds := img.Bounds()

		if w.icon != nil {
			err := drawImage(img,
				w.icon,
				iconsize,
				image.Pt(-1, margin))

			if err != nil {
				return err
			}

			bounds.Min.Y += iconsize + margin
			bounds.Max.Y -= margin
		}

		drawString(img,
			bounds,
			ttfFont,
			w.label,
			w.dev.DPI,
			w.fontsize,
			w.color,
			image.Pt(-1, -1))
	} else if w.icon != nil {
		err := drawImage(img,
			w.icon,
			height,
			image.Pt(-1, -1))

		if err != nil {
			return err
		}
	}

	return w.render(w.dev, img)
}
