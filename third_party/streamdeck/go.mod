// muesli/streamdeck v0.4.0, carried here so macOS can drive the deck through
// bearsh/hid while every other platform keeps karalabe/hid (see hid_darwin.go).
module github.com/muesli/streamdeck

go 1.14

require (
	github.com/bearsh/hid v1.4.1
	github.com/karalabe/hid v1.0.1-0.20190806082151-9c14560f9ee8
	golang.org/x/image v0.7.0
)
