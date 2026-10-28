# deckmaster

[![Latest Release](https://img.shields.io/github/release/muesli/deckmaster.svg?style=for-the-badge)](https://github.com/muesli/deckmaster/releases)
[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg?style=for-the-badge)](https://pkg.go.dev/github.com/muesli/deckmaster)
[![Software License](https://img.shields.io/badge/license-MIT-blue.svg?style=for-the-badge)](/LICENSE)
[![Build Status](https://img.shields.io/github/actions/workflow/status/muesli/deckmaster/build.yml?branch=master&style=for-the-badge)](https://github.com/muesli/deckmaster/actions)
[![Go ReportCard](https://goreportcard.com/badge/github.com/muesli/deckmaster?style=for-the-badge)](https://goreportcard.com/report/muesli/deckmaster)

An application to control your Elgato Stream Deck on Linux

## Features

- Multiple pages & navigation between decks
- Buttons (icons & text)
- Background images
- Brightness control
- Supports different actions for short & long presses
- Comes with a collection of widgets:
    - Buttons
    - Time (with formatting)
    - CPU/Mem/GPU usage
    - Temperature sensors
    - Weather
    - Command output
    - Recently used windows (X11-only)
    - Several widgets on one key, switched with a tap
- Lets you trigger several actions:
    - Run commands
    - Emulate a key-press
    - Paste to clipboard
    - Trigger a dbus call

## Installation

### Packages

- Arch Linux: [deckmaster](https://aur.archlinux.org/packages/deckmaster/)
- [Packages](https://github.com/muesli/deckmaster/releases) in Alpine, Debian & RPM formats
- [Binaries](https://github.com/muesli/deckmaster/releases) for various architectures

### From source

Make sure you have a working Go environment (Go 1.17 or higher is required).
See the [install instructions](https://golang.org/doc/install.html).

To install deckmaster, simply run:

    git clone https://github.com/muesli/deckmaster.git
    cd deckmaster
    go build

## System Setup

On Linux you need to set up some `udev` rules to be able to access the device as
a regular user. Edit `/etc/udev/rules.d/99-streamdeck.rules` and add these lines:

```
SUBSYSTEM=="usb", ATTRS{idVendor}=="0fd9", ATTRS{idProduct}=="0060", MODE:="666", GROUP="plugdev", SYMLINK+="streamdeck"
SUBSYSTEM=="usb", ATTRS{idVendor}=="0fd9", ATTRS{idProduct}=="006d", MODE:="666", GROUP="plugdev", SYMLINK+="streamdeck"
SUBSYSTEM=="usb", ATTRS{idVendor}=="0fd9", ATTRS{idProduct}=="0080", MODE:="666", GROUP="plugdev", SYMLINK+="streamdeck"
SUBSYSTEM=="usb", ATTRS{idVendor}=="0fd9", ATTRS{idProduct}=="0063", MODE:="666", GROUP="plugdev", SYMLINK+="streamdeck-mini"
SUBSYSTEM=="usb", ATTRS{idVendor}=="0fd9", ATTRS{idProduct}=="006c", MODE:="666", GROUP="plugdev", SYMLINK+="streamdeck-xl"
```

Make sure your user is part of the `plugdev` group and reload the rules with
`sudo udevadm control --reload-rules`. Unplug and re-plug the device, and you
should be good to go.

### Starting deckmaster automatically

If you want deckmaster to be started automatically upon device plugin, you can
use systemd path activation, adding `streamdeck.path` and `streamdeck.service`
files to `$HOME/.config/systemd/user`.

`streamdeck.path` contents:

```ini
[Unit]
Description="Stream Deck Device Path"

[Path]
# the device name will be different if you use streamdeck-mini or streamdeck-xl
PathExists=/dev/streamdeck
Unit=streamdeck.service

[Install]
WantedBy=multi-user.target
```

`streamdeck.service` contents:

```ini
[Unit]
Description=Deckmaster Service

[Service]
# adjust the path to deckmaster and .deck file to suit your needs
ExecStart=/usr/local/bin/deckmaster --deck path-to/some.deck
Restart=on-failure
ExecReload=kill -HUP $MAINPID

[Install]
WantedBy=default.target
```

Then enable and start the `streamdeck.path` unit:

```bash
systemctl --user enable streamdeck.path
systemctl --user start streamdeck.path
```

## Usage

Start `deckmaster` with the initial deck configuration you want to load:

```bash
deckmaster -deck deck/main.deck
```

You can control the brightness, in percent:

```bash
deckmaster -brightness 50
```

Control a specific streamdeck:

```bash
deckmaster -device [serial number]
```

Set a sleep timeout after which the screen gets turned off:

```bash
deckmaster -sleep 10m
```

## Configuration

You can find a few example configurations in the [decks](https://github.com/muesli/deckmaster/tree/master/decks)
directory. Edit them to your needs!

### Widgets

Any widget is build up the following way:

```toml
[[keys]]
  index = 0
```

`index` needs to be present in every widget and describes the position of the
widget on the streamdeck. `index` is 0-indexed and counted from top to bottom
and left to right.

#### Update interval for widgets

Optionally, you can configure an update `interval` for each widget:

```toml
[keys.widget]
  id = "button"
  interval = 500 # optional
```

The attribute `interval` defines the time in `ms` between two consecutive
updates of a widget.

#### Button

A simple button that can display an image and/or a label.

```toml
[keys.widget]
  id = "button"
  [keys.widget.config]
    icon = "/some/image.png" # optional
    iconCommand = "~/bin/player-icon" # optional
    label = "My Button" # optional
    fontsize = 10.0 # optional
    color = "#fefefe" # optional
    flatten = true # optional
```

If `flatten` is `true` all opaque pixels of the icon will have the color `color`.

`iconCommand` is a shell command that deckmaster runs on every update of the
widget, once a second unless the widget sets its own `interval`. The command
prints the path of the image to show, either absolute or relative to the deck
file. `icon` is shown until the command first answers. When the command fails
or prints a path that can't be loaded, the key keeps its current icon.

#### Recent Window (requires X11)

Displays the icon of a recently used window/application. Pressing the button
activates the window.

```toml
[keys.widget]
  id = "recentWindow"
  [keys.widget.config]
    window = 1
    showTitle = true # optional
```

If `showTitle` is `true`, the title of the window will be displayed below the
window icon.

#### Time

A flexible widget that can display the current time or date.

```toml
[keys.widget]
  id = "time"
  [keys.widget.config]
    format = "%H;%i;%s"
    font = "bold;regular;thin" # optional
    color = "#fefefe" # optional
    layout = "0x0+72x24;0x24+72x24;0x48+72x24" # optional
```

With `layout` custom layouts can be definded in the format `[posX]x[posY]+[width]x[height]`.

Values for `format` are:

| %   | gets replaced with                                                 |
| --- | ------------------------------------------------------------------ |
| %Y  | A full numeric representation of a year, 4 digits                  |
| %y  | A two digit representation of a year                               |
| %F  | A full textual representation of a month, such as January or March |
| %M  | A short textual representation of a month, three letters           |
| %m  | Numeric representation of a month, with leading zeros              |
| %l  | A full textual representation of the day of the week               |
| %D  | A textual representation of a day, three letters                   |
| %d  | Day of the month, 2 digits with leading zeros                      |
| %h  | 12-hour format of an hour with leading zeros                       |
| %H  | 24-hour format of an hour with leading zeros                       |
| %i  | Minutes with leading zeros                                         |
| %s  | Seconds with leading zeros                                         |
| %a  | Lowercase Ante meridiem and Post meridiem                          |
| %t  | Timezone abbreviation                                              |

#### Top

This widget shows the current CPU, memory or GPU utilization as a bar graph.

```toml
[keys.widget]
  id = "top"
  [keys.widget.config]
    mode = "cpu"
    color = "#fefefe" # optional
    fillColor = "#d497de" # optional
```

There are four values for `mode`:

| mode     | shows                                  |
| -------- | -------------------------------------- |
| `cpu`    | CPU usage                              |
| `memory` | memory usage                           |
| `gpu`    | how busy the GPU is                    |
| `vram`   | how much of the GPU's memory is in use |

`gpu` and `vram` read the amdgpu card with the most VRAM from
`/sys/class/drm`, so they need Linux and an AMD GPU. When an integrated GPU
sits beside a discrete one, that picks the discrete card.

`mode` can hold several modes split on `;`, which share the key as columns
side by side. `fillColor` splits the same way, one color per column in the same
order, and a column without a color of its own gets the default fill:

```toml
[keys.widget]
  id = "top"
  [keys.widget.config]
    mode = "gpu;vram"
    fillColor = "#d497de;#9bb6de" # optional
```

#### Temp

This widget shows one hwmon temperature sensor in the same bar as `top`. The
bar fills towards the sensor's critical temperature.

```toml
[keys.widget]
  id = "temp"
  [keys.widget.config]
    chip = "k10temp"
    sensor = "Tctl" # optional
    device = "nvme0" # optional
    label = "CPU" # optional
    warn = 80 # optional
    crit = 95 # optional
    color = "#fefefe" # optional
    fillColor = "#d497de" # optional
```

`chip` is the `name` of a chip in `/sys/class/hwmon`, and `sensor` is the
`temp*_label` of a sensor on it. Leave `sensor` out to pick a chip's unlabelled
sensor. When two chips share a name, as two NVMe drives do, `device` picks the
one whose resolved sysfs device path contains it. `label` is drawn under the
bar and defaults to `sensor`.

`warn` and `crit` are in °C. Without them the widget uses the sensor's own
`temp*_max` and `temp*_crit`, and a sensor without a critical temperature fills
the bar at 100 °C. The bar turns orange at the warning temperature and red at
the critical one. A sensor that can't be read shows a dash and is looked up
again on the next update, once a second unless the widget sets its own
`interval`.

#### Command

A widget that displays the output of commands.

```toml
[keys.widget]
  id = "command"
  [keys.widget.config]
    command = "echo 'Files:'; ls -a ~ | wc -l"
    font = "regular;bold" # optional
    color = "#fefefe" # optional
    layout = "0x0+72x20;0x20+72x52" # optional
```

#### Weather

A widget that displays the weather condition and temperature.

```toml
[keys.widget]
  id = "weather"
  [keys.widget.config]
    location = "MyCity" # optional
    unit = "celsius" # optional
    color = "#fefefe" # optional
    flatten = true # optional
    theme = "openmoji" # optional
```

The supported location types can be found [here](http://wttr.in/:help). The unit
has to be either `celsius` or `fahrenheit`. If `flatten` is `true` all opaque
pixels of the condition icon will have the color `color`. In case `theme` is set
corresponding icons with correct names need to be placed in
`~/.local/share/deckmaster/themes/[theme]`. The default icons with their
respective names can be found [here](https://github.com/muesli/deckmaster/tree/master/assets/weather).

#### Toggle

A widget that holds several widgets on one key and shows one at a time. A tap
switches to the next one.

```toml
[keys.widget]
  id = "toggle"
  [[keys.widget.config.widgets]]
    id = "clock"
  [[keys.widget.config.widgets]]
    id = "top"
    interval = 1000 # optional
    [keys.widget.config.widgets.config]
      mode = "cpu;memory"
```

Each entry in `widgets` takes the same `id`, `interval` and `config` as a key's
own `widget` table, and a toggle needs at least two. Every entry is built when
the deck loads, so switching back keeps what a widget has already fetched. A
tap switches only on a key without an `action` of its own, and holding the key
doesn't switch.

### Actions

You can hook up any key with several actions. A regular keypress will trigger
the widget's configured `keys.action`, while holding the key will trigger
`keys.action_hold`.

#### Switch deck

```toml
[keys.action]
  deck = "relative/path/to/another.deck"
```

#### Run a command

```toml
[keys.action]
  exec = "some_command --with-parameters"
```

#### Emulate key-presses

```toml
[keys.action]
  keycode = "Leftctrl-C"
```

Emulate a series of key-presses with delay in between:

```toml
[keys.action]
  keycode = "Leftctrl-X+500 / Leftctrl-V / Num1"
```

A list of available `keycodes` can be found here: [keycodes](https://github.com/muesli/deckmaster/blob/master/keycodes.go)

#### Repeat while held

Any action can repeat while its key is held:

```toml
[keys.action]
  keycode = "Volumeup"
  repeat = 100
```

Once the key is held past a long press (350 ms), the action fires every
`repeat` milliseconds until the key is released. A repeating key never fires
its `keys.action_hold`. A short tap still fires the action once.

#### Paste to clipboard

```toml
[keys.action]
  paste = "some text"
```

#### Trigger a dbus call

```toml
[keys.action]
  [dbus]
    object = "object"
    path = "path"
    method = "method"
    value = "value"
```

#### Device actions

Increase the brightness. If no value is specified, it will be increased by 10%:

```toml
[keys.action]
  device = "brightness+5"
```

Decrease the brightness. If no value is specified, it will be decreased by 10%:

```toml
[keys.action]
  device = "brightness-5"
```

Set the brightness to a specific value between 0 and 100:

```toml
[keys.action]
  device = "brightness=50"
```

Put the device into sleep mode, blanking the screen until the next key gets
pressed:

```toml
[keys.action]
  device = "sleep"
```

### Background Image

You can configure each deck to display an individual wallpaper behind its
widgets:

```toml
background = "/some/image.png"
```

### Re-using another deck's configuration

If you specify a `parent` inside a deck's configuration, it will inherit all
of the parent's settings that are not overwritten by the deck's own settings.
This even works recursively:

```toml
parent = "another.deck"
```

## More Decks!

* [deckmaster-emojis](https://github.com/muesli/deckmaster-emojis), an Emoji keyboard deck
* [deckmaster-helldivers2](https://github.com/boj/deckmaster-helldivers2), a deck for calling down Helldivers 2 game stratagems

Made your own useful decks? Submit a pull request!

## Feedback

Got some feedback or suggestions? Please open an issue or drop me a note!

* [Twitter](https://twitter.com/mueslix)
* [The Fediverse](https://mastodon.social/@fribbledom)
