package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type generationReferenceURLRequest struct {
	URL string `json:"url"`
}

type downloadedGenerationReference struct {
	File      *os.File
	Name      string
	MIMEType  string
	Size      int64
	SourceURL string
}

type referenceIPLookup func(context.Context, string, string) ([]net.IP, error)

func (s *Server) importGenerationReferenceURL(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(c.Context(), projectID); err != nil {
		return err
	}
	var req generationReferenceURLRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	downloaded, err := downloadGenerationReferenceURL(c.Context(), req.URL, s.cfg.TempDir(), net.DefaultResolver.LookupIP)
	if err != nil {
		return fmt.Errorf("%w: %v", service.ErrInvalidInput, err)
	}
	defer func() {
		_ = downloaded.File.Close()
		_ = os.Remove(downloaded.File.Name())
	}()
	uploadID := uuid.New()
	managed, err := s.storage.UploadManaged(c.Context(), storage.ManagedUploadInput{
		ProjectID: &projectID, OwnerID: &uploadID, Purpose: "references", OriginalName: downloaded.Name,
		UploadInput: storage.UploadInput{Reader: downloaded.File, Size: downloaded.Size, ContentType: downloaded.MIMEType},
	})
	if err != nil {
		return err
	}
	item, err := s.store.CreateGenerationReferenceUpload(c.Context(), db.CreateGenerationReferenceUploadInput{
		ID: uploadID, ProjectID: projectID, ObjectID: managed.Record.ID, Name: downloaded.Name,
		MediaType: mediaTypeFromMIME(downloaded.MIMEType), MimeType: downloaded.MIMEType, FileSizeBytes: downloaded.Size,
		Metadata: db.JSON(map[string]any{"source_kind": "url", "source_url": downloaded.SourceURL}),
	})
	if err != nil {
		_ = s.storage.DeleteManaged(c.Context(), managed.Record.ID)
		return err
	}
	return writeCreated(c, item)
}

func downloadGenerationReferenceURL(ctx context.Context, rawURL, tempDir string, lookup referenceIPLookup) (downloadedGenerationReference, error) {
	parsed, err := validateGenerationReferenceURL(ctx, rawURL, lookup)
	if err != nil {
		return downloadedGenerationReference{}, err
	}
	client := generationReferenceHTTPClient(lookup)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return downloadedGenerationReference{}, errors.New("cannot prepare online reference request")
	}
	req.Header.Set("User-Agent", "SagaFlow/0.1 online-reference")
	response, err := client.Do(req)
	if err != nil {
		return downloadedGenerationReference{}, errors.New("cannot download the online reference")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return downloadedGenerationReference{}, fmt.Errorf("online reference returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxUploadSize {
		return downloadedGenerationReference{}, errors.New("online reference is too large")
	}
	file, err := os.CreateTemp(tempDir, "online-reference-*")
	if err != nil {
		return downloadedGenerationReference{}, err
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}
	written, err := io.Copy(file, io.LimitReader(response.Body, maxUploadSize+1))
	if err != nil {
		cleanup()
		return downloadedGenerationReference{}, errors.New("online reference download was interrupted")
	}
	if written == 0 || written > maxUploadSize {
		cleanup()
		return downloadedGenerationReference{}, errors.New("online reference is empty or too large")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return downloadedGenerationReference{}, err
	}
	reader := bufio.NewReaderSize(file, 512)
	sample, peekErr := reader.Peek(512)
	if peekErr != nil && !errors.Is(peekErr, io.EOF) && !errors.Is(peekErr, bufio.ErrBufferFull) {
		cleanup()
		return downloadedGenerationReference{}, peekErr
	}
	detected := http.DetectContentType(sample)
	declared, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if detected == "application/octet-stream" && declared != "" {
		detected = declared
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return downloadedGenerationReference{}, err
	}
	name := path.Base(response.Request.URL.Path)
	if name == "" || name == "." || name == "/" {
		name = "online-reference"
	}
	return downloadedGenerationReference{File: file, Name: name, MIMEType: detected, Size: written, SourceURL: response.Request.URL.String()}, nil
}

func generationReferenceHTTPClient(lookup referenceIPLookup) *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil, ForceAttemptHTTP2: true, ResponseHeaderTimeout: 12 * time.Second,
		TLSHandshakeTimeout: 8 * time.Second, IdleConnTimeout: 30 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid online reference address")
		}
		ips, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, errors.New("cannot resolve online reference host")
		}
		for _, ip := range ips {
			if !blockedReferenceIP(ip) {
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}
		}
		return nil, errors.New("online reference host resolves to a private or local address")
	}
	return &http.Client{
		Transport: transport, Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("online reference redirected too many times")
			}
			_, err := validateGenerationReferenceURL(req.Context(), req.URL.String(), lookup)
			return err
		},
	}
}

func validateGenerationReferenceURL(ctx context.Context, rawURL string, lookup referenceIPLookup) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("online reference must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return nil, errors.New("online reference URL cannot contain a username or password")
	}
	ips, err := lookup(ctx, "ip", parsed.Hostname())
	if err != nil || len(ips) == 0 {
		return nil, errors.New("cannot resolve online reference host")
	}
	for _, ip := range ips {
		if blockedReferenceIP(ip) {
			return nil, errors.New("online reference cannot use a private or local address")
		}
	}
	return parsed, nil
}

func blockedReferenceIP(ip net.IP) bool {
	return ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

func generationReferenceSourceURL(raw []byte) string {
	var metadata struct {
		SourceKind string `json:"source_kind"`
		SourceURL  string `json:"source_url"`
	}
	if json.Unmarshal(raw, &metadata) != nil || metadata.SourceKind != "url" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(metadata.SourceURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.String()
}
