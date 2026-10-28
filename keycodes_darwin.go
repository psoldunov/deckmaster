//go:build darwin
// +build darwin

package main

// macKeycodes maps Linux evdev key codes to macOS virtual key codes (kVK_* in
// HIToolbox/Events.h). Keys a PC keyboard has and a Mac keyboard lacks map the
// way macOS maps them from a USB keyboard: Insert is Help, and Print Screen,
// Scroll Lock and Pause are F13 to F15.
var macKeycodes = map[int]uint16{
	1:   0x35, // Esc
	2:   0x12, // Num1
	3:   0x13, // Num2
	4:   0x14, // Num3
	5:   0x15, // Num4
	6:   0x17, // Num5
	7:   0x16, // Num6
	8:   0x1A, // Num7
	9:   0x1C, // Num8
	10:  0x19, // Num9
	11:  0x1D, // Num0
	12:  0x1B, // Minus
	13:  0x18, // Equal
	14:  0x33, // Backspace
	15:  0x30, // Tab
	16:  0x0C, // Q
	17:  0x0D, // W
	18:  0x0E, // E
	19:  0x0F, // R
	20:  0x11, // T
	21:  0x10, // Y
	22:  0x20, // U
	23:  0x22, // I
	24:  0x1F, // O
	25:  0x23, // P
	26:  0x21, // Leftbrace
	27:  0x1E, // Rightbrace
	28:  0x24, // Enter
	29:  0x3B, // Leftctrl
	30:  0x00, // A
	31:  0x01, // S
	32:  0x02, // D
	33:  0x03, // F
	34:  0x05, // G
	35:  0x04, // H
	36:  0x26, // J
	37:  0x28, // K
	38:  0x25, // L
	39:  0x29, // Semicolon
	40:  0x27, // Apostrophe
	41:  0x32, // Grave
	42:  0x38, // Leftshift
	43:  0x2A, // Backslash
	44:  0x06, // Z
	45:  0x07, // X
	46:  0x08, // C
	47:  0x09, // V
	48:  0x0B, // B
	49:  0x2D, // N
	50:  0x2E, // M
	51:  0x2B, // Comma
	52:  0x2F, // Dot
	53:  0x2C, // Slash
	54:  0x3C, // Rightshift
	55:  0x43, // Kpasterisk
	56:  0x3A, // Leftalt: Option
	57:  0x31, // Space
	58:  0x39, // Capslock
	59:  0x7A, // F1
	60:  0x78, // F2
	61:  0x63, // F3
	62:  0x76, // F4
	63:  0x60, // F5
	64:  0x61, // F6
	65:  0x62, // F7
	66:  0x64, // F8
	67:  0x65, // F9
	68:  0x6D, // F10
	69:  0x47, // Numlock: Clear
	70:  0x6B, // Scrolllock: F14
	71:  0x59, // Kp7
	72:  0x5B, // Kp8
	73:  0x5C, // Kp9
	74:  0x4E, // Kpminus
	75:  0x56, // Kp4
	76:  0x57, // Kp5
	77:  0x58, // Kp6
	78:  0x45, // Kpplus
	79:  0x53, // Kp1
	80:  0x54, // Kp2
	81:  0x55, // Kp3
	82:  0x52, // Kp0
	83:  0x41, // Kpdot
	86:  0x0A, // 102Nd: the ISO key beside left shift
	87:  0x67, // F11
	88:  0x6F, // F12
	89:  0x5E, // Ro: JIS underscore
	96:  0x4C, // Kpenter
	97:  0x3E, // Rightctrl
	98:  0x4B, // Kpslash
	99:  0x69, // Sysrq: F13
	100: 0x3D, // Rightalt: right Option
	102: 0x73, // Home
	103: 0x7E, // Up
	104: 0x74, // Pageup
	105: 0x7B, // Left
	106: 0x7C, // Right
	107: 0x77, // End
	108: 0x7D, // Down
	109: 0x79, // Pagedown
	110: 0x72, // Insert: Help
	111: 0x75, // Delete: forward delete
	117: 0x51, // Kpequal
	119: 0x71, // Pause: F15
	121: 0x5F, // Kpcomma: JIS keypad comma
	122: 0x68, // Hangeul: JIS Kana
	123: 0x66, // Hanja: JIS Eisu
	124: 0x5D, // Yen
	125: 0x37, // Leftmeta: Command
	126: 0x36, // Rightmeta: right Command
	138: 0x72, // Help
	183: 0x69, // F13
	184: 0x6B, // F14
	185: 0x71, // F15
	186: 0x6A, // F16
	187: 0x40, // F17
	188: 0x4F, // F18
	189: 0x50, // F19
	190: 0x5A, // F20
}

// macSystemKeys maps Linux evdev key codes of media keys to the NX_KEYTYPE_*
// values (IOKit/hidsystem/ev_keymap.h) macOS posts them as.
var macSystemKeys = map[int]int{
	113: 7,  // Mute
	114: 1,  // Volumedown
	115: 0,  // Volumeup
	116: 6,  // Power
	161: 14, // Ejectcd
	163: 17, // Nextsong
	164: 16, // Playpause
	165: 18, // Previoussong
	168: 20, // Rewind
	208: 19, // Fastforward
	224: 3,  // Brightnessdown
	225: 2,  // Brightnessup
	228: 23, // Kbdillumtoggle
	229: 22, // Kbdillumdown
	230: 21, // Kbdillumup
}

// macModifiers maps the virtual key codes of modifier keys to the CGEventFlags
// mask each one sets while held.
var macModifiers = map[uint16]uint64{
	0x38: 0x20000,  // Shift
	0x3C: 0x20000,  // right Shift
	0x3B: 0x40000,  // Control
	0x3E: 0x40000,  // right Control
	0x3A: 0x80000,  // Option
	0x3D: 0x80000,  // right Option
	0x37: 0x100000, // Command
	0x36: 0x100000, // right Command
}
