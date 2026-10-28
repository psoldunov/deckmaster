//go:build darwin
// +build darwin

package streamdeck

// karalabe/hid bundles a 2019 hidapi that names macOS devices by their
// IOService path. On Apple Silicon those paths run up against the 512-byte
// io_string_t, IORegistryEntryGetPath fails, and every Stream Deck enumerates
// with an empty path and is skipped. bearsh/hid keeps karalabe/hid's API on
// hidapi 0.11, which names devices by registry entry ID instead.
import "github.com/bearsh/hid"

type (
	hidDevice     = hid.Device
	hidDeviceInfo = hid.DeviceInfo
)

var hidEnumerate = hid.Enumerate
