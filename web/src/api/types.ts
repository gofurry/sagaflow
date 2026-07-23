export type ID = string

export interface Account { id: ID; username: string; display_name: string; last_login_at: string | null; created_at: string; updated_at: string }
export interface Principal { account_id: ID; session_id: ID; username: string; display_name: string }
export interface CurrentUser { user: Principal }
export interface S3Connection { id: ID; name: string; provider: string; endpoint: string; public_endpoint: string; region: string; bucket: string; prefix: string; force_path_style: boolean; enabled: boolean; is_default: boolean; created_at: string; updated_at: string }
export interface AssetRemoteExport {
  id: ID
  asset_id: ID
  connection_id: ID
  object_key: string
  etag: string
  public_url: string
  state: string
  connection_name: string
  connection_is_default: boolean
  connection_enabled: boolean
  created_at: string
  updated_at: string
}
export interface Project { id: ID; title: string; description: string; created_at: string; updated_at: string }
export interface Episode { id: ID; project_id: ID; episode_number: number; title: string; notes: string; target_duration_seconds: number | null; target_shot_count: number | null; created_at: string; updated_at: string }
export interface EpisodeScript { id: ID; episode_id: ID; version: number; body: string; note: string; status: AssetStatus; created_at: string }
export type AssetKind = 'character' | 'scene' | 'prop' | 'material'
export type AssetStatus = 'candidate' | 'adopted' | 'discarded'
export type MediaType = 'image' | 'audio' | 'video' | 'text' | 'file'
export interface AssetGroup { id: ID; project_id: ID; parent_id: ID | null; kind: AssetKind; name: string; description: string; sort_order: number; created_at: string; updated_at: string }
export interface Asset { id: ID; project_id: ID; group_id: ID | null; staged_asset_id: ID | null; episode_id: ID | null; canvas_node_id: ID | null; name: string; media_type: MediaType; source: 'upload' | 'generated'; status: AssetStatus; mime_type: string; file_size_bytes: number; storage_backend: string; original_url: string; provider_code: string; model_identifier: string; metadata: Record<string, unknown>; created_at: string; updated_at: string }
export type MediaTool = 'inspect' | 'transcode' | 'aspect' | 'audio' | 'trim' | 'merge' | 'subtitle' | 'screenshot'
export interface MediaToolsStatus { available: boolean; ffmpeg_path: string; ffprobe_path: string; version: string; source: string; message: string }
export interface MediaJob {
  id: ID
  project_id: ID
  target_asset_group_id: ID | null
  tool: MediaTool
  source_asset_ids: ID[]
  output_name: string
  parameters: Record<string, unknown>
  status: JobStatus
  stage: string
  progress: number
  output_asset_id: ID | null
  output_staged_asset_id: ID | null
  command_snapshot: string[]
  probe_snapshot: Record<string, unknown>
  error_message: string
  cancel_requested: boolean
  started_at: string | null
  finished_at: string | null
  created_at: string
  updated_at: string
}
export interface CanvasNodeDTO { id: ID; type: string; position: { x: number; y: number }; width?: number; height?: number; z_index?: number; data: CanvasNodeData }
export interface CanvasEdgeDTO { id: ID; source: ID; target: ID; source_handle?: string | null; target_handle?: string | null; type: CanvasEdgeKind; data?: CanvasEdgeData }
export interface CanvasAnnotationDTO { id: ID; type: CanvasAnnotationKind; position: { x: number; y: number }; width: number; height: number; stroke_color: string; stroke_width: number; line_style: CanvasAnnotationLineStyle; opacity: number; label: string; label_position: CanvasAnnotationLabelPosition; z_index?: number }
export interface CanvasDocument { nodes: CanvasNodeDTO[]; edges: CanvasEdgeDTO[]; annotations: CanvasAnnotationDTO[] }
export type CanvasNodeKind = 'asset' | 'video' | 'note'
export type CanvasEdgeKind = 'reference' | 'annotation' | 'relation'
export type CanvasEdgeRouting = 'curve' | 'step'
export type CanvasAnnotationKind = 'arrow' | 'line' | 'rectangle' | 'ellipse'
export type CanvasAnnotationLineStyle = 'solid' | 'dashed'
export type CanvasAnnotationLabelPosition = 'top-left' | 'top-center' | 'top-right' | 'middle-left' | 'center' | 'middle-right' | 'bottom-left' | 'bottom-center' | 'bottom-right'
export interface CanvasEdgeData extends Record<string, unknown> { relation: CanvasEdgeKind; routing?: CanvasEdgeRouting; note?: string }
export interface CanvasNodeData extends Record<string, unknown> { kind: CanvasNodeKind; title: string; body?: string; asset_id?: ID; asset_status?: AssetStatus; media_type?: MediaType; group_id?: ID | null; group_path?: string; shot_number?: number; target_duration_seconds?: number; selected_video_asset_id?: ID; color?: string }
export type Capability = 'text' | 'image' | 'audio' | 'video' | 'multimodal'
export type ProviderAuthType = 'none' | 'api_key' | 'bearer'
export interface ModelProvider { id: ID; code: string; adapter_code: string; display_name: string; base_url: string; auth_type: ProviderAuthType; capabilities: Capability[]; enabled: boolean; metadata: Record<string, unknown>; created_at: string; updated_at: string }
export interface Model { id: ID; provider_id: ID; provider_code: string; provider_name: string; model_id: string; display_name: string; capability: Capability; input_modalities: MediaType[]; features: string[]; parameter_schema: JSONSchema; default_parameters: Record<string, unknown>; enabled: boolean; available: boolean; metadata: Record<string, unknown>; created_at: string; updated_at: string }
export interface OllamaServerInfo { version: string; model_count: number; running_count: number }
export interface OllamaModelInfo { name: string; digest: string; size: number; modified_at: string; details: Record<string, unknown>; capabilities: string[]; parameters: string; context_length: number; input_modalities: MediaType[]; features: string[]; supports_text_output: boolean }
export interface OllamaDiscovery { server: OllamaServerInfo; models: OllamaModelInfo[] }
export interface SiliconFlowServerInfo { provider: 'siliconflow'; model_count: number; last_checked_at: string }
export interface SiliconFlowModelInfo { name: string; display_name: string; task: string; capability: Capability; input_modalities: MediaType[]; features: string[]; support_status: ModelSupportStatus; supports_generation: boolean }
export interface SiliconFlowDiscovery { server: SiliconFlowServerInfo; models: SiliconFlowModelInfo[] }
export interface TencentTokenHubServerInfo { provider: 'tencent_tokenhub'; model_count: number; last_checked_at: string }
export interface TencentTokenHubModelInfo extends SiliconFlowModelInfo { lifecycle_status: ModelLifecycleStatus; remote_status: string }
export interface TencentTokenHubDiscovery { server: TencentTokenHubServerInfo; models: TencentTokenHubModelInfo[] }
export interface MoonshotServerInfo { provider: 'moonshot'; model_count: number; last_checked_at: string }
export interface MoonshotModelInfo extends TencentTokenHubModelInfo { context_length: number }
export interface MoonshotDiscovery { server: MoonshotServerInfo; models: MoonshotModelInfo[] }
export type CloudModelDiscovery = SiliconFlowDiscovery | TencentTokenHubDiscovery | MoonshotDiscovery
export type ModelSupportStatus = 'verified' | 'compatible' | 'experimental'
export type ModelLifecycleStatus = 'active' | 'deprecated' | 'retired'
export interface ModelSyncResult { server: ConnectionServerInfo; models: Model[]; imported: number; updated: number }
export interface CatalogManifestInfo { path: string; installed: boolean; schema_version: number; catalog_version: string; published_at?: string; model_count: number }
export interface CatalogUpdateStatus { overlay: CatalogManifestInfo; source_url: string }
export interface CatalogImportResult { overlay: CatalogManifestInfo; sync: { created: number; updated: number; skipped: number } }
export interface ComfyUIDeviceInfo { name: string; type: string; index: number | null; vram_total: number; vram_free: number }
export interface ComfyUIServerInfo { version: string; system: Record<string, unknown>; devices: ComfyUIDeviceInfo[]; features: Record<string, unknown>; node_count: number; model_count: number }
export interface ComfyUIDiscovery { server: ComfyUIServerInfo; nodes: string[]; models: Record<string, string[]> }
export type ConnectionServerInfo = OllamaServerInfo | ComfyUIServerInfo | SiliconFlowServerInfo | TencentTokenHubServerInfo | MoonshotServerInfo
export type ConnectionDiscovery = OllamaDiscovery | ComfyUIDiscovery | CloudModelDiscovery
export interface JSONSchema {
  type?: string
  properties?: Record<string, {
    type?: string
    enum?: unknown[]
    minimum?: number
    maximum?: number
    multipleOf?: number
    title?: string
    description?: string
    format?: string
    readOnly?: boolean
    default?: unknown
    items?: { type?: string }
  }>
}
export interface ModelPreset { id: ID; model_id: ID; name: string; parameters: Record<string, unknown>; is_default: boolean; created_at: string; updated_at: string }
export interface WorkflowTemplate { id: ID; code: string; name: string; description: string; capability: Exclude<Capability, 'multimodal'>; input_modalities: MediaType[]; workflow: Record<string, unknown>; parameter_schema: JSONSchema; default_parameters: Record<string, unknown>; bindings: Record<string, unknown>; outputs: unknown[]; requirements: Record<string, unknown>; enabled: boolean; version: number; checksum: string; created_at: string; updated_at: string }
export interface WorkflowBinding { node_id: string; input: string; source: 'prompt' | 'parameter' | 'input'; parameter?: string; input_index?: number; required?: boolean }
export interface WorkflowOutputSelector { node_id: string; key: string; media_type: MediaType; mime_type?: string }
export interface WorkflowRequirements { nodes: string[]; models: Array<{ folder: string; name: string }> }
export interface WorkflowAnalysisIssue { level: 'info' | 'warning'; code: string; message: string; node_ids?: string[] }
export interface WorkflowAnalysis {
  workflow: Record<string, unknown>
  capability: Exclude<Capability, 'multimodal'>
  input_modalities: MediaType[]
  parameter_schema: JSONSchema
  default_parameters: Record<string, unknown>
  bindings: Record<string, WorkflowBinding>
  outputs: WorkflowOutputSelector[]
  requirements: WorkflowRequirements
  issues: WorkflowAnalysisIssue[]
  summary: { node_count: number; parameter_count: number; binding_count: number; input_count: number; output_count: number; model_count: number }
}
export type WorkflowCompatibilityStatus = 'unknown' | 'ready' | 'missing_nodes' | 'missing_resources' | 'incompatible'
export interface WorkflowCompatibility { workflow_template_id: ID; provider_id: ID; status: WorkflowCompatibilityStatus; report: { missing_nodes?: string[]; missing_resources?: Array<{ folder: string; name: string }>; required_nodes?: string[]; checked_at?: string }; checked_at: string }
export interface PromptPreset { id: ID; model_id: ID | null; model_preset_id: ID | null; name: string; description: string; capability: Exclude<Capability, 'multimodal'>; content: string; created_at: string; updated_at: string }
export interface ProviderCredential { id: ID; provider_id: ID; provider_code: string; name: string; key_hint: string; is_active: boolean; created_at: string; updated_at: string; last_used_at: string | null }
export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
export type JobStage = 'queued' | 'requesting' | 'generating' | 'fetching' | 'storing' | 'completed' | 'failed' | 'canceled' | 'interrupted'
export type GenerationReferenceSource = 'asset' | 'upload'
export interface GenerationInputReference { source: GenerationReferenceSource; id: ID; remote_export_id?: ID }
export interface GenerationReferenceUpload { id: ID; project_id: ID; job_id: ID | null; name: string; media_type: MediaType; mime_type: string; file_size_bytes: number; created_at: string }
export interface GenerationJob { id: ID; project_id: ID; episode_id: ID | null; canvas_node_id: ID | null; target_asset_group_id: ID | null; prompt_preset_id: ID | null; model_preset_id: ID | null; target_kind: 'model' | 'workflow'; provider_id: ID | null; model_id: ID | null; workflow_template_id: ID | null; capability: Capability; prompt: string; parameters: Record<string, unknown>; input_references: GenerationInputReference[]; output_name: string; output_staged_asset_ids: ID[]; status: JobStatus; stage: JobStage; provider_job_id: string; error_message: string; provider_code: string; model_identifier: string; created_at: string; updated_at: string }
export interface InferenceInputSnapshot { id: string; name: string; media_type: MediaType | string; mime_type: string; provider_url: boolean; content_stream: boolean }
export interface InferenceRequestSnapshot { request_id: string; provider_code: string; adapter_code: string; endpoint: string; target_kind: 'model' | 'workflow'; target_id: string; capability: Capability; prompt: string; parameters: Record<string, unknown>; inputs: InferenceInputSnapshot[] }
export interface InferenceArtifactSnapshot { media_type: MediaType | string; mime_type: string; source_url?: string; metadata: Record<string, unknown> }
export interface InferenceResultSnapshot { artifacts: InferenceArtifactSnapshot[]; usage: Record<string, unknown> }
export interface GenerationInvocationEvent { id: ID; invocation_id: ID; sequence: number; stage: string; progress: number; message: string; provider_job_id: string; usage_snapshot: Record<string, unknown>; detail_snapshot: Record<string, unknown>; elapsed_ms: number; created_at: string }
export interface GenerationInvocation { id: ID; job_id: ID; provider_code: string; model_identifier: string; capability: Capability; credential_id: ID | null; credential_source: string; request_snapshot: InferenceRequestSnapshot; response_snapshot: InferenceResultSnapshot; usage_snapshot: Record<string, unknown>; status: 'running' | 'succeeded' | 'failed'; provider_job_id: string; error_kind: string; error_message: string; error_status_code: number; retryable: boolean; output_staged_asset_ids: ID[]; output_asset_ids: ID[]; started_at: string; finished_at: string | null; duration_ms: number; created_at: string; updated_at: string; events: GenerationInvocationEvent[] }
export interface StagedAsset { id: ID; job_id: ID | null; project_id: ID; source: 'generated' | 'upload'; name: string; media_type: MediaType; mime_type: string; file_size_bytes: number; content_text: string; original_url: string; provider_code: string; model_identifier: string; parameters_snapshot: Record<string, unknown>; input_snapshot: Record<string, unknown>; metadata: Record<string, unknown>; created_at: string; updated_at: string; asset_id: ID | null; asset_group_id: ID | null; asset_name: string; target_asset_group_id: ID | null; job_status: JobStatus | ''; capability: Capability | ''; prompt: string; error_message: string }
export interface StagedAssetPage { items: StagedAsset[]; total: number; page: number; page_size: number }
export interface StagedAssetSummary { total: number; unprocessed: number; imported: number; by_capability: Partial<Record<Capability, number>> }
export interface VoiceProfile { id: ID; provider_id: ID; model_id: ID; provider_code: string; model_identifier: string; name: string; description: string; voice_id: string; status: 'ready' | 'error'; source_name: string; source_mime_type: string; source_file_size_bytes: number; prompt_text: string; preview_mime_type: string; preview_file_size_bytes: number; activated_at: string | null; metadata: Record<string, unknown>; created_at: string; updated_at: string }
export interface AuthStatus { enabled: boolean; initialized: boolean; authenticated: boolean; user: Principal | null }
