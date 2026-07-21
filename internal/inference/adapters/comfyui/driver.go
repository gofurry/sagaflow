package comfyui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
	"github.com/google/uuid"
)

const (
	maxInputs       = 16
	maxArtifactSize = int64(512 << 20)
)

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := providerName(request.Runtime)
	if request.Target.Kind != inference.TargetWorkflow {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "ComfyUI adapter requires a workflow target", false, nil)
	}
	spec, err := DecodeWorkflowSpec(request.Target.Spec)
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, err.Error(), false, err)
	}
	if len(request.Inputs) > maxInputs {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, fmt.Sprintf("ComfyUI accepts at most %d workflow inputs", maxInputs), false, nil)
	}
	release, err := d.acquire(ctx, request.Runtime)
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorUnavailable, provider, "wait for ComfyUI connection capacity", true, err)
	}
	defer release()

	runID := strings.TrimSpace(request.ProviderRunID)
	if runID == "" {
		runID = deterministicPromptID(request.ID)
		state, history, stateErr := d.lookup(ctx, request.Runtime, runID)
		if stateErr != nil {
			return inference.Result{}, stateErr
		}
		if state == "completed" {
			return d.resultFromHistory(request.Runtime, spec, history)
		}
		if state != "queued" && state != "running" {
			uploads := make([]uploadResponse, 0, len(request.Inputs))
			for index, input := range request.Inputs {
				upload, uploadErr := d.uploadInput(ctx, request.Runtime, request.ID, index, input)
				if uploadErr != nil {
					return inference.Result{}, uploadErr
				}
				uploads = append(uploads, upload)
				if err := inference.Emit(ctx, events, inference.Event{Stage: "uploading", Progress: .1 + .1*float64(index+1)/float64(max(1, len(request.Inputs))), Message: fmt.Sprintf("uploaded workflow input %d/%d", index+1, len(request.Inputs))}); err != nil {
					return inference.Result{}, err
				}
			}
			workflow, bindErr := bindWorkflow(spec, request, uploads)
			if bindErr != nil {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, bindErr.Error(), false, bindErr)
			}
			if err := d.submit(ctx, request, runID, workflow, events); err != nil {
				return inference.Result{}, err
			}
		}
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", Progress: .25, Message: "ComfyUI workflow is queued", ProviderRunID: runID}); err != nil {
		return inference.Result{}, err
	}
	history, err := d.wait(ctx, request.Runtime, runID, events)
	if err != nil {
		return inference.Result{}, err
	}
	return d.resultFromHistory(request.Runtime, spec, history)
}

func (d *Driver) submit(ctx context.Context, request inference.Request, runID string, workflow map[string]WorkflowNode, events inference.EventSink) error {
	nodeIDs := make([]string, 0, len(workflow))
	for id := range workflow {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	payload := map[string]any{
		"prompt": workflow, "prompt_id": runID, "client_id": request.ID,
		"extra_data": map[string]any{"sagaflow_job_id": request.ID, "sagaflow_workflow": request.Target.ID},
	}
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "ComfyUI workflow request prepared", ProviderRunID: runID,
		Details: map[string]any{"prompt_id": runID, "workflow": request.Target.ID, "node_ids": nodeIDs, "node_count": len(nodeIDs), "parameters": request.Parameters},
	}); err != nil {
		return err
	}
	var response promptResponse
	if err := d.doJSON(ctx, request.Runtime, http.MethodPost, "/prompt", payload, &response); err != nil {
		state, _, lookupErr := d.lookup(ctx, request.Runtime, runID)
		if lookupErr == nil && state != "missing" {
			return inference.Emit(ctx, events, inference.Event{Stage: state, Progress: .25, Message: "ComfyUI accepted the workflow before the connection was interrupted", ProviderRunID: runID})
		}
		return err
	}
	if response.PromptID == "" {
		return inference.NewError(inference.ErrorInvalidOutput, providerName(request.Runtime), "ComfyUI returned no prompt_id", false, nil)
	}
	if response.PromptID != runID {
		return inference.NewError(inference.ErrorInvalidOutput, providerName(request.Runtime), "ComfyUI returned an unexpected prompt_id", false, nil)
	}
	return inference.Emit(ctx, events, inference.Event{Stage: "queued", Progress: .25, Message: "ComfyUI accepted the workflow", ProviderRunID: runID, Details: map[string]any{"queue_number": response.Number, "node_errors": response.NodeErrors}})
}

func (d *Driver) wait(ctx context.Context, runtime inference.Runtime, runID string, events inference.EventSink) (historyEntry, error) {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	lastState := ""
	for {
		state, history, err := d.lookup(ctx, runtime, runID)
		if err != nil {
			return historyEntry{}, err
		}
		switch state {
		case "completed":
			if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", Progress: .75, Message: "ComfyUI workflow outputs are ready", ProviderRunID: runID}); err != nil {
				return historyEntry{}, err
			}
			return history, nil
		case "failed":
			message := historyFailure(history)
			return historyEntry{}, inference.NewError(inference.ErrorProvider, providerName(runtime), message, false, nil)
		case "running", "queued":
			if state != lastState {
				progress := .35
				if state == "running" {
					progress = .5
				}
				if err := inference.Emit(ctx, events, inference.Event{Stage: state, Progress: progress, Message: "ComfyUI workflow is " + state, ProviderRunID: runID}); err != nil {
					return historyEntry{}, err
				}
				lastState = state
			}
		}
		select {
		case <-ctx.Done():
			cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_ = d.cancel(cancelCtx, runtime, runID)
			return historyEntry{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

type historyEntry struct {
	Outputs map[string]map[string]json.RawMessage `json:"outputs"`
	Status  struct {
		StatusStr string `json:"status_str"`
		Completed bool   `json:"completed"`
		Messages  []any  `json:"messages"`
	} `json:"status"`
}

func (d *Driver) lookup(ctx context.Context, runtime inference.Runtime, runID string) (string, historyEntry, error) {
	historyRoot := map[string]historyEntry{}
	if err := d.getJSON(ctx, runtime, "/history/"+url.PathEscape(runID), &historyRoot); err != nil {
		return "", historyEntry{}, err
	}
	if history, ok := historyRoot[runID]; ok {
		status := strings.ToLower(history.Status.StatusStr)
		if history.Status.Completed || len(history.Outputs) > 0 {
			if status == "error" || status == "failed" || status == "cancelled" || status == "canceled" {
				return "failed", history, nil
			}
			return "completed", history, nil
		}
		return "running", history, nil
	}
	var queue struct {
		Running []json.RawMessage `json:"queue_running"`
		Pending []json.RawMessage `json:"queue_pending"`
	}
	if err := d.getJSON(ctx, runtime, "/queue", &queue); err != nil {
		return "", historyEntry{}, err
	}
	if queueContains(queue.Running, runID) {
		return "running", historyEntry{}, nil
	}
	if queueContains(queue.Pending, runID) {
		return "queued", historyEntry{}, nil
	}
	return "missing", historyEntry{}, nil
}

func queueContains(items []json.RawMessage, runID string) bool {
	for _, raw := range items {
		var values []any
		if json.Unmarshal(raw, &values) == nil && len(values) > 1 && fmt.Sprint(values[1]) == runID {
			return true
		}
	}
	return false
}

func (d *Driver) cancel(ctx context.Context, runtime inference.Runtime, runID string) error {
	if err := d.doJSON(ctx, runtime, http.MethodPost, "/queue", map[string]any{"delete": []string{runID}}, nil); err != nil {
		return err
	}
	return d.doJSON(ctx, runtime, http.MethodPost, "/interrupt", map[string]any{"prompt_id": runID}, nil)
}

func deterministicPromptID(requestID string) string {
	if parsed, err := uuid.Parse(strings.TrimSpace(requestID)); err == nil {
		return parsed.String()
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("sagaflow:"+requestID)).String()
}

func historyFailure(history historyEntry) string {
	data, _ := json.Marshal(history.Status.Messages)
	message := adapterutil.Truncate(string(data), 1200)
	if message == "" || message == "null" || message == "[]" {
		message = "ComfyUI workflow execution failed"
	}
	return message
}

func (d *Driver) uploadInput(ctx context.Context, runtime inference.Runtime, jobID string, index int, input inference.Input) (uploadResponse, error) {
	provider := providerName(runtime)
	if input.MediaType != "image" || input.Content == nil {
		return uploadResponse{}, inference.NewError(inference.ErrorInvalidRequest, provider, "ComfyUI workflow inputs currently require image content", false, nil)
	}
	reader, info, err := input.Content.Open(ctx)
	if err != nil {
		return uploadResponse{}, inference.NewError(inference.ErrorInvalidRequest, provider, "open ComfyUI workflow input", false, err)
	}
	filename := safeFilename(input.Name, input.ID, info.MIMEType)
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	go func() {
		defer reader.Close()
		part, createErr := writer.CreateFormFile("image", fmt.Sprintf("%02d-%s", index+1, filename))
		if createErr == nil {
			_, createErr = io.Copy(part, reader)
		}
		if createErr == nil {
			createErr = writer.WriteField("type", "input")
		}
		if createErr == nil {
			createErr = writer.WriteField("subfolder", "sagaflow/"+deterministicPromptID(jobID))
		}
		if createErr == nil {
			createErr = writer.WriteField("overwrite", "true")
		}
		if closeErr := writer.Close(); createErr == nil {
			createErr = closeErr
		}
		_ = pipeWriter.CloseWithError(createErr)
	}()
	endpoint, err := apiEndpoint(runtime.Endpoint, "/upload/image")
	if err != nil {
		pipeReader.Close()
		return uploadResponse{}, inference.NewError(inference.ErrorInvalidRequest, provider, "invalid ComfyUI endpoint", false, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pipeReader)
	if err != nil {
		pipeReader.Close()
		return uploadResponse{}, inference.NewError(inference.ErrorInvalidRequest, provider, "create ComfyUI upload request", false, err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	setHeaders(request, runtime.APIKey)
	response, err := d.http.Do(request)
	if err != nil {
		pipeReader.Close()
		return uploadResponse{}, inference.NewError(inference.ErrorUnavailable, provider, "upload ComfyUI workflow input", true, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return uploadResponse{}, inference.NewError(inference.ErrorProvider, provider, "read ComfyUI upload response", true, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return uploadResponse{}, inference.ResponseError(provider, response.StatusCode, adapterutil.Truncate(string(data), 600))
	}
	var uploaded uploadResponse
	if err := json.Unmarshal(data, &uploaded); err != nil || uploaded.Name == "" {
		return uploadResponse{}, inference.NewError(inference.ErrorInvalidOutput, provider, "decode ComfyUI upload response", false, err)
	}
	return uploaded, nil
}

func safeFilename(name, id, mimeType string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "." || name == "" {
		name = strings.TrimSpace(id)
	}
	if filepath.Ext(name) == "" {
		if extensions, _ := mime.ExtensionsByType(strings.Split(mimeType, ";")[0]); len(extensions) > 0 {
			name += extensions[0]
		} else {
			name += ".png"
		}
	}
	return name
}
