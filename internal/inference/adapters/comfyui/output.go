package comfyui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) resultFromHistory(runtime inference.Runtime, spec WorkflowSpec, history historyEntry) (inference.Result, error) {
	provider := providerName(runtime)
	artifacts := make([]inference.Artifact, 0)
	for _, selector := range spec.Outputs {
		node := history.Outputs[selector.NodeID]
		raw, ok := node[selector.Key]
		if !ok {
			continue
		}
		values := []json.RawMessage{raw}
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			values = list
		}
		for _, value := range values {
			artifact, ok, err := d.outputArtifact(runtime, selector, value)
			if err != nil {
				return inference.Result{}, err
			}
			if ok {
				artifacts = append(artifacts, artifact)
			}
		}
	}
	if len(artifacts) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "configured ComfyUI outputs were not present in history", false, nil)
	}
	return inference.Result{Artifacts: artifacts, Usage: map[string]any{"workflow_version": spec.Version, "workflow_checksum": spec.Checksum}}, nil
}

func (d *Driver) outputArtifact(runtime inference.Runtime, selector OutputSelector, raw json.RawMessage) (inference.Artifact, bool, error) {
	provider := providerName(runtime)
	if selector.MediaType == "text" {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			return inference.Artifact{
				MediaType: "text", MIMEType: defaultMIME(selector),
				Metadata: adapterutil.JSON(map[string]any{"node_id": selector.NodeID, "output_key": selector.Key}),
				Content:  inference.BytesContent([]byte(text), defaultMIME(selector)),
			}, true, nil
		}
		var object map[string]any
		if json.Unmarshal(raw, &object) == nil {
			if value, ok := object["text"].(string); ok {
				return inference.Artifact{
					MediaType: "text", MIMEType: defaultMIME(selector),
					Metadata: adapterutil.JSON(map[string]any{"node_id": selector.NodeID, "output_key": selector.Key}),
					Content:  inference.BytesContent([]byte(value), defaultMIME(selector)),
				}, true, nil
			}
		}
	}
	var file struct {
		Filename  string `json:"filename"`
		Subfolder string `json:"subfolder"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || strings.TrimSpace(file.Filename) == "" {
		return inference.Artifact{}, false, nil
	}
	if file.Type == "" {
		file.Type = "output"
	}
	endpoint, err := apiEndpoint(runtime.Endpoint, "/view")
	if err != nil {
		return inference.Artifact{}, false, inference.NewError(inference.ErrorInvalidRequest, provider, "build ComfyUI output URL", false, err)
	}
	parsed, _ := url.Parse(endpoint)
	query := parsed.Query()
	query.Set("filename", file.Filename)
	query.Set("subfolder", file.Subfolder)
	query.Set("type", file.Type)
	parsed.RawQuery = query.Encode()
	mimeType := strings.TrimSpace(selector.MIMEType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(file.Filename)))
	}
	if mimeType == "" {
		mimeType = defaultMIME(selector)
	}
	metadata := adapterutil.JSON(map[string]any{
		"node_id": selector.NodeID, "output_key": selector.Key, "filename": file.Filename,
		"subfolder": file.Subfolder, "folder_type": file.Type,
	})
	return inference.Artifact{
		MediaType: selector.MediaType, MIMEType: mimeType, SourceURL: parsed.String(), Metadata: metadata,
		Content: &remoteContent{client: d.http, runtime: runtime, url: parsed.String(), mimeType: mimeType},
	}, true, nil
}

func defaultMIME(selector OutputSelector) string {
	if selector.MIMEType != "" {
		return selector.MIMEType
	}
	switch selector.MediaType {
	case "text":
		return "text/plain; charset=utf-8"
	case "image":
		return "image/png"
	case "audio":
		return "audio/mpeg"
	case "video":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

type remoteContent struct {
	client   HTTPDoer
	runtime  inference.Runtime
	url      string
	mimeType string
}

func (s *remoteContent) Open(ctx context.Context) (io.ReadCloser, inference.ContentInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, inference.ContentInfo{}, inference.NewError(inference.ErrorInvalidRequest, providerName(s.runtime), "create ComfyUI output request", false, err)
	}
	setHeaders(request, s.runtime.APIKey)
	response, err := s.client.Do(request)
	if err != nil {
		return nil, inference.ContentInfo{}, inference.NewError(inference.ErrorUnavailable, providerName(s.runtime), "download ComfyUI output", true, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 600))
		return nil, inference.ContentInfo{}, inference.ResponseError(providerName(s.runtime), response.StatusCode, adapterutil.Truncate(string(body), 600))
	}
	if response.ContentLength > maxArtifactSize {
		response.Body.Close()
		return nil, inference.ContentInfo{}, inference.NewError(inference.ErrorInvalidOutput, providerName(s.runtime), "ComfyUI output exceeds artifact limit", false, nil)
	}
	mimeType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = s.mimeType
	}
	return &limitedContent{ReadCloser: response.Body, remaining: maxArtifactSize, provider: providerName(s.runtime)}, inference.ContentInfo{MIMEType: mimeType, Size: response.ContentLength}, nil
}

type limitedContent struct {
	io.ReadCloser
	remaining int64
	provider  string
}

func (r *limitedContent) Read(buffer []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		if count, err := r.ReadCloser.Read(probe[:]); count > 0 {
			return 0, inference.NewError(inference.ErrorInvalidOutput, r.provider, "ComfyUI output exceeds artifact limit", false, nil)
		} else {
			return 0, err
		}
	}
	if int64(len(buffer)) > r.remaining {
		buffer = buffer[:r.remaining]
	}
	count, err := r.ReadCloser.Read(buffer)
	r.remaining -= int64(count)
	return count, err
}

var _ inference.ContentSource = (*remoteContent)(nil)

func outputKey(nodeID, key string) string { return fmt.Sprintf("%s.%s", nodeID, key) }
