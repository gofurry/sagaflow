// Package inference defines SagaFlow's provider-neutral model execution contract.
//
// Gateway and its drivers are safe for concurrent use after construction. Request,
// Result, and Event values are request-scoped and must not be mutated concurrently.
package inference

import (
	"context"
	"encoding/json"
	"io"
)

type Capability string

const (
	CapabilityText  Capability = "text"
	CapabilityImage Capability = "image"
	CapabilityAudio Capability = "audio"
	CapabilityVideo Capability = "video"
)

type Runtime struct {
	ProviderCode  string
	AdapterCode   string
	Endpoint      string
	APIKey        string
	Configuration json.RawMessage
}

type TargetKind string

const (
	TargetModel    TargetKind = "model"
	TargetWorkflow TargetKind = "workflow"
)

type Target struct {
	Kind       TargetKind
	ID         string
	Capability Capability
	Spec       json.RawMessage
}

type Request struct {
	ID            string
	Runtime       Runtime
	Target        Target
	Operation     string
	Control       json.RawMessage
	Prompt        string
	Parameters    map[string]any
	Inputs        []Input
	ProviderRunID string
}

type Input struct {
	ID        string
	Name      string
	MediaType string
	MIMEType  string
	URL       string
	Content   ContentSource
}

type ContentInfo struct {
	MIMEType string
	Size     int64
}

// ContentSource opens a fresh stream for every call. The caller owns and must
// close the returned reader. A size below zero means that the size is unknown.
type ContentSource interface {
	Open(ctx context.Context) (io.ReadCloser, ContentInfo, error)
}

type OpenContentFunc func(context.Context) (io.ReadCloser, ContentInfo, error)

func (f OpenContentFunc) Open(ctx context.Context) (io.ReadCloser, ContentInfo, error) {
	return f(ctx)
}

type Artifact struct {
	MediaType string
	MIMEType  string
	SourceURL string
	Metadata  json.RawMessage
	Content   ContentSource
}

type Result struct {
	Artifacts []Artifact
	Usage     map[string]any
}

type Event struct {
	Stage         string
	Progress      float64
	Message       string
	ProviderRunID string
	Usage         map[string]any
	Details       map[string]any
}

type EventSink func(context.Context, Event) error

func Emit(ctx context.Context, sink EventSink, event Event) error {
	if sink == nil {
		return nil
	}
	return sink(ctx, event)
}

type Executor interface {
	Execute(ctx context.Context, request Request, events EventSink) (Result, error)
}

type Driver interface {
	Execute(ctx context.Context, request Request, events EventSink) (Result, error)
}
