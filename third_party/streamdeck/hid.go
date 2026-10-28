//go:build !darwin
// +build !darwin

package streamdeck

import "github.com/karalabe/hid"

type (
	hidDevice     = hid.Device
	hidDeviceInfo = hid.DeviceInfo
)

var hidEnumerate = hid.Enumerate
