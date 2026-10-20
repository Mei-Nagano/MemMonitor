package main

// 开发用：把卡片合成结果导出成 PNG（分别贴到白底和黑底上），
// 用于核对文字抗锯齿、圆角、透明度的真实像素值。

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strconv"
	"time"
)

func dumpCard(prefix string) {
	initDPI()
	cfg = loadConfig()
	createFonts()
	initGdiplus()
	initImageList()

	// 导出用满不透明度：预览图不会把桌面内容透出来
	cfg.Alpha = 255

	state.TargetName = cfg.TargetName
	state.TargetPid = cfg.TargetPid
	state.W = cfg.Width
	// MEMMON_WIDTH=240 可指定导出宽度，避免受当前配置影响
	if v := os.Getenv("MEMMON_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			state.W = int32(n)
		}
	}
	if t := os.Getenv("MEMMON_DEMO"); t != "" {
		state.Toast = t
		state.ToastUntil = time.Now().UnixMilli() + 3600000
	}
	sample()
	// MEMMON_SPARK=1 时灌一段平缓的历史，让预览图里的趋势线有内容
	if os.Getenv("MEMMON_SPARK") != "" {
		base := float64(state.MemBytes)
		for i := 0; i < 60; i++ {
			f := 0.62 + 0.38*math.Sin(float64(i)*0.22)
			state.push(uint64(base * f))
		}
	}
	renderCard()

	L := layoutCard()
	w, h := int(L.W), int(L.H)
	px := cardSurf.pixels()

	writeOver := func(path string, br, bg, bb uint8) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := (y*w + x) * 4
				a := float64(px[i+3]) / 255
				// 预乘 alpha 叠到不透明底色上
				r := float64(px[i+2]) + float64(br)*(1-a)
				g := float64(px[i+1]) + float64(bg)*(1-a)
				b := float64(px[i]) + float64(bb)*(1-a)
				img.Set(x, y, color.RGBA{clamp8(r), clamp8(g), clamp8(b), 255})
			}
		}
		f, err := os.Create(path)
		if err != nil {
			fmt.Println(err)
			return
		}
		png.Encode(f, img)
		f.Close()
	}

	writeOver(prefix+"_white.png", 255, 255, 255)
	writeOver(prefix+"_black.png", 0, 0, 0)
	writePreview(prefix+".png", 22)
	fmt.Printf("dump %dx%d alpha=%d -> %s_white.png / %s_black.png / %s.png\n", w, h, cfg.Alpha, prefix, prefix, prefix)
	fmt.Printf("icon rect: %d,%d - %d,%d\n", L.Icon.Left, L.Icon.Top, L.Icon.Right, L.Icon.Bottom)
	show := func(label string, x, y int32) {
		if x < 0 || y < 0 || int(x) >= w || int(y) >= h {
			return
		}
		i := (int(y)*w + int(x)) * 4
		fmt.Printf("  %-10s (%3d,%3d) BGRA=%d,%d,%d,%d\n", label, x, y,
			px[i], px[i+1], px[i+2], px[i+3])
	}
	show("corner", 0, 0)
	show("bg", 5, 5)
	show("icon-tl", L.Icon.Left, L.Icon.Top)
	show("icon-c", (L.Icon.Left+L.Icon.Right)/2, (L.Icon.Top+L.Icon.Bottom)/2)
	show("icon-br", L.Icon.Right-1, L.Icon.Bottom-1)
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// writePreview 把卡片叠到一张中性渐变背景上，四周留白，用作仓库预览图。
// 背景是程序生成的，不包含任何桌面内容。
func writePreview(path string, pad int) {
	L := layoutCard()
	w, h := int(L.W), int(L.H)
	px := cardSurf.pixels()
	W, H := w+pad*2, h+pad*2

	img := image.NewRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			// 左上偏亮的深蓝灰 → 右下更深，斜向渐变
			t := (float64(x)/float64(W) + float64(y)/float64(H)) / 2
			br := 44.0 - 26*t
			bg := 52.0 - 30*t
			bb := 66.0 - 38*t

			i := 0
			cx, cy := x-pad, y-pad
			if cx < 0 || cy < 0 || cx >= w || cy >= h {
				img.Set(x, y, color.RGBA{clamp8(br), clamp8(bg), clamp8(bb), 255})
				continue
			}
			i = (cy*w + cx) * 4
			a := float64(px[i+3]) / 255
			r := float64(px[i+2]) + br*(1-a)
			g := float64(px[i+1]) + bg*(1-a)
			b := float64(px[i]) + bb*(1-a)
			img.Set(x, y, color.RGBA{clamp8(r), clamp8(g), clamp8(b), 255})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	png.Encode(f, img)
	f.Close()
}
