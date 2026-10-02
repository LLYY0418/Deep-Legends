package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func lootBackgroundFixture(t *testing.T, alpha uint8, noisyBorder bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.NRGBA{10, 35, 45, alpha}
			// 底色带轻微渐变，模拟真实素材的暗角。
			c.G += uint8((x + y) / 16)
			if noisyBorder && (x == 0 || y == 0 || x == 63 || y == 63) {
				c = color.NRGBA{uint8(x * 4), uint8(y * 4), 200, alpha}
			}
			if x >= 20 && x < 44 && y >= 12 && y < 52 {
				c = color.NRGBA{190, 150, 60, alpha}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestStripSolidPNGBackgroundRemovesOnlyTheBackground(t *testing.T) {
	result := stripSolidPNGBackground(lootBackgroundFixture(t, 255, false))
	img, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range [][2]int{{0, 0}, {63, 63}, {5, 30}, {60, 10}} {
		if _, _, _, a := img.At(point[0], point[1]).RGBA(); a != 0 {
			t.Fatalf("background %v alpha=%d", point, a>>8)
		}
	}
	for _, point := range [][2]int{{32, 32}, {20, 12}, {43, 51}} {
		if _, _, _, a := img.At(point[0], point[1]).RGBA(); a>>8 != 255 {
			t.Fatalf("subject %v alpha=%d", point, a>>8)
		}
	}
}

func TestStripSolidPNGBackgroundLeavesOtherImagesAlone(t *testing.T) {
	transparent := lootBackgroundFixture(t, 0, false)
	if !bytes.Equal(stripSolidPNGBackground(transparent), transparent) {
		t.Fatal("image that already has transparency must be returned unchanged")
	}
	noisy := lootBackgroundFixture(t, 255, true)
	if !bytes.Equal(stripSolidPNGBackground(noisy), noisy) {
		t.Fatal("image without a solid border must be returned unchanged")
	}
	if !bytes.Equal(stripSolidPNGBackground([]byte("not a png")), []byte("not a png")) {
		t.Fatal("undecodable data must be returned unchanged")
	}
}
