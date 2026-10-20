// Command icon builds the small original geometric PR Nanny icon using only Go's standard library.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	for _, cfg := range []struct {
		path     string
		size     int
		template bool
	}{{"assets/icon.png", 512, false}, {"assets/tray.png", 44, true}} {
		img := image.NewNRGBA(image.Rect(0, 0, cfg.size, cfg.size))
		scale := float64(cfg.size) / 24
		for y := 0; y < cfg.size; y++ {
			for x := 0; x < cfg.size; x++ {
				px, py := (float64(x)+.5)/scale, (float64(y)+.5)/scale
				inside := inPolygon(px, py, [][2]float64{{12, 2}, {20, 5}, {20, 11}, {18.5, 16}, {15.5, 20}, {12, 22}, {8.5, 20}, {5.5, 16}, {4, 11}, {4, 5}})
				if inside {
					if cfg.template {
						img.SetNRGBA(x, y, color.NRGBA{A: 255})
					} else {
						img.SetNRGBA(x, y, color.NRGBA{R: 22, G: 139, B: 112, A: 255})
					}
				}
				if inside && (line(px, py, 8, 12, 11, 15) < 1 || line(px, py, 11, 15, 17, 8) < 1) {
					if cfg.template {
						img.SetNRGBA(x, y, color.NRGBA{})
					} else {
						img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
					}
				}
			}
		}
		f, e := os.Create(cfg.path)
		if e != nil {
			panic(e)
		}
		if e = png.Encode(f, img); e != nil {
			panic(e)
		}
		if e = f.Close(); e != nil {
			panic(e)
		}
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
