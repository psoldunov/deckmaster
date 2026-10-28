package main

import (
	"fmt"
	"reflect"
)

// repainter is implemented by every widget through BaseWidget.
type repainter interface {
	Repaint()
}

// ToggleWidget shows one of several widgets on a single key. A tap switches to
// the next one, so a key can hold the time and the date, or two meters.
type ToggleWidget struct {
	*BaseWidget

	views   []Widget
	current int
}

// NewToggleWidget returns a new ToggleWidget.
func NewToggleWidget(bw *BaseWidget, opts WidgetConfig) (*ToggleWidget, error) {
	configs, err := toggleViewConfigs(opts.Config["widgets"])
	if err != nil {
		return nil, err
	}

	// Every view is built once, so a switch keeps what it has fetched and
	// starts nothing new in the background.
	views := make([]Widget, 0, len(configs))
	for _, c := range configs {
		v, err := NewWidget(bw.dev, bw.base, KeyConfig{Index: bw.key, Widget: c}, bw.background)
		if err != nil {
			return nil, fmt.Errorf("toggle widget: %w", err)
		}
		views = append(views, v)
	}

	return &ToggleWidget{
		BaseWidget: bw,
		views:      views,
	}, nil
}

// toggleViewConfigs reads the `widgets` list of a toggle widget. Each entry is
// a table shaped like a key's own `widget` table.
func toggleViewConfigs(v interface{}) ([]WidgetConfig, error) {
	var tables []map[string]interface{}
	switch vt := v.(type) {
	case []map[string]interface{}:
		tables = vt
	case []interface{}:
		for _, t := range vt {
			table, ok := t.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("toggle widget: each entry in widgets must be a table, not %v", reflect.TypeOf(t))
			}
			tables = append(tables, table)
		}
	default:
		return nil, fmt.Errorf("toggle widget: widgets must be a list of tables, not %v", reflect.TypeOf(v))
	}
	if len(tables) < 2 {
		return nil, fmt.Errorf("toggle widget: widgets needs at least two entries, has %d", len(tables))
	}

	configs := make([]WidgetConfig, 0, len(tables))
	for i, t := range tables {
		var c WidgetConfig
		if id, ok := t["id"].(string); ok && id != "" {
			c.ID = id
		} else {
			return nil, fmt.Errorf("toggle widget: entry %d needs an id", i)
		}
		if interval, ok := t["interval"]; ok {
			var ms int64
			if err := ConfigValue(interval, &ms); err != nil || ms < 0 {
				return nil, fmt.Errorf("toggle widget: entry %d has an invalid interval %v", i, interval)
			}
			c.Interval = uint(ms)
		}
		if config, ok := t["config"]; ok {
			table, ok := config.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("toggle widget: config of entry %d must be a table", i)
			}
			c.Config = table
		}
		configs = append(configs, c)
	}

	return configs, nil
}

// RequiresUpdate returns true when the shown widget wants to be repainted.
func (w *ToggleWidget) RequiresUpdate() bool {
	return w.views[w.current].RequiresUpdate()
}

// Update renders the shown widget.
func (w *ToggleWidget) Update() error {
	return w.views[w.current].Update()
}

// Repaint makes the shown widget draw again on the next update.
func (w *ToggleWidget) Repaint() {
	if r, ok := w.views[w.current].(repainter); ok {
		r.Repaint()
	}
}

// TriggerAction switches to the next widget on a tap. deckmaster calls it only
// for a key without an action of its own. A hold does nothing.
func (w *ToggleWidget) TriggerAction(hold bool) {
	if hold {
		return
	}

	w.current = (w.current + 1) % len(w.views)
	w.Repaint()
}
