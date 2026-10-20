package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
)

func main() {
	in, out := os.Args[1], os.Args[2]
	n, _ := strconv.Atoi(os.Args[3])
	f, err := os.Open(in)
	if err != nil {
		fmt.Println(err)
		return
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	b := src.Bounds()
	w, h := b.Dx()*n, b.Dy()*n
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(b.Min.X+x/n, b.Min.Y+y/n))
		}
	}
	o, _ := os.Create(out)
	png.Encode(o, dst)
	o.Close()
	fmt.Println("zoom ->", out)
}
