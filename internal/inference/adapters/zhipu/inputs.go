package zhipu

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
)

const maxReferenceSize = int64(25 << 20)

func (d *Driver) materializeReferences(ctx context.Context, request inference.Request, allowed ...string) (inference.Request, error) {
	accepted := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		accepted[value] = struct{}{}
	}
	updated := request
	updated.Inputs = append([]inference.Input(nil), request.Inputs...)
	for index := range updated.Inputs {
		input := &updated.Inputs[index]
		if _, ok := accepted[input.MediaType]; !ok {
			return inference.Request{}, inference.NewError(inference.ErrorInvalidRequest, provider(request), "unsupported Zhipu reference type "+input.MediaType, false, nil)
		}
		if strings.TrimSpace(input.URL) != "" {
			continue
		}
		value, err := inputDataURL(ctx, *input)
		if err != nil {
			return inference.Request{}, err
		}
		input.URL = value
	}
	return updated, nil
}

func inputDataURL(ctx context.Context, input inference.Input) (string, error) {
	if input.Content == nil {
		return "", inference.NewError(inference.ErrorInvalidRequest, providerCode, "reference content is unavailable", false, nil)
	}
	reader, info, err := input.Content.Open(ctx)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	if info.Size > maxReferenceSize {
		return "", inference.NewError(inference.ErrorInvalidRequest, providerCode, "reference exceeds 25 MB", false, nil)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxReferenceSize+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxReferenceSize {
		return "", inference.NewError(inference.ErrorInvalidRequest, providerCode, "reference exceeds 25 MB", false, nil)
	}
	mimeType := strings.TrimSpace(strings.Split(input.MIMEType, ";")[0])
	if mimeType == "" {
		mimeType = strings.TrimSpace(strings.Split(info.MIMEType, ";")[0])
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}

func traceReference(value string) string {
	if strings.HasPrefix(value, "data:") {
		return "[LOCAL_DATA]"
	}
	return value
}
