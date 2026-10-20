// Regenerate the Windows icon resources from icon.svg with go generate .
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/draw"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	svg, err := oksvg.ReadIcon("icon.svg")
	if err != nil {
		return err
	}
	const resolution = 1024
	rendered := image.NewRGBA(image.Rect(0, 0, resolution, resolution))
	svg.SetTarget(0, 0, resolution, resolution)
	svg.Draw(rasterx.NewDasher(resolution, resolution, rasterx.NewScannerGV(resolution, resolution, rendered, rendered.Bounds())), 1)
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var header, payload bytes.Buffer
	for _, value := range []uint16{0, 1, uint16(len(sizes))} {
		binary.Write(&header, binary.LittleEndian, value)
	}
	for _, size := range sizes {
		bitmap := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(bitmap, bitmap.Bounds(), rendered, rendered.Bounds(), draw.Src, nil)
		var data bytes.Buffer
		if err := png.Encode(&data, bitmap); err != nil {
			return err
		}
		header.Write([]byte{byte(size % 256), byte(size % 256), 0, 0})
		for _, value := range []uint16{1, 32} {
			binary.Write(&header, binary.LittleEndian, value)
		}
		for _, value := range []uint32{uint32(data.Len()), uint32(6 + 16*len(sizes) + payload.Len())} {
			binary.Write(&header, binary.LittleEndian, value)
		}
		payload.Write(data.Bytes())
	}
	if err := os.WriteFile("icon.ico", append(header.Bytes(), payload.Bytes()...), 0644); err != nil {
		return err
	}
	command := exec.Command("windres", "-F", "pe-x86-64", "-i", "icon.rc", "-o", "icon_windows_amd64.syso", "-O", "coff")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("compile Windows icon (requires MinGW windres): %w", err)
	}
	return nil
}
