package zhipu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const maxAudioResponseSize = int64(256 << 20)

func (d *Driver) generateSpeech(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	if len(request.Inputs) != 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Zhipu speech generation uses a saved voice ID and does not accept reference assets", false, nil)
	}
	format := adapterutil.StringParam(request.Parameters, "response_format", "wav")
	payload := map[string]any{"model": request.Target.ID, "input": request.Prompt, "voice": adapterutil.StringParam(request.Parameters, "voice", "tongtong"), "response_format": format}
	for _, key := range []string{"speed", "volume", "watermark_enabled"} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	url := endpoint(request.Runtime.Endpoint, "/audio/speech")
	if err := emitRequest(ctx, events, url, payload); err != nil {
		return inference.Result{}, err
	}
	data, mimeType, err := d.doBinaryJSON(ctx, providerName, url, request.Runtime.APIKey, payload)
	if err != nil {
		return inference.Result{}, err
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = audioMIME(format)
	}
	return inference.Result{Artifacts: []inference.Artifact{{
		MediaType: "audio", MIMEType: mimeType, Content: inference.BytesContent(data, mimeType),
	}}}, nil
}

func (d *Driver) transcribe(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	if len(request.Inputs) != 1 || request.Inputs[0].MediaType != "audio" || request.Inputs[0].Content == nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Zhipu transcription requires one audio reference", false, nil)
	}
	reader, info, err := request.Inputs[0].Content.Open(ctx)
	if err != nil {
		return inference.Result{}, err
	}
	defer reader.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", request.Inputs[0].Name)
	if err != nil {
		return inference.Result{}, err
	}
	if _, err := io.Copy(part, io.LimitReader(reader, maxReferenceSize+1)); err != nil {
		return inference.Result{}, err
	}
	if int64(body.Len()) > maxReferenceSize+(1<<20) {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "transcription input exceeds 25 MB", false, nil)
	}
	if err := writer.WriteField("model", request.Target.ID); err != nil {
		return inference.Result{}, err
	}
	if err := writer.WriteField("stream", "false"); err != nil {
		return inference.Result{}, err
	}
	for _, key := range []string{"prompt", "hotwords"} {
		if value := adapterutil.StringParam(request.Parameters, key, ""); value != "" {
			if err := writer.WriteField(key, value); err != nil {
				return inference.Result{}, err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return inference.Result{}, err
	}
	url := endpoint(request.Runtime.Endpoint, "/audio/transcriptions")
	if err := emitRequest(ctx, events, url, map[string]any{"model": request.Target.ID, "file": map[string]any{"name": request.Inputs[0].Name, "size": info.Size}}); err != nil {
		return inference.Result{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return inference.Result{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+request.Runtime.APIKey)
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := d.client.Do(httpRequest)
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorUnavailable, providerName, "send transcription request", true, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return inference.Result{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return inference.Result{}, inference.ResponseError(providerName, response.StatusCode, adapterutil.Truncate(string(data), 600))
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &result); err != nil || strings.TrimSpace(result.Text) == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "Zhipu transcription returned no text", false, err)
	}
	return inference.Result{Artifacts: []inference.Artifact{{
		MediaType: "text", MIMEType: "text/markdown; charset=utf-8",
		Content: inference.BytesContent([]byte(result.Text), "text/markdown; charset=utf-8"),
	}}}, nil
}

func (d *Driver) doBinaryJSON(ctx context.Context, providerName, url, key string, payload any) ([]byte, string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "audio/*, application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(request)
	if err != nil {
		return nil, "", inference.NewError(inference.ErrorUnavailable, providerName, "send speech request", true, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAudioResponseSize+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxAudioResponseSize {
		return nil, "", inference.NewError(inference.ErrorInvalidOutput, providerName, "speech response exceeds size limit", false, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", inference.ResponseError(providerName, response.StatusCode, adapterutil.Truncate(string(data), 600))
	}
	mimeType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if strings.Contains(mimeType, "json") {
		return nil, "", inference.NewError(inference.ErrorInvalidOutput, providerName, fmt.Sprintf("speech response is JSON: %s", adapterutil.Truncate(string(data), 600)), false, nil)
	}
	return data, mimeType, nil
}

func audioMIME(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	default:
		return "application/octet-stream"
	}
}
