package db

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID          uuid.UUID `db:"id" json:"id"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type Account struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	Singleton    int32      `db:"singleton" json:"-"`
	Username     string     `db:"username" json:"username"`
	DisplayName  string     `db:"display_name" json:"display_name"`
	PasswordHash string     `db:"password_hash" json:"-"`
	LastLoginAt  *time.Time `db:"last_login_at" json:"last_login_at"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
}

type Session struct {
	ID         uuid.UUID  `db:"id" json:"id"`
	TokenHash  string     `db:"token_hash" json:"-"`
	ExpiresAt  time.Time  `db:"expires_at" json:"expires_at"`
	RevokedAt  *time.Time `db:"revoked_at" json:"revoked_at"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	LastSeenAt time.Time  `db:"last_seen_at" json:"last_seen_at"`
}

type Episode struct {
	ID                    uuid.UUID `db:"id" json:"id"`
	ProjectID             uuid.UUID `db:"project_id" json:"project_id"`
	EpisodeNumber         int32     `db:"episode_number" json:"episode_number"`
	Title                 string    `db:"title" json:"title"`
	Notes                 string    `db:"notes" json:"notes"`
	TargetDurationSeconds *int32    `db:"target_duration_seconds" json:"target_duration_seconds"`
	TargetShotCount       *int32    `db:"target_shot_count" json:"target_shot_count"`
	CreatedAt             time.Time `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time `db:"updated_at" json:"updated_at"`
}

type EpisodeScript struct {
	ID        uuid.UUID `db:"id" json:"id"`
	EpisodeID uuid.UUID `db:"episode_id" json:"episode_id"`
	Version   int32     `db:"version" json:"version"`
	Body      string    `db:"body" json:"body"`
	Note      string    `db:"note" json:"note"`
	Status    string    `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type AssetGroup struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	ProjectID   uuid.UUID  `db:"project_id" json:"project_id"`
	ParentID    *uuid.UUID `db:"parent_id" json:"parent_id"`
	Kind        string     `db:"kind" json:"kind"`
	Name        string     `db:"name" json:"name"`
	Description string     `db:"description" json:"description"`
	SortOrder   int32      `db:"sort_order" json:"sort_order"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}

type Asset struct {
	ID                 uuid.UUID       `db:"id" json:"id"`
	ProjectID          uuid.UUID       `db:"project_id" json:"project_id"`
	GroupID            *uuid.UUID      `db:"group_id" json:"group_id"`
	StagedAssetID      *uuid.UUID      `db:"staged_asset_id" json:"staged_asset_id"`
	EpisodeID          *uuid.UUID      `db:"episode_id" json:"episode_id"`
	CanvasNodeID       *uuid.UUID      `db:"canvas_node_id" json:"canvas_node_id"`
	ObjectID           uuid.UUID       `db:"object_id" json:"object_id"`
	Name               string          `db:"name" json:"name"`
	MediaType          string          `db:"media_type" json:"media_type"`
	Source             string          `db:"source" json:"source"`
	Status             string          `db:"status" json:"status"`
	MimeType           string          `db:"mime_type" json:"mime_type"`
	FileSizeBytes      int64           `db:"file_size_bytes" json:"file_size_bytes"`
	OriginalURL        string          `db:"original_url" json:"original_url"`
	ProviderCode       string          `db:"provider_code" json:"provider_code"`
	ModelIdentifier    string          `db:"model_identifier" json:"model_identifier"`
	ParametersSnapshot json.RawMessage `db:"parameters_snapshot" json:"parameters_snapshot"`
	InputSnapshot      json.RawMessage `db:"input_snapshot" json:"input_snapshot"`
	Metadata           json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt          time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at" json:"updated_at"`
}

type CanvasNode struct {
	ID                    uuid.UUID       `db:"id" json:"id"`
	EpisodeID             uuid.UUID       `db:"episode_id" json:"episode_id"`
	NodeType              string          `db:"node_type" json:"node_type"`
	PositionX             float64         `db:"position_x" json:"position_x"`
	PositionY             float64         `db:"position_y" json:"position_y"`
	Width                 *float64        `db:"width" json:"width"`
	Height                *float64        `db:"height" json:"height"`
	ZIndex                int32           `db:"z_index" json:"z_index"`
	Data                  json.RawMessage `db:"data" json:"data"`
	AssetID               *uuid.UUID      `db:"asset_id" json:"asset_id"`
	Title                 string          `db:"title" json:"title"`
	Body                  string          `db:"body" json:"body"`
	ShotNumber            *int32          `db:"shot_number" json:"shot_number"`
	TargetDurationSeconds *int32          `db:"target_duration_seconds" json:"target_duration_seconds"`
	SelectedVideoAssetID  *uuid.UUID      `db:"selected_video_asset_id" json:"selected_video_asset_id"`
	Color                 string          `db:"color" json:"color"`
	CreatedAt             time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time       `db:"updated_at" json:"updated_at"`
}

type CanvasEdge struct {
	ID           uuid.UUID       `db:"id" json:"id"`
	EpisodeID    uuid.UUID       `db:"episode_id" json:"episode_id"`
	SourceNodeID uuid.UUID       `db:"source_node_id" json:"source_node_id"`
	TargetNodeID uuid.UUID       `db:"target_node_id" json:"target_node_id"`
	SourceHandle *string         `db:"source_handle" json:"source_handle"`
	TargetHandle *string         `db:"target_handle" json:"target_handle"`
	EdgeType     string          `db:"edge_type" json:"edge_type"`
	Data         json.RawMessage `db:"data" json:"data"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
}

type CanvasAnnotation struct {
	ID             uuid.UUID `db:"id" json:"id"`
	EpisodeID      uuid.UUID `db:"episode_id" json:"episode_id"`
	AnnotationType string    `db:"annotation_type" json:"annotation_type"`
	PositionX      float64   `db:"position_x" json:"position_x"`
	PositionY      float64   `db:"position_y" json:"position_y"`
	Width          float64   `db:"width" json:"width"`
	Height         float64   `db:"height" json:"height"`
	StrokeColor    string    `db:"stroke_color" json:"stroke_color"`
	StrokeWidth    float64   `db:"stroke_width" json:"stroke_width"`
	LineStyle      string    `db:"line_style" json:"line_style"`
	Opacity        float64   `db:"opacity" json:"opacity"`
	Label          string    `db:"label" json:"label"`
	LabelPosition  string    `db:"label_position" json:"label_position"`
	ZIndex         int32     `db:"z_index" json:"z_index"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

type Canvas struct {
	Nodes       []CanvasNode       `json:"nodes"`
	Edges       []CanvasEdge       `json:"edges"`
	Annotations []CanvasAnnotation `json:"annotations"`
}

type ModelProvider struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	Code           string          `db:"code" json:"code"`
	AdapterCode    string          `db:"adapter_code" json:"adapter_code"`
	DisplayName    string          `db:"display_name" json:"display_name"`
	BaseURL        string          `db:"base_url" json:"base_url"`
	AuthType       string          `db:"auth_type" json:"auth_type"`
	Capabilities   []string        `db:"capabilities" json:"capabilities"`
	Enabled        bool            `db:"enabled" json:"enabled"`
	MaxConcurrency int32           `db:"max_concurrency" json:"max_concurrency"`
	Metadata       json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
}

type ProviderCredential struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	ProviderID      uuid.UUID  `db:"provider_id" json:"provider_id"`
	ProviderCode    string     `db:"provider_code" json:"provider_code"`
	Name            string     `db:"name" json:"name"`
	EncryptedAPIKey string     `db:"encrypted_api_key" json:"-"`
	KeyHint         string     `db:"key_hint" json:"key_hint"`
	IsActive        bool       `db:"is_active" json:"is_active"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
	LastUsedAt      *time.Time `db:"last_used_at" json:"last_used_at"`
}

type Model struct {
	ID                uuid.UUID       `db:"id" json:"id"`
	ProviderID        uuid.UUID       `db:"provider_id" json:"provider_id"`
	ProviderCode      string          `db:"provider_code" json:"provider_code"`
	ProviderName      string          `db:"provider_name" json:"provider_name"`
	ModelID           string          `db:"model_id" json:"model_id"`
	DisplayName       string          `db:"display_name" json:"display_name"`
	Capability        string          `db:"capability" json:"capability"`
	InputModalities   []string        `db:"input_modalities" json:"input_modalities"`
	Features          []string        `db:"features" json:"features"`
	ParameterSchema   json.RawMessage `db:"parameter_schema" json:"parameter_schema"`
	DefaultParameters json.RawMessage `db:"default_parameters" json:"default_parameters"`
	Enabled           bool            `db:"enabled" json:"enabled"`
	Available         bool            `db:"available" json:"available"`
	Metadata          json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt         time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time       `db:"updated_at" json:"updated_at"`
}

type ModelPreset struct {
	ID         uuid.UUID       `db:"id" json:"id"`
	ModelID    uuid.UUID       `db:"model_id" json:"model_id"`
	Name       string          `db:"name" json:"name"`
	Parameters json.RawMessage `db:"parameters" json:"parameters"`
	IsDefault  bool            `db:"is_default" json:"is_default"`
	CreatedAt  time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updated_at"`
}

type WorkflowTemplate struct {
	ID                uuid.UUID       `db:"id" json:"id"`
	Code              string          `db:"code" json:"code"`
	Name              string          `db:"name" json:"name"`
	Description       string          `db:"description" json:"description"`
	Capability        string          `db:"capability" json:"capability"`
	InputModalities   []string        `db:"input_modalities" json:"input_modalities"`
	Workflow          json.RawMessage `db:"workflow" json:"workflow"`
	ParameterSchema   json.RawMessage `db:"parameter_schema" json:"parameter_schema"`
	DefaultParameters json.RawMessage `db:"default_parameters" json:"default_parameters"`
	Bindings          json.RawMessage `db:"bindings" json:"bindings"`
	Outputs           json.RawMessage `db:"outputs" json:"outputs"`
	Requirements      json.RawMessage `db:"requirements" json:"requirements"`
	Enabled           bool            `db:"enabled" json:"enabled"`
	Version           int32           `db:"version" json:"version"`
	Checksum          string          `db:"checksum" json:"checksum"`
	CreatedAt         time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time       `db:"updated_at" json:"updated_at"`
}

type WorkflowCompatibility struct {
	WorkflowTemplateID uuid.UUID       `db:"workflow_template_id" json:"workflow_template_id"`
	ProviderID         uuid.UUID       `db:"provider_id" json:"provider_id"`
	Status             string          `db:"status" json:"status"`
	Report             json.RawMessage `db:"report" json:"report"`
	CheckedAt          time.Time       `db:"checked_at" json:"checked_at"`
}

type PromptPreset struct {
	ID            uuid.UUID  `db:"id" json:"id"`
	ModelID       *uuid.UUID `db:"model_id" json:"model_id"`
	ModelPresetID *uuid.UUID `db:"model_preset_id" json:"model_preset_id"`
	Name          string     `db:"name" json:"name"`
	Description   string     `db:"description" json:"description"`
	Capability    string     `db:"capability" json:"capability"`
	Content       string     `db:"content" json:"content"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`
}

type VoiceProfile struct {
	ID                   uuid.UUID       `db:"id" json:"id"`
	ProviderID           uuid.UUID       `db:"provider_id" json:"provider_id"`
	ModelID              uuid.UUID       `db:"model_id" json:"model_id"`
	ProviderCode         string          `db:"provider_code" json:"provider_code,omitempty"`
	ModelIdentifier      string          `db:"model_identifier" json:"model_identifier,omitempty"`
	Name                 string          `db:"name" json:"name"`
	Description          string          `db:"description" json:"description"`
	VoiceID              string          `db:"voice_id" json:"voice_id"`
	Status               string          `db:"status" json:"status"`
	SourceName           string          `db:"source_name" json:"source_name"`
	SourceMimeType       string          `db:"source_mime_type" json:"source_mime_type"`
	SourceFileSizeBytes  int64           `db:"source_file_size_bytes" json:"source_file_size_bytes"`
	PromptText           string          `db:"prompt_text" json:"prompt_text"`
	PreviewMimeType      string          `db:"preview_mime_type" json:"preview_mime_type"`
	PreviewFileSizeBytes int64           `db:"preview_file_size_bytes" json:"preview_file_size_bytes"`
	SourceObjectID       uuid.UUID       `db:"source_object_id" json:"source_object_id"`
	PromptObjectID       *uuid.UUID      `db:"prompt_object_id" json:"prompt_object_id"`
	PreviewObjectID      uuid.UUID       `db:"preview_object_id" json:"preview_object_id"`
	ProviderFileID       string          `db:"provider_file_id" json:"provider_file_id"`
	ProviderPromptFileID string          `db:"provider_prompt_file_id" json:"provider_prompt_file_id"`
	ActivatedAt          *time.Time      `db:"activated_at" json:"activated_at"`
	Metadata             json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt            time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time       `db:"updated_at" json:"updated_at"`
}

type GenerationJob struct {
	ID                   uuid.UUID       `db:"id" json:"id"`
	ProjectID            uuid.UUID       `db:"project_id" json:"project_id"`
	EpisodeID            *uuid.UUID      `db:"episode_id" json:"episode_id"`
	CanvasNodeID         *uuid.UUID      `db:"canvas_node_id" json:"canvas_node_id"`
	TargetAssetGroupID   *uuid.UUID      `db:"target_asset_group_id" json:"target_asset_group_id"`
	PromptPresetID       *uuid.UUID      `db:"prompt_preset_id" json:"prompt_preset_id"`
	ModelPresetID        *uuid.UUID      `db:"model_preset_id" json:"model_preset_id"`
	TargetKind           string          `db:"target_kind" json:"target_kind"`
	ProviderID           *uuid.UUID      `db:"provider_id" json:"provider_id"`
	ModelID              *uuid.UUID      `db:"model_id" json:"model_id"`
	WorkflowTemplateID   *uuid.UUID      `db:"workflow_template_id" json:"workflow_template_id"`
	TargetSnapshot       json.RawMessage `db:"target_snapshot" json:"target_snapshot"`
	Capability           string          `db:"capability" json:"capability"`
	Prompt               string          `db:"prompt" json:"prompt"`
	Parameters           json.RawMessage `db:"parameters" json:"parameters"`
	InputReferences      json.RawMessage `db:"input_references" json:"input_references"`
	InputSnapshot        json.RawMessage `db:"input_snapshot" json:"input_snapshot"`
	OutputName           string          `db:"output_name" json:"output_name"`
	OutputStagedAssetIDs []uuid.UUID     `db:"output_staged_asset_ids" json:"output_staged_asset_ids"`
	Status               string          `db:"status" json:"status"`
	Stage                string          `db:"stage" json:"stage"`
	Progress             float64         `db:"progress" json:"progress"`
	ProviderJobID        string          `db:"provider_job_id" json:"provider_job_id"`
	ErrorMessage         string          `db:"error_message" json:"error_message"`
	Attempt              int32           `db:"attempt" json:"attempt"`
	MaxAttempts          int32           `db:"max_attempts" json:"max_attempts"`
	AvailableAt          time.Time       `db:"available_at" json:"available_at"`
	LeaseUntil           *time.Time      `db:"lease_until" json:"lease_until"`
	StartedAt            *time.Time      `db:"started_at" json:"started_at"`
	FinishedAt           *time.Time      `db:"finished_at" json:"finished_at"`
	CreatedAt            time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time       `db:"updated_at" json:"updated_at"`
	ProviderCode         string          `db:"provider_code" json:"provider_code,omitempty"`
	ProviderBaseURL      string          `db:"provider_base_url" json:"-"`
	ModelIdentifier      string          `db:"model_identifier" json:"model_identifier,omitempty"`
}

type GenerationInvocation struct {
	ID                   uuid.UUID                   `db:"id" json:"id"`
	JobID                uuid.UUID                   `db:"job_id" json:"job_id"`
	ProviderCode         string                      `db:"provider_code" json:"provider_code"`
	ModelIdentifier      string                      `db:"model_identifier" json:"model_identifier"`
	Capability           string                      `db:"capability" json:"capability"`
	CredentialID         *uuid.UUID                  `db:"credential_id" json:"credential_id"`
	CredentialSource     string                      `db:"credential_source" json:"credential_source"`
	RequestSnapshot      json.RawMessage             `db:"request_snapshot" json:"request_snapshot"`
	ResponseSnapshot     json.RawMessage             `db:"response_snapshot" json:"response_snapshot"`
	UsageSnapshot        json.RawMessage             `db:"usage_snapshot" json:"usage_snapshot"`
	Status               string                      `db:"status" json:"status"`
	ProviderJobID        string                      `db:"provider_job_id" json:"provider_job_id"`
	ErrorKind            string                      `db:"error_kind" json:"error_kind"`
	ErrorMessage         string                      `db:"error_message" json:"error_message"`
	ErrorStatusCode      int32                       `db:"error_status_code" json:"error_status_code"`
	Retryable            bool                        `db:"retryable" json:"retryable"`
	OutputStagedAssetIDs []uuid.UUID                 `db:"output_staged_asset_ids" json:"output_staged_asset_ids"`
	OutputAssetIDs       []uuid.UUID                 `db:"output_asset_ids" json:"output_asset_ids"`
	StartedAt            time.Time                   `db:"started_at" json:"started_at"`
	FinishedAt           *time.Time                  `db:"finished_at" json:"finished_at"`
	DurationMS           int64                       `db:"duration_ms" json:"duration_ms"`
	CreatedAt            time.Time                   `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time                   `db:"updated_at" json:"updated_at"`
	Events               []GenerationInvocationEvent `db:"-" json:"events"`
}

type GenerationInvocationEvent struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	InvocationID   uuid.UUID       `db:"invocation_id" json:"invocation_id"`
	Sequence       int32           `db:"sequence" json:"sequence"`
	Stage          string          `db:"stage" json:"stage"`
	Progress       float64         `db:"progress" json:"progress"`
	Message        string          `db:"message" json:"message"`
	ProviderJobID  string          `db:"provider_job_id" json:"provider_job_id"`
	UsageSnapshot  json.RawMessage `db:"usage_snapshot" json:"usage_snapshot"`
	DetailSnapshot json.RawMessage `db:"detail_snapshot" json:"detail_snapshot"`
	ElapsedMS      int64           `db:"elapsed_ms" json:"elapsed_ms"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
}

type GenerationInputReference struct {
	Source         string     `json:"source"`
	ID             uuid.UUID  `json:"id"`
	RemoteExportID *uuid.UUID `json:"remote_export_id,omitempty"`
}

type GenerationReferenceUpload struct {
	ID            uuid.UUID       `db:"id" json:"id"`
	ProjectID     uuid.UUID       `db:"project_id" json:"project_id"`
	JobID         *uuid.UUID      `db:"job_id" json:"job_id"`
	ObjectID      uuid.UUID       `db:"object_id" json:"object_id"`
	Name          string          `db:"name" json:"name"`
	MediaType     string          `db:"media_type" json:"media_type"`
	MimeType      string          `db:"mime_type" json:"mime_type"`
	FileSizeBytes int64           `db:"file_size_bytes" json:"file_size_bytes"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
}

type StagedAsset struct {
	ID                 uuid.UUID       `db:"id" json:"id"`
	JobID              *uuid.UUID      `db:"job_id" json:"job_id"`
	ProjectID          uuid.UUID       `db:"project_id" json:"project_id"`
	ObjectID           uuid.UUID       `db:"object_id" json:"object_id"`
	Source             string          `db:"source" json:"source"`
	Name               string          `db:"name" json:"name"`
	MediaType          string          `db:"media_type" json:"media_type"`
	MimeType           string          `db:"mime_type" json:"mime_type"`
	FileSizeBytes      int64           `db:"file_size_bytes" json:"file_size_bytes"`
	ContentText        string          `db:"content_text" json:"content_text"`
	OriginalURL        string          `db:"original_url" json:"original_url"`
	ProviderCode       string          `db:"provider_code" json:"provider_code"`
	ModelIdentifier    string          `db:"model_identifier" json:"model_identifier"`
	ParametersSnapshot json.RawMessage `db:"parameters_snapshot" json:"parameters_snapshot"`
	InputSnapshot      json.RawMessage `db:"input_snapshot" json:"input_snapshot"`
	Metadata           json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt          time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at" json:"updated_at"`
	AssetID            *uuid.UUID      `db:"asset_id" json:"asset_id"`
	AssetGroupID       *uuid.UUID      `db:"asset_group_id" json:"asset_group_id"`
	AssetName          string          `db:"asset_name" json:"asset_name"`
	TargetAssetGroupID *uuid.UUID      `db:"target_asset_group_id" json:"target_asset_group_id"`
	JobStatus          string          `db:"job_status" json:"job_status"`
	Capability         string          `db:"capability" json:"capability"`
	Prompt             string          `db:"prompt" json:"prompt"`
	ErrorMessage       string          `db:"error_message" json:"error_message"`
}

type StagedAssetPage struct {
	Items    []StagedAsset `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

type StagedAssetSummary struct {
	Total        int64            `json:"total"`
	Unprocessed  int64            `json:"unprocessed"`
	Imported     int64            `json:"imported"`
	ByCapability map[string]int64 `json:"by_capability"`
}

type S3Connection struct {
	ID                   uuid.UUID `db:"id" json:"id"`
	Name                 string    `db:"name" json:"name"`
	Provider             string    `db:"provider" json:"provider"`
	Endpoint             string    `db:"endpoint" json:"endpoint"`
	PublicEndpoint       string    `db:"public_endpoint" json:"public_endpoint"`
	Region               string    `db:"region" json:"region"`
	Bucket               string    `db:"bucket" json:"bucket"`
	Prefix               string    `db:"prefix" json:"prefix"`
	ForcePathStyle       bool      `db:"force_path_style" json:"force_path_style"`
	EncryptedCredentials string    `db:"encrypted_credentials" json:"-"`
	Enabled              bool      `db:"enabled" json:"enabled"`
	IsDefault            bool      `db:"is_default" json:"is_default"`
	CreatedAt            time.Time `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time `db:"updated_at" json:"updated_at"`
}

type LocalObject struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	ProjectID    *uuid.UUID `db:"project_id" json:"project_id"`
	ObjectKey    string     `db:"object_key" json:"object_key"`
	OriginalName string     `db:"original_name" json:"original_name"`
	Purpose      string     `db:"purpose" json:"purpose"`
	MimeType     string     `db:"mime_type" json:"mime_type"`
	SizeBytes    int64      `db:"size_bytes" json:"size_bytes"`
	SHA256       string     `db:"sha256" json:"sha256"`
	State        string     `db:"state" json:"state"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
}

type AssetRemoteExport struct {
	ID           uuid.UUID `db:"id" json:"id"`
	AssetID      uuid.UUID `db:"asset_id" json:"asset_id"`
	ConnectionID uuid.UUID `db:"connection_id" json:"connection_id"`
	ObjectKey    string    `db:"object_key" json:"object_key"`
	ETag         string    `db:"etag" json:"etag"`
	PublicURL    string    `db:"public_url" json:"public_url"`
	State        string    `db:"state" json:"state"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}
