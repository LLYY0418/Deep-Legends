package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
)

// R146：CommunityDragon 上有些战利品图标（0924 的冠军杯赛挑战券）自带不透明的
// 深青底，其他图标都是透明底，放在卡片里会露出一块方形色块。这里只对「四周是
// 同一种纯色」的不透明 PNG 做处理：从图片边缘向内、沿颜色连续的像素把背景抠成
// 透明，主体（金色票券）与背景颜色差很大，不会被碰到。已经带透明度的图、边缘
// 不是纯色的图、解码失败的数据一律原样返回。
const (
	lootBackgroundBorderShare     = 0.9 // 边缘像素里至少这个比例接近同一颜色才认为是纯色底
	lootBackgroundBorderTolerance = 45.0
	lootBackgroundMaxDistance     = 70.0
	lootBackgroundMaxStep         = 18.0
	lootBackgroundSoftStart       = 35.0
	lootBackgroundMinRemoved      = 0.05 // 抠掉的面积占比低于此值视为没有底，原样返回
	lootBackgroundMaxRemoved      = 0.85 // 高于此值说明把主体也吞了，原样返回
)

func stripSolidPNGBackground(data []byte) []byte {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 8 || height < 8 {
		return data
	}
	pixels := make([][4]float64, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			if a != 0xffff {
				return data // 已有透明度
			}
			pixels[y*width+x] = [4]float64{float64(r >> 8), float64(g >> 8), float64(b >> 8), 255}
		}
	}
	distance := func(a, b [4]float64) float64 {
		return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
	}
	var border []int
	for x := 0; x < width; x++ {
		border = append(border, x, (height-1)*width+x)
	}
	for y := 1; y < height-1; y++ {
		border = append(border, y*width, y*width+width-1)
	}
	channel := func(index int) float64 {
		values := make([]float64, len(border))
		for i, position := range border {
			values[i] = pixels[position][index]
		}
		sort.Float64s(values)
		return values[len(values)/2]
	}
	background := [4]float64{channel(0), channel(1), channel(2), 255}
	near := 0
	for _, position := range border {
		if distance(pixels[position], background) <= lootBackgroundBorderTolerance {
			near++
		}
	}
	if float64(near) < lootBackgroundBorderShare*float64(len(border)) {
		return data
	}
	removed := make([]bool, width*height)
	queue := make([]int, 0, len(border))
	for _, position := range border {
		if distance(pixels[position], background) <= lootBackgroundMaxDistance {
			removed[position] = true
			queue = append(queue, position)
		}
	}
	for head := 0; head < len(queue); head++ {
		position := queue[head]
		x, y := position%width, position/width
		for _, next := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
			if next[0] < 0 || next[1] < 0 || next[0] >= width || next[1] >= height {
				continue
			}
			neighbour := next[1]*width + next[0]
			if removed[neighbour] {
				continue
			}
			if distance(pixels[neighbour], background) <= lootBackgroundMaxDistance && distance(pixels[neighbour], pixels[position]) <= lootBackgroundMaxStep {
				removed[neighbour] = true
				queue = append(queue, neighbour)
			}
		}
	}
	share := float64(len(queue)) / float64(width*height)
	if share < lootBackgroundMinRemoved || share > lootBackgroundMaxRemoved {
		return data
	}
	output := image.NewNRGBA(bounds)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			position := y*width + x
			pixel := pixels[position]
			alpha := 255.0
			if removed[position] {
				d := distance(pixel, background)
				alpha = math.Max(0, math.Min(255, (d-lootBackgroundSoftStart)/(lootBackgroundMaxDistance-lootBackgroundSoftStart)*255))
			}
			output.SetNRGBA(bounds.Min.X+x, bounds.Min.Y+y, color.NRGBA{uint8(pixel[0]), uint8(pixel[1]), uint8(pixel[2]), uint8(alpha)})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, output); err != nil {
		return data
	}
	return encoded.Bytes()
}
