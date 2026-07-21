package adapterutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
)

const (
	defaultJSONLimit     = int64(32 << 20)
	defaultArtifactLimit = int64(512 << 20)
)

type HTTPClient struct {
	client        *http.Client
	jsonLimit     int64
	artifactLimit int64
}

func NewHTTPClient(client *http.Client) *HTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPClient{client: client, jsonLimit: defaultJSONLimit, artifactLimit: defaultArtifactLimit}
}

func (c *HTTPClient) DoJSON(ctx context.Context, provider, method, endpoint, key string, payload, out any) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "encode provider request", false, err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "create provider request", false, err)
	}
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, inference.NewError(inference.ErrorUnavailable, provider, "send provider request", true, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.jsonLimit+1))
	if err != nil {
		return nil, inference.NewError(inference.ErrorProvider, provider, "read provider response", true, err)
	}
	if int64(len(data)) > c.jsonLimit {
		return nil, inference.NewError(inference.ErrorInvalidOutput, provider, "provider response exceeds JSON limit", false, nil)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return data, inference.ResponseError(provider, resp.StatusCode, Truncate(string(data), 600))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return data, inference.NewError(inference.ErrorInvalidOutput, provider, "decode provider response", false, err)
	}
	return data, nil
}

func (c *HTTPClient) URLContent(provider, url string) inference.ContentSource {
	return &urlContent{client: c.client, provider: provider, url: url, limit: c.artifactLimit}
}

type urlContent struct {
	client   *http.Client
	provider string
	url      string
	limit    int64
}

func (s *urlContent) Open(ctx context.Context) (io.ReadCloser, inference.ContentInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, inference.ContentInfo{}, inference.NewError(inference.ErrorInvalidRequest, s.provider, "create artifact request", false, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, inference.ContentInfo{}, inference.NewError(inference.ErrorUnavailable, s.provider, "download provider artifact", true, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 600))
		return nil, inference.ContentInfo{}, inference.ResponseError(s.provider, resp.StatusCode, Truncate(string(body), 600))
	}
	if resp.ContentLength > s.limit {
		resp.Body.Close()
		return nil, inference.ContentInfo{}, inference.NewError(inference.ErrorInvalidOutput, s.provider, "provider artifact exceeds size limit", false, nil)
	}
	mimeType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	return &boundedReadCloser{reader: resp.Body, remaining: s.limit, provider: s.provider}, inference.ContentInfo{MIMEType: mimeType, Size: resp.ContentLength}, nil
}

type boundedReadCloser struct {
	reader    io.ReadCloser
	remaining int64
	provider  string
}

func (r *boundedReadCloser) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, inference.NewError(inference.ErrorInvalidOutput, r.provider, "provider artifact exceeds size limit", false, nil)
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func (r *boundedReadCloser) Close() error { return r.reader.Close() }

func Required(value, field, provider string) error {
	if strings.TrimSpace(value) == "" {
		return inference.NewError(inference.ErrorInvalidRequest, provider, fmt.Sprintf("%s is required", field), false, nil)
	}
	return nil
}
