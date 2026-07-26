package bailian

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const maxImageEditInputSize = int64(10 << 20)

type imageEditControl struct {
	Type         string  `json:"type"`
	SourceWidth  int     `json:"source_width"`
	SourceHeight int     `json:"source_height"`
	TopScale     float64 `json:"top_scale"`
	BottomScale  float64 `json:"bottom_scale"`
	LeftScale    float64 `json:"left_scale"`
	RightScale   float64 `json:"right_scale"`
}

func (d *Driver) editImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := request.Runtime.ProviderCode
	var control imageEditControl
	if err := json.Unmarshal(request.Control, &control); err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "decode image edit control", false, err)
	}
	control.Type = strings.ToLower(strings.TrimSpace(control.Type))
	if control.Type != request.Operation {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "image edit operation does not match its control payload", false, nil)
	}
	wantInputs := 1
	if control.Type == "inpaint" {
		wantInputs = 2
	}
	if len(request.Inputs) != wantInputs {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, fmt.Sprintf("%s requires %d image input(s)", control.Type, wantInputs), false, nil)
	}
	baseURL, baseData, err := imageEditInput(ctx, request.Inputs[0], provider)
	if err != nil {
		return inference.Result{}, err
	}
	if err := validateImageDimensions(baseData, control.SourceWidth, control.SourceHeight, false); err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "source image dimensions do not match the editor snapshot", false, err)
	}

	input := map[string]any{
		"prompt":         request.Prompt,
		"base_image_url": baseURL,
	}
	parameters := map[string]any{
		"n":         int(adapterutil.NumberParam(request.Parameters, "n", 1)),
		"watermark": adapterutil.BoolParam(request.Parameters, "watermark", false),
	}
	adapterutil.CopyParam(parameters, request.Parameters, "seed")
	switch control.Type {
	case "outpaint":
		for _, scale := range []float64{control.TopScale, control.BottomScale, control.LeftScale, control.RightScale} {
			if scale < 1 || scale > 2 {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "outpainting scales must be between 1 and 2", false, nil)
			}
		}
		input["function"] = "expand"
		parameters["top_scale"] = control.TopScale
		parameters["bottom_scale"] = control.BottomScale
		parameters["left_scale"] = control.LeftScale
		parameters["right_scale"] = control.RightScale
	case "inpaint":
		maskURL, maskData, maskErr := imageEditInput(ctx, request.Inputs[1], provider)
		if maskErr != nil {
			return inference.Result{}, maskErr
		}
		if err := validateImageDimensions(maskData, control.SourceWidth, control.SourceHeight, true); err != nil {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "mask must be a non-empty pure black-and-white image matching the source", false, err)
		}
		input["function"] = "description_edit_with_mask"
		input["mask_image_url"] = maskURL
	default:
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "Wan image edit operation must be outpaint or inpaint", false, nil)
	}

	payload := map[string]any{"model": request.Target.ID, "input": input, "parameters": parameters}
	taskID, err := d.submitTask(ctx, request, events, "/api/v1/services/aigc/image2image/image-synthesis", payload)
	if err != nil {
		return inference.Result{}, err
	}
	response, raw, err := d.waitTask(ctx, request, events, taskID)
	if err != nil {
		return inference.Result{}, err
	}
	urls := imageURLs(response)
	if len(urls) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "completed Bailian image edit task has no image URL", false, nil)
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", ProviderRunID: taskID}); err != nil {
		return inference.Result{}, err
	}
	artifacts := make([]inference.Artifact, 0, len(urls))
	for _, url := range urls {
		artifacts = append(artifacts, inference.Artifact{
			MediaType: "image", MIMEType: "image/png", SourceURL: url,
			Metadata: adapterutil.JSONSummary(raw, "request_id", "usage"), Content: d.http.URLContent(provider, url),
		})
	}
	return inference.Result{Artifacts: artifacts, Usage: usage(response)}, nil
}

func imageEditInput(ctx context.Context, input inference.Input, provider string) (string, []byte, error) {
	if input.MediaType != "image" || input.Content == nil {
		return "", nil, inference.NewError(inference.ErrorInvalidRequest, provider, "image edit inputs must contain local image content", false, nil)
	}
	reader, info, err := input.Content.Open(ctx)
	if err != nil {
		return "", nil, err
	}
	defer reader.Close()
	if info.Size > maxImageEditInputSize {
		return "", nil, inference.NewError(inference.ErrorInvalidRequest, provider, "image edit input exceeds 10 MB", false, nil)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxImageEditInputSize+1))
	if err != nil {
		return "", nil, err
	}
	if int64(len(data)) > maxImageEditInputSize {
		return "", nil, inference.NewError(inference.ErrorInvalidRequest, provider, "image edit input exceeds 10 MB", false, nil)
	}
	mimeType := strings.TrimSpace(strings.Split(input.MIMEType, ";")[0])
	if mimeType == "" {
		mimeType = strings.TrimSpace(strings.Split(info.MIMEType, ";")[0])
	}
	switch mimeType {
	case "image/png", "image/jpeg":
	default:
		return "", nil, inference.NewError(inference.ErrorInvalidRequest, provider, "unsupported image edit input format "+mimeType, false, nil)
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), data, nil
}

func validateImageDimensions(data []byte, width, height int, requireMask bool) error {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		return fmt.Errorf("got %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
	if !requireMask {
		return nil
	}
	hasBlack, hasWhite := false, false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := decoded.At(x, y).RGBA()
			switch {
			case r == 0 && g == 0 && b == 0:
				hasBlack = true
			case r == 0xffff && g == 0xffff && b == 0xffff:
				hasWhite = true
			default:
				return fmt.Errorf("mask contains a non-binary pixel at %d,%d", x, y)
			}
		}
	}
	if !hasBlack || !hasWhite {
		return fmt.Errorf("mask must contain both preserved and editable regions")
	}
	return nil
}
