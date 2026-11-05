// Command icon builds the small original geometric PR Nanny icon using only Go's standard library.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// The tray glyph is a template image: black shield, transparent check mark,
// 18x18 px and 36x36 px for MenubarKit's TrayIconLoader. It is supersampled so
// edges are anti-aliased at menu-bar size.
func main() {
	write("assets/icon.png", render(512, false, 1))
	write("macos/Resources/PRNannyTrayIcon.png", render(18, true, 8))
	write("macos/Resources/PRNannyTrayIcon@2x.png", render(36, true, 8))
}

var shield = [][2]float64{{12, 2}, {20, 5}, {20, 11}, {18.5, 16}, {15.5, 20}, {12, 22}, {8.5, 20}, {5.5, 16}, {4, 11}, {4, 5}}

func render(size int, template bool, samples int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / 24
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			body, check := 0, 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := (float64(x) + (float64(sx)+.5)/float64(samples)) / scale
					py := (float64(y) + (float64(sy)+.5)/float64(samples)) / scale
					if !inPolygon(px, py, shield) {
						continue
					}
					if line(px, py, 8, 12, 11, 15) < 1 || line(px, py, 11, 15, 17, 8) < 1 {
						check++
					} else {
						body++
					}
				}
			}
			total := samples * samples
			switch {
			case template:
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(255 * body / total)})
			case check*2 > total:
				img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
			case body*2 > total:
				img.SetNRGBA(x, y, color.NRGBA{R: 22, G: 139, B: 112, A: 255})
			}
		}
	}
	return img
}

func write(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	if err = png.Encode(f, img); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
}

func inPolygon(x, y float64, p [][2]float64) bool {
	in := false
	j := len(p) - 1
	for i := range p {
		if (p[i][1] > y) != (p[j][1] > y) && x < (p[j][0]-p[i][0])*(y-p[i][1])/(p[j][1]-p[i][1])+p[i][0] {
			in = !in
		}
		j = i
	}
	return in
}
func line(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-(ax+t*dx), y-(ay+t*dy))
}
