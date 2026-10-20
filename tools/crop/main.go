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
	x, _ := strconv.Atoi(os.Args[3])
	y, _ := strconv.Atoi(os.Args[4])
	w, _ := strconv.Atoi(os.Args[5])
	h, _ := strconv.Atoi(os.Args[6])
	n, _ := strconv.Atoi(os.Args[7])
	f, _ := os.Open(in)
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	dst := image.NewRGBA(image.Rect(0, 0, w*n, h*n))
	for j := 0; j < h*n; j++ {
		for i := 0; i < w*n; i++ {
			dst.Set(i, j, src.At(x+i/n, y+j/n))
		}
	}
	o, _ := os.Create(out)
	png.Encode(o, dst)
	o.Close()
	fmt.Println("ok")
}
