package processing

import (
	"context"
	"math"

	"github.com/imgproxy/imgproxy/v3/config"
	"github.com/imgproxy/imgproxy/v3/imagedata"
	"github.com/imgproxy/imgproxy/v3/imath"
	"github.com/imgproxy/imgproxy/v3/options"
	"github.com/imgproxy/imgproxy/v3/vips"
)

var watermarkPipeline = pipeline{
	prepare,
	scaleOnLoad,
	importColorProfile,
	scale,
	rotateAndFlip,
	padding,
	stripMetadata,
}

func prepareWatermark(wm *vips.Image, wmData *imagedata.ImageData, opts *options.WatermarkOptions, imgWidth, imgHeight int, offsetScale float64, framesCount int) error {
	if err := wm.Load(wmData, 1, 1.0, 1); err != nil {
		return err
	}

	po := options.NewProcessingOptions()
	po.ResizingType = options.ResizeFit
	po.Dpr = 1
	po.Enlarge = true
	po.Format = wmData.Type
	po.StripMetadata = true
	po.KeepCopyright = false

	if opts.Scale > 0 {
		po.Width = imath.Max(imath.ScaleToEven(imgWidth, opts.Scale), 1)
		po.Height = imath.Max(imath.ScaleToEven(imgHeight, opts.Scale), 1)
	}

	if opts.Replicate {
		offX := int(math.RoundToEven(opts.Gravity.X * offsetScale))
		offY := int(math.RoundToEven(opts.Gravity.Y * offsetScale))

		po.Padding.Enabled = true
		po.Padding.Left = offX / 2
		po.Padding.Right = offX - po.Padding.Left
		po.Padding.Top = offY / 2
		po.Padding.Bottom = offY - po.Padding.Top
	}

	if err := watermarkPipeline.Run(context.Background(), wm, po, wmData); err != nil {
		return err
	}

	if opts.Replicate || framesCount > 1 {
		// We need to copy image if we're going to replicate.
		// Replication requires image to be read several times, and this requires
		// random access to pixels
		if err := wm.CopyMemory(); err != nil {
			return err
		}
	}

	if opts.Replicate {
		if err := wm.Replicate(imgWidth, imgHeight); err != nil {
			return err
		}
	} else {
		left, top := calcPosition(imgWidth, imgHeight, wm.Width(), wm.Height(), &opts.Gravity, offsetScale, true)
		if err := wm.Embed(imgWidth, imgHeight, left, top); err != nil {
			return err
		}
	}

	if framesCount > 1 {
		if err := wm.Replicate(imgWidth, imgWidth*framesCount); err != nil {
			return err
		}
	}

	wm.RemoveHeader("palette-bit-depth")

	return nil
}

func applyWatermark(img *vips.Image, wmData *imagedata.ImageData, opts *options.WatermarkOptions, offsetScale float64, framesCount int) error {
	if err := img.RgbColourspace(); err != nil {
		return err
	}

	wm := new(vips.Image)
	defer wm.Clear()

	width := img.Width()
	height := img.Height()

	if err := prepareWatermark(wm, wmData, opts, width, height/framesCount, offsetScale, framesCount); err != nil {
		return err
	}

	opacity := opts.Opacity * config.WatermarkOpacity

	return img.ApplyWatermark(wm, opacity)
}

func watermark(pctx *pipelineContext, img *vips.Image, po *options.ProcessingOptions, imgdata *imagedata.ImageData) error {
	if !po.Watermark.Enabled || imagedata.Watermark == nil {
		return nil
	}

	return applyWatermark(img, imagedata.Watermark, &po.Watermark, pctx.dprScale, 1)
}
