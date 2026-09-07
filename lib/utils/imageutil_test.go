package utils

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/headlesslab/wand/lib/proto"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestSplicePngVertical(t *testing.T) {
	g := setup(t)
	a := image.NewRGBA(image.Rect(0, 0, 1000, 200))
	b := image.NewRGBA(image.Rect(0, 0, 1000, 300))

	g.Run("jpeg", func(g got.G) {
		format := proto.PageCaptureScreenshotFormatJpeg
		processor, err := NewImgProcessor(format)
		if err != nil {
			g.Err(err)
		}
		aBs, _ := processor.Encode(a, nil)
		bBs, _ := processor.Encode(b, nil)

		bs, err := SplicePngVertical([]ImgWithBox{
			{Img: aBs},
			{Img: bBs},
		}, format, nil)
		g.E(err)

		img, err := processor.Decode(bytes.NewBuffer(bs))
		g.E(err)

		g.Eq(img.Bounds().Dy(), 500)
		g.Eq(img.Bounds().Dx(), 1000)
	})
	g.Run("jpegWithOptions", func(g got.G) {
		format := proto.PageCaptureScreenshotFormatJpeg
		processor, err := NewImgProcessor(format)
		g.E(err)

		aBs, _ := processor.Encode(a, nil)
		bBs, _ := processor.Encode(b, nil)

		bs, err := SplicePngVertical([]ImgWithBox{
			{Img: aBs},
			{Img: bBs},
		}, format, &ImgOption{
			Quality: 10,
		})
		g.E(err)

		img, err := processor.Decode(bytes.NewBuffer(bs))
		g.E(err)

		g.Eq(img.Bounds().Dy(), 500)
		g.Eq(img.Bounds().Dx(), 1000)
	})
	g.Run("jpegWithBox", func(g got.G) {
		format := proto.PageCaptureScreenshotFormatJpeg
		processor, err := NewImgProcessor(format)
		g.E(err)

		aBs, _ := processor.Encode(a, nil)
		bBs, _ := processor.Encode(b, nil)

		bs, err := SplicePngVertical([]ImgWithBox{
			{
				Img: aBs,
				Box: &image.Rectangle{
					Max: image.Point{
						X: a.Bounds().Dx(),
						Y: 100,
					},
				},
			},
			{Img: bBs},
		}, format, nil)
		g.E(err)

		img, err := processor.Decode(bytes.NewBuffer(bs))
		g.E(err)

		g.Eq(img.Bounds().Dy(), 400)
		g.Eq(img.Bounds().Dx(), 1000)
	})
	g.Run("errorEncode", func(g got.G) {
		format := proto.PageCaptureScreenshotFormatPng
		processor, err := NewImgProcessor(format)
		g.E(err)

		aBs, _ := processor.Encode(a, nil)
		bBs, _ := processor.Encode(b, nil)

		_, err = SplicePngVertical([]ImgWithBox{
			{
				Img: aBs,
				Box: &image.Rectangle{},
			},
			{
				Img: bBs,
				Box: &image.Rectangle{},
			},
		}, format, nil)
		// invalid image size: 0x0
		g.Err(err)
	})
	g.Run("noFile", func(g got.G) {
		_, err := SplicePngVertical(nil, "", nil)
		g.E(err)
	})
	g.Run("oneFile", func(g got.G) {
		bs, err := SplicePngVertical([]ImgWithBox{
			{Img: []byte{1}},
		}, "", nil)
		g.E(err)
		g.Eq(1, len(bs))
	})
	g.Run("unsupportedFormat", func(g got.G) {
		_, err := SplicePngVertical([]ImgWithBox{
			{Img: []byte{1}},
			{Img: []byte{1}},
		}, "gif", nil)
		g.Err(err)
	})
	g.Run("errorFile", func(g got.G) {
		_, err := SplicePngVertical([]ImgWithBox{
			{Img: []byte{1}},
			{Img: []byte{1}},
		}, "", nil)
		g.Err(err)
	})
}

func TestNewImgProcessor(t *testing.T) {
	g := setup(t)
	type args struct {
		format proto.PageCaptureScreenshotFormat
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "jpeg",
			args: args{
				format: proto.PageCaptureScreenshotFormatJpeg,
			},
			wantErr: false,
		},
		{
			name: "default",
			args: args{
				format: "",
			},
			wantErr: false,
		},
		{
			name: "png",
			args: args{
				format: proto.PageCaptureScreenshotFormatPng,
			},
			wantErr: false,
		},
		{
			name: "webP",
			args: args{
				/* cspell: disable-next-line */
				format: proto.PageCaptureScreenshotFormatWebp,
			},
			wantErr: true,
		},
	}

	a := image.NewRGBA(image.Rect(0, 0, 1000, 200))
	// errImg := image.NewRGBA(image.Rect(0, 0, 0, 0))

	for _, tt := range tests {
		g.Run(tt.name, func(g got.G) {
			processor, err := NewImgProcessor(tt.args.format)
			if tt.wantErr {
				g.Eq(err != nil, tt.wantErr)
			}
			if err != nil {
				return
			}
			buf, err := processor.Encode(a, nil)
			if err != nil {
				g.Err(err)
			}
			img, err := processor.Decode(bytes.NewBuffer(buf))
			if err != nil {
				g.Err(err)
			}

			g.Eq(1000, img.Bounds().Dx())
			g.Eq(200, img.Bounds().Dy())

			_, err = processor.Decode(bytes.NewBuffer(nil))
			g.Err(err)
		})
	}
}

// spliceReference is the pixel-by-pixel splice SplicePngVertical used to do,
// kept as what its one-pass draw is held to: every pixel of the box, or of
// the whole image, goes under the images before it through color.Color, the
// column in place, so a box that starts right of the edge leaves the columns
// before it black and loses the ones past the width. The sizing is the
// splice's own, repeated, so this characterizes the whole of it: a change to
// the sizing has to be made twice.
func spliceReference(g got.G, files []ImgWithBox, format proto.PageCaptureScreenshotFormat) *image.RGBA {
	processor, err := NewImgProcessor(format)
	g.E(err)

	var images []image.Image
	var width, height int
	for _, file := range files {
		img, err := processor.Decode(bytes.NewReader(file.Img))
		g.E(err)
		images = append(images, img)
		if file.Box != nil {
			width = file.Box.Dx()
			height += file.Box.Dy()
		} else {
			width = img.Bounds().Dx()
			height += img.Bounds().Dy()
		}
	}

	spliceImg := image.NewRGBA(image.Rect(0, 0, width, height))
	var destY int
	for i, file := range files {
		bounds := images[i].Bounds()
		if file.Box != nil {
			bounds = *file.Box
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				spliceImg.Set(x, y-bounds.Min.Y+destY, images[i].At(x, y))
			}
		}
		destY += bounds.Dy()
	}
	return spliceImg
}

// The splice's output is the bytes the reference gives, for the pixel
// formats the two decoders produce (non-premultiplied RGBA from png, YCbCr
// from jpeg), with opaque, translucent and transparent pixels, whole images
// and boxes, one of them starting right of the left edge.
func TestSplicePngVerticalMatchesReference(t *testing.T) {
	g := setup(t)

	shot := func(w, h int, seed int) *image.NRGBA {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetNRGBA(x, y, color.NRGBA{
					R: uint8(x*7 + seed),
					G: uint8(y*3 + seed),
					B: uint8(x ^ y),
					A: uint8((x*y + seed) * 5),
				})
			}
		}
		return img
	}

	for _, format := range []proto.PageCaptureScreenshotFormat{
		proto.PageCaptureScreenshotFormatPng,
		proto.PageCaptureScreenshotFormatJpeg,
	} {
		g.Run(string(format), func(g got.G) {
			processor, err := NewImgProcessor(format)
			g.E(err)
			a, err := processor.Encode(shot(40, 30, 1), nil)
			g.E(err)
			b, err := processor.Encode(shot(40, 20, 2), nil)
			g.E(err)
			c, err := processor.Encode(shot(40, 25, 3), nil)
			g.E(err)

			files := []ImgWithBox{
				{Img: a},
				{Img: b, Box: &image.Rectangle{Min: image.Pt(0, 5), Max: image.Pt(40, 15)}},
				{Img: c, Box: &image.Rectangle{Min: image.Pt(3, 2), Max: image.Pt(40, 20)}},
			}

			bs, err := SplicePngVertical(files, format, nil)
			g.E(err)
			want, err := processor.Encode(spliceReference(g, files, format), nil)
			g.E(err)
			g.Eq(bs, want)
		})
	}
}
