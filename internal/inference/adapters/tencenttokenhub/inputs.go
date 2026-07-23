package tencenttokenhub

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
)

const maxReferenceSize = int64(10 << 20)

type materializedInput struct {
	MediaType string
	MIMEType  string
	URL       string
	Base64    string
}

func materializeInputs(ctx context.Context, request inference.Request, allowed ...string) ([]materializedInput, error) {
	accepted := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		accepted[value] = struct{}{}
	}
	result := make([]materializedInput, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		if _, ok := accepted[input.MediaType]; !ok {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider(request), "unsupported TokenHub reference type "+input.MediaType, false, nil)
		}
		materialized := materializedInput{
			MediaType: input.MediaType,
			MIMEType:  cleanMIME(input.MIMEType),
			URL:       strings.TrimSpace(input.URL),
		}
		if materialized.URL == "" {
			if input.Content == nil {
				return nil, inference.NewError(inference.ErrorInvalidRequest, provider(request), "reference content is unavailable", false, nil)
			}
			reader, info, err := input.Content.Open(ctx)
			if err != nil {
				return nil, err
			}
			data, readErr := io.ReadAll(io.LimitReader(reader, maxReferenceSize+1))
			closeErr := reader.Close()
			if readErr != nil {
				return nil, readErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			if int64(len(data)) > maxReferenceSize || info.Size > maxReferenceSize {
				return nil, inference.NewError(inference.ErrorInvalidRequest, provider(request), "TokenHub reference exceeds 10 MB", false, nil)
			}
			if materialized.MIMEType == "" {
				materialized.MIMEType = cleanMIME(info.MIMEType)
			}
			if materialized.MIMEType == "" {
				materialized.MIMEType = "application/octet-stream"
			}
			materialized.Base64 = base64.StdEncoding.EncodeToString(data)
		}
		result = append(result, materialized)
	}
	return result, nil
}

func (input materializedInput) dataURL() string {
	if input.URL != "" {
		return input.URL
	}
	return fmt.Sprintf("data:%s;base64,%s", input.MIMEType, input.Base64)
}

func (input materializedInput) traceValue() string {
	if input.URL != "" {
		return input.URL
	}
	return "[LOCAL_DATA]"
}

func cleanMIME(value string) string {
	return strings.TrimSpace(strings.Split(value, ";")[0])
}
