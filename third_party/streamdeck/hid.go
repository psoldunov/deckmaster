//go:build !darwin
// +build !darwin

package streamdeck

import "github.com/karalabe/hid"

type hidDeviceInfo = hid.DeviceInfo

var hidEnumerate = hid.Enumerate
