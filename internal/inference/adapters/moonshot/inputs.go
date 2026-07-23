package moonshot

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
)

// Kimi limits the complete JSON request body to 100 MB. A 70 MiB raw limit
// leaves room for base64 expansion, prompt text and JSON framing.
const maxReferenceBytes = int64(70 << 20)

type materializedInput struct {
	MediaType string
	Value     string
}

func materializeInputs(ctx context.Context, request inference.Request) ([]materializedInput, error) {
	result := make([]materializedInput, 0, len(request.Inputs))
	var total int64
	for _, input := range request.Inputs {
		if input.MediaType != "image" && input.MediaType != "video" {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider(request), "unsupported Kimi reference type "+input.MediaType, false, nil)
		}
		if value := strings.TrimSpace(input.URL); strings.HasPrefix(value, "data:") || strings.HasPrefix(value, "ms://") {
			result = append(result, materializedInput{MediaType: input.MediaType, Value: value})
			continue
		}
		if input.Content == nil {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider(request), "Kimi references require local content, a data URL or an ms:// file reference", false, nil)
		}
		reader, info, err := input.Content.Open(ctx)
		if err != nil {
			return nil, err
		}
		remaining := maxReferenceBytes - total
		if remaining <= 0 || info.Size > remaining {
			reader.Close()
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider(request), "Kimi references exceed the 70 MB local payload limit", false, nil)
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, remaining+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		total += int64(len(data))
		if total > maxReferenceBytes {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider(request), "Kimi references exceed the 70 MB local payload limit", false, nil)
		}
		mimeType := cleanMIME(input.MIMEType)
		if mimeType == "" {
			mimeType = cleanMIME(info.MIMEType)
		}
		if mimeType == "" {
			mimeType = defaultMIME(input.MediaType)
		}
		result = append(result, materializedInput{
			MediaType: input.MediaType,
			Value:     fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)),
		})
	}
	return result, nil
}

func cleanMIME(value string) string {
	return strings.TrimSpace(strings.Split(value, ";")[0])
}

func defaultMIME(mediaType string) string {
	if mediaType == "video" {
		return "video/mp4"
	}
	return "image/png"
}

func traceReference(value string) string {
	if strings.HasPrefix(value, "data:") {
		return "[LOCAL_DATA]"
	}
	return value
}
