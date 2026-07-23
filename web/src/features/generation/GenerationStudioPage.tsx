import { useEffect, useMemo, useState } from 'react'
import { AppstoreOutlined, AudioOutlined, BarsOutlined, FileTextOutlined, PlusOutlined, ReloadOutlined, ThunderboltOutlined, VideoCameraOutlined } from '@ant-design/icons'
import { App, AutoComplete, Button, Input, Select, Steps } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Asset, Capability, CanvasEdgeDTO, CanvasNodeDTO, Episode, GenerationJob, MediaType, Model, ModelPreset, ModelProvider, Project, WorkflowCompatibility, WorkflowTemplate } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { MarkdownEditor } from '../../components/Markdown'
import { ModelParameterEditor } from '../../components/ModelParameterEditor'
import { GenerationReferencePicker, type GenerationReferenceDraft } from './GenerationReferencePicker'
import { StagedAssetGallery, type ResultViewMode } from './StagedAssetGallery'

type GenerationCapability = Extract<Capability, 'text' | 'image' | 'audio' | 'video'>
interface GenerationDraft {
  targetKind?: 'model' | 'workflow'
  modelID?: string
  workflowID?: string
  providerID?: string
  promptPresetID?: string
  modelPresetID?: string
  shotID?: string
  outputName: string
  prompt: string
  parameters: Record<string, unknown>
  parametersCustomized: boolean
  inputReferences: GenerationReferenceDraft[]
}

interface CreateJobVariables {
  capability: GenerationCapability
  request: Parameters<typeof api.createJob>[0]
}

const capabilities: GenerationCapability[] = ['text', 'image', 'audio', 'video']
const capabilityMeta: Record<GenerationCapability, { label: string; references: MediaType[] }> = {
  text: { label: '文本', references: [] },
  image: { label: '图像', references: ['image'] },
  audio: { label: '音频', references: [] },
  video: { label: '视频', references: ['image', 'video', 'audio'] },
}
const BASE_PARAMETERS = '__model_base__'
const CUSTOM_PARAMETERS = '__custom__'
const emptyDraft = (): GenerationDraft => ({ outputName: '', prompt: '', parameters: {}, parametersCustomized: false, inputReferences: [] })

export function GenerationStudioPage({ episode, onError, project }: { episode: Episode | null; onError: (error: unknown) => void; project: Project }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [capability, setCapability] = useState<GenerationCapability>('text')
  const [viewMode, setViewMode] = useState<ResultViewMode>('grid')
  const [sessionJobIDs, setSessionJobIDs] = useState<string[]>([])
  const [selectedJobIDs, setSelectedJobIDs] = useState<Partial<Record<GenerationCapability, string>>>({})
  const [drafts, setDrafts] = useState<Record<GenerationCapability, GenerationDraft>>({
    text: emptyDraft(),
    image: emptyDraft(),
    audio: emptyDraft(),
    video: emptyDraft(),
  })
	const modelsQuery = useQuery({ queryKey: ['models', project.id], queryFn: () => api.models(undefined, project.id) })
  const providersQuery = useQuery({ queryKey: ['providers'], queryFn: api.providers })
	const workflowsQuery = useQuery({ queryKey: ['workflows', project.id], queryFn: () => api.workflows(undefined, project.id) })
  const compatibilityQuery = useQuery({ queryKey: ['workflow-compatibilities'], queryFn: () => api.workflowCompatibilities() })
  const presetsQuery = useQuery({ queryKey: ['presets'], queryFn: () => api.presets() })
  const promptsQuery = useQuery({ queryKey: ['prompt-presets'], queryFn: () => api.promptPresets() })
  const assetsQuery = useQuery({ queryKey: ['assets', project.id], queryFn: () => api.assets(project.id) })
  const groupsQuery = useQuery({ queryKey: ['asset-groups', project.id], queryFn: () => api.assetGroups(project.id) })
  const stagedSummaryQuery = useQuery({ queryKey: ['staged-summary', project.id], queryFn: () => api.stagedAssetSummary(project.id) })
  const voicesQuery = useQuery({ queryKey: ['voice-profiles'], queryFn: api.voiceProfiles })
  const canvasQuery = useQuery({ queryKey: ['canvas', episode?.id], queryFn: () => api.canvas(episode!.id), enabled: !!episode })
  const jobsQuery = useQuery({
    queryKey: ['jobs', project.id],
    queryFn: () => api.jobs(project.id),
    refetchInterval: (query) => (query.state.data ?? []).some((job) => job.status === 'queued' || job.status === 'running') ? 2500 : false,
  })
  const draft = drafts[capability]
  const allModels = useMemo(() => modelsQuery.data ?? [], [modelsQuery.data])
  const providers = useMemo(() => providersQuery.data ?? [], [providersQuery.data])
  const workflows = useMemo(() => workflowsQuery.data ?? [], [workflowsQuery.data])
  const compatibilities = useMemo(() => compatibilityQuery.data ?? [], [compatibilityQuery.data])
  const allPresets = useMemo(() => presetsQuery.data ?? [], [presetsQuery.data])
  const models = useMemo(() => allModels.filter((model) => model.enabled && model.available && model.capability === capability), [allModels, capability])
  const model = models.find((item) => item.id === draft.modelID)
  const workflow = workflows.find((item) => item.id === draft.workflowID)
  const targetDefinition = draft.targetKind === 'workflow' ? workflow : model
  const readyWorkflowTargets = useMemo(() => workflowTargets(workflows, compatibilities, providers, capability), [capability, compatibilities, providers, workflows])
  const allowedReferences = useMemo(() => targetDefinition
    ? targetDefinition.input_modalities.filter((item): item is MediaType => item !== 'text')
    : capabilityMeta[capability].references, [capability, targetDefinition])
  const requiresPublishedAssets = draft.targetKind === 'model' && !!model && requiresPublishedReferences(model, providers)
  const modelPresets = useMemo(() => allPresets.filter((preset) => preset.model_id === draft.modelID), [allPresets, draft.modelID])
  const prompts = useMemo(() => (promptsQuery.data ?? []).filter((preset) => preset.capability === capability), [capability, promptsQuery.data])
  const allJobs = useMemo(() => jobsQuery.data ?? [], [jobsQuery.data])
  const relevantJobs = useMemo(() => allJobs.filter((job) => capabilities.includes(job.capability as GenerationCapability) && (job.capability !== 'video' || job.episode_id === episode?.id)), [allJobs, episode?.id])
  const jobs = useMemo(() => relevantJobs.filter((job) => job.capability === capability), [capability, relevantJobs])
  const taskJobs = useMemo(() => {
    const session = new Set(sessionJobIDs)
    return relevantJobs.filter((job) => session.has(job.id) || job.status === 'queued' || job.status === 'running')
  }, [relevantJobs, sessionJobIDs])
  const videoShots = useMemo(() => (canvasQuery.data?.nodes ?? []).filter((node) => node.data.kind === 'video').sort((left, right) => (left.data.shot_number ?? 0) - (right.data.shot_number ?? 0)), [canvasQuery.data?.nodes])
  const selectedShot = videoShots.find((node) => node.id === draft.shotID)
  const videoReferences = useMemo(() => selectedShot ? canvasReferenceAssets(selectedShot, canvasQuery.data?.nodes ?? [], canvasQuery.data?.edges ?? [], assetsQuery.data ?? []) : [], [assetsQuery.data, canvasQuery.data?.edges, canvasQuery.data?.nodes, selectedShot])
  const unsupportedVideoReferences = useMemo(() => targetDefinition && capability === 'video'
    ? videoReferences.filter((asset) => !allowedReferences.includes(asset.media_type))
    : [], [allowedReferences, capability, targetDefinition, videoReferences])
  const videoNotes = useMemo(() => selectedShot ? canvasNotes(selectedShot, canvasQuery.data?.nodes ?? [], canvasQuery.data?.edges ?? []) : [], [canvasQuery.data?.edges, canvasQuery.data?.nodes, selectedShot])
  const selectedJobID = selectedJobIDs[capability]
  const selectedJob = selectedJobID ? jobs.find((job) => job.id === selectedJobID) : undefined
  const updateDraft = (patch: Partial<GenerationDraft>) => setDrafts((current) => ({ ...current, [capability]: { ...current[capability], ...patch } }))
  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['jobs', project.id] }),
    queryClient.invalidateQueries({ queryKey: ['staged-assets', project.id] }),
    queryClient.invalidateQueries({ queryKey: ['staged-summary', project.id] }),
    queryClient.invalidateQueries({ queryKey: ['prompt-presets'] }),
    queryClient.invalidateQueries({ queryKey: ['presets'] }),
    queryClient.invalidateQueries({ queryKey: ['workflows'] }),
    queryClient.invalidateQueries({ queryKey: ['workflow-compatibilities'] }),
  ])
  const createJob = useMutation({
    mutationFn: ({ request }: CreateJobVariables) => api.createJob(request),
    onSuccess: async (job, variables) => {
      queryClient.setQueryData<GenerationJob[]>(['jobs', project.id], (current = []) => [job, ...current.filter((item) => item.id !== job.id)])
      setSessionJobIDs((current) => current.includes(job.id) ? current : [job.id, ...current])
      setSelectedJobIDs((current) => ({ ...current, [variables.capability]: job.id }))
      setDrafts((current) => ({ ...current, [variables.capability]: { ...current[variables.capability], inputReferences: [] } }))
      await queryClient.invalidateQueries({ queryKey: ['jobs', project.id] })
      message.success('生成任务已进入队列')
    },
    onError,
  })
  useEffect(() => {
    if (!jobs.some((job) => job.status === 'succeeded' && job.output_staged_asset_ids.length > 0)) return
    queryClient.invalidateQueries({ queryKey: ['staged-assets', project.id] })
    queryClient.invalidateQueries({ queryKey: ['staged-summary', project.id] })
    if (capability === 'video' && episode) {
      queryClient.invalidateQueries({ queryKey: ['assets', project.id] })
      queryClient.invalidateQueries({ queryKey: ['canvas', episode.id] })
    }
  }, [capability, episode, jobs, project.id, queryClient])

  useEffect(() => {
    setSessionJobIDs([])
    setSelectedJobIDs({})
  }, [project.id])

  useEffect(() => {
    setSelectedJobIDs((current) => current.video ? { ...current, video: undefined } : current)
  }, [episode?.id])

  useEffect(() => {
    setDrafts((current) => {
      const video = current.video
      if (!video.shotID || videoShots.some((shot) => shot.id === video.shotID)) return current
      return { ...current, video: { ...video, shotID: undefined } }
    })
  }, [episode?.id, videoShots])

  const selectTarget = (key: string) => {
    const [kind, firstID, secondID] = key.split(':')
    if (kind === 'model') {
      const next = models.find((item) => item.id === firstID)
      if (!next) return
      const defaultPreset = defaultModelPreset(allPresets, next)
      const inputReferences = compatibleDraftReferences(draft.inputReferences, next.input_modalities, requiresPublishedReferences(next, providers))
      discardTemporaryReferences(draft.inputReferences, inputReferences)
      updateDraft({
        targetKind: 'model', modelID: next.id, workflowID: undefined, providerID: undefined,
        modelPresetID: defaultPreset?.id, parameters: defaultPreset?.parameters ?? next.default_parameters,
        parametersCustomized: false,
        inputReferences,
      })
      return
    }
    const next = readyWorkflowTargets.find((item) => item.workflow.id === firstID && item.provider.id === secondID)
    if (!next) return
    const inputReferences = compatibleDraftReferences(draft.inputReferences, next.workflow.input_modalities, false)
    discardTemporaryReferences(draft.inputReferences, inputReferences)
    updateDraft({
      targetKind: 'workflow', modelID: undefined, modelPresetID: undefined, workflowID: next.workflow.id, providerID: next.provider.id,
      parameters: next.workflow.default_parameters, parametersCustomized: false,
      inputReferences,
    })
  }
  const selectPrompt = (presetID?: string) => {
    if (!presetID) {
      updateDraft({ promptPresetID: undefined })
      return
    }
    const preset = prompts.find((item) => item.id === presetID)
    if (!preset) return
    const nextModel = preset.model_id ? models.find((item) => item.id === preset.model_id) : undefined
    if (!nextModel) {
      updateDraft({ promptPresetID: preset.id, prompt: preset.content })
      return
    }
    const recommended = preset.model_preset_id ? allPresets.find((item) => item.id === preset.model_preset_id && item.model_id === nextModel.id) : undefined
    const nextPreset = recommended ?? defaultModelPreset(allPresets, nextModel)
    const inputReferences = compatibleDraftReferences(draft.inputReferences, nextModel.input_modalities, requiresPublishedReferences(nextModel, providers))
    discardTemporaryReferences(draft.inputReferences, inputReferences)
    updateDraft({
      promptPresetID: preset.id,
      prompt: preset.content,
      targetKind: 'model',
      modelID: nextModel.id,
      workflowID: undefined,
      providerID: undefined,
      modelPresetID: nextPreset?.id,
      parameters: nextPreset?.parameters ?? nextModel.default_parameters,
      parametersCustomized: false,
      inputReferences,
    })
  }
  const selectModelPreset = (value: string) => {
    if (!model) return
    if (value === BASE_PARAMETERS) {
      updateDraft({ modelPresetID: undefined, parameters: model.default_parameters, parametersCustomized: false })
      return
    }
    const preset = modelPresets.find((item) => item.id === value)
    if (preset) updateDraft({ modelPresetID: preset.id, parameters: preset.parameters, parametersCustomized: false })
  }
  const selectShot = (shotID: string) => {
    const shot = videoShots.find((item) => item.id === shotID)
    if (!shot) return
    updateDraft({
      shotID,
      outputName: shot.data.title,
      prompt: shot.data.body ?? '',
      inputReferences: [],
    })
  }
  const appendVideoNotes = () => {
    const noteText = videoNotes.map((note) => `## ${note.data.title}\n\n${note.data.body ?? ''}`).join('\n\n')
    if (!noteText) return
    updateDraft({ prompt: [draft.prompt.trim(), noteText].filter(Boolean).join('\n\n') })
  }
  const run = () => {
    if (capability === 'video' && (!episode || !draft.shotID)) return message.warning('请先选择当前分集的视频分镜')
    if (!draft.targetKind || (draft.targetKind === 'model' ? !draft.modelID : !draft.workflowID || !draft.providerID)) return message.warning('请先选择生成目标')
    if (capability === 'video' && unsupportedVideoReferences.length > 0) return message.warning('画布包含当前生成目标不支持的参考类型，请更换目标或调整参考连线')
    if (!draft.prompt.trim()) return message.warning('请输入 Prompt')
    createJob.mutate({
      capability,
      request: {
        project_id: project.id,
        episode_id: capability === 'video' ? episode?.id : undefined,
        canvas_node_id: capability === 'video' ? draft.shotID : undefined,
        prompt_preset_id: draft.promptPresetID,
        model_preset_id: draft.targetKind === 'model' ? draft.modelPresetID : undefined,
        target_kind: draft.targetKind,
        model_id: draft.targetKind === 'model' ? draft.modelID : undefined,
        workflow_template_id: draft.targetKind === 'workflow' ? draft.workflowID : undefined,
        provider_id: draft.targetKind === 'workflow' ? draft.providerID : undefined,
        output_name: draft.outputName.trim() || undefined,
        prompt: draft.prompt,
        parameters: draft.parameters,
        input_references: capability === 'video' ? undefined : draft.inputReferences.map(({ source, id }) => ({ source, id })),
      },
    })
  }
  const resetDraft = () => {
    draft.inputReferences.filter((reference) => reference.source === 'upload').forEach((reference) => void api.deleteGenerationReference(reference.id).catch(() => undefined))
    setDrafts((current) => ({ ...current, [capability]: emptyDraft() }))
    setSelectedJobIDs((current) => ({ ...current, [capability]: undefined }))
  }
  const focusTask = (job: GenerationJob) => {
    if (!capabilities.includes(job.capability as GenerationCapability)) return
    const nextCapability = job.capability as GenerationCapability
    setCapability(nextCapability)
    setSelectedJobIDs((current) => ({ ...current, [nextCapability]: job.id }))
  }
  const refreshing = stagedSummaryQuery.isFetching || jobsQuery.isFetching
  const voiceParameterKey = capability === 'audio' && draft.targetKind === 'model' && model ? modelVoiceParameter(model) : undefined
  const modelVoices = voiceParameterKey && model ? (voicesQuery.data ?? []).filter((voice) => voice.provider_id === model.provider_id) : []
  const parameterPresetValue = draft.targetKind === 'workflow' ? BASE_PARAMETERS : draft.parametersCustomized ? CUSTOM_PARAMETERS : (draft.modelPresetID ?? BASE_PARAMETERS)
  const selectedTargetKey = draft.targetKind === 'workflow' && draft.workflowID && draft.providerID
    ? `workflow:${draft.workflowID}:${draft.providerID}`
    : draft.modelID ? `model:${draft.modelID}` : undefined

  return <div className="page page-generation">
    <FloatingToolbar ariaLabel="生成工具栏" items={[
      { key: 'run', label: `开始生成${capabilityMeta[capability].label}`, icon: <ThunderboltOutlined/>, active: true, disabled: !selectedTargetKey || !draft.prompt.trim() || (capability === 'video' && (!draft.shotID || unsupportedVideoReferences.length > 0)), loading: createJob.isPending, onClick: run },
      { key: 'new', label: '新建生成', icon: <PlusOutlined/>, onClick: resetDraft },
      { key: 'refresh', label: '刷新任务与结果', icon: <ReloadOutlined/>, loading: refreshing, onClick: () => void refresh() },
    ]}/>

    <section className="generation-page-content">
      <div aria-label="生成资源类型" className="generation-kind-tabs" role="tablist">
        {capabilities.map((item) => <button aria-selected={capability === item} className={capability === item ? 'active' : ''} key={item} onClick={() => setCapability(item)} role="tab" type="button">
          <strong>{capabilityMeta[item].label}</strong>
          <em>{stagedSummaryQuery.data?.by_capability[item] ?? 0}</em>
        </button>)}
      </div>

      <div className="generation-workbench">
        {capability === 'video' && <div className="generation-shot-selector">
          <label>
            <span>当前分集视频分镜</span>
            <Select
              disabled={!episode || !videoShots.length}
              onChange={selectShot}
              options={videoShots.map((shot) => ({ value: shot.id, label: `${String(shot.data.shot_number ?? 0).padStart(2, '0')} · ${shot.data.title}` }))}
              placeholder={!episode ? '请先选择分集' : videoShots.length ? '选择要生成的视频分镜' : '当前分集还没有视频分镜'}
              value={draft.shotID}
            />
          </label>
          <p>{selectedShot ? `${selectedShot.data.target_duration_seconds ?? '未设'} 秒 · ${videoReferences.length} 个参考资产 · ${videoNotes.length} 条关联备注` : '视频生成必须绑定画布中的一个视频分镜。'}</p>
        </div>}
        <div className="generation-fields">
          <label>
            <span>Prompt 预设</span>
            <Select allowClear onChange={selectPrompt} options={prompts.map((preset) => ({ value: preset.id, label: preset.name }))} placeholder="可选，从模型页管理" value={draft.promptPresetID}/>
          </label>
          <label>
            <span>生成目标</span>
            <Select
              onChange={selectTarget}
              options={[
                { label: '模型', options: models.map((item) => ({ value: `model:${item.id}`, label: `${item.provider_name} · ${item.display_name}` })) },
                { label: 'ComfyUI 工作流', options: readyWorkflowTargets.map((item) => ({ value: `workflow:${item.workflow.id}:${item.provider.id}`, label: `${item.provider.display_name} · ${item.workflow.name}` })) },
              ].filter((group) => group.options.length > 0)}
              placeholder={models.length || readyWorkflowTargets.length ? '选择模型或工作流' : '当前类型没有可用的生成目标'}
              showSearch optionFilterProp="label" value={selectedTargetKey}
            />
          </label>
          <label>
            <span>参数预设</span>
            <Select
              disabled={!model || draft.targetKind === 'workflow'}
              onChange={selectModelPreset}
              options={[
                ...(draft.parametersCustomized ? [{ value: CUSTOM_PARAMETERS, label: `自定义 · 基于 ${modelPresets.find((item) => item.id === draft.modelPresetID)?.name ?? '模型基础参数'}`, disabled: true }] : []),
                ...modelPresets.map((preset) => ({ value: preset.id, label: preset.is_default ? `${preset.name} · 默认` : preset.name })),
                { value: BASE_PARAMETERS, label: '模型基础参数' },
              ]}
              placeholder={draft.targetKind === 'workflow' ? '工作流使用模板默认参数' : '选择模型后可用'}
              value={model && draft.targetKind === 'model' ? parameterPresetValue : undefined}
            />
          </label>
        </div>
        <label className="generation-output-name">
          <span>素材名称（可选）</span>
          <Input maxLength={160} onChange={(event) => updateDraft({ outputName: event.target.value })} placeholder="例如：林默雨巷镜头；多结果会自动添加 01、02…" value={draft.outputName}/>
        </label>
        <MarkdownEditor height={360} onChange={(prompt) => updateDraft({ prompt })} placeholder={`输入${capabilityMeta[capability].label}生成 Prompt…`} value={draft.prompt}/>
        {capability !== 'video' && allowedReferences.length > 0 && <GenerationReferencePicker
          allowedMedia={allowedReferences}
          assets={assetsQuery.data ?? []}
          groups={groupsQuery.data ?? []}
          onChange={(inputReferences) => updateDraft({ inputReferences })}
          onError={onError}
          projectID={project.id}
          requiresPublishedAssets={requiresPublishedAssets}
          value={draft.inputReferences}
        />}
        {capability === 'video' && selectedShot && <div className="generation-canvas-references">
          <div className="generation-canvas-reference-heading"><div><strong>画布参考</strong><span>参考关系来自画布，返回画布修改连线</span></div>{videoNotes.length > 0 && <Button onClick={appendVideoNotes} size="small">将关联备注加入 Prompt</Button>}</div>
          {videoReferences.length ? <div className="generation-canvas-reference-grid">{videoReferences.map((asset) => <div className={unsupportedVideoReferences.some((item) => item.id === asset.id) ? 'unsupported' : ''} key={asset.id}>
            <GenerationAssetPreview asset={asset}/><span>{asset.name}</span>
          </div>)}</div> : <p className="generation-canvas-reference-empty">这个分镜还没有连接参考资产，可以无参考生成。</p>}
          {unsupportedVideoReferences.length > 0 && <p className="generation-reference-warning">当前目标不支持 {unsupportedVideoReferences.map((asset) => asset.name).join('、')} 的媒体类型，请返回画布调整参考连线或更换生成目标。</p>}
          {requiresPublishedAssets && videoReferences.length > 0 && <p className="generation-reference-notice">当前云模型要求这些参考资产已在“资产”页手动发布到 S3。</p>}
          {videoNotes.length > 0 && <div className="generation-linked-notes">{videoNotes.map((note) => <span key={note.id}>{note.data.title}</span>)}</div>}
        </div>}
        {voiceParameterKey && model && <label className="generation-voice-field">
          <span>{model.parameter_schema.properties?.[voiceParameterKey]?.title ?? '音色'}</span>
          <AutoComplete
            onChange={(voiceID) => updateDraft({ parameters: { ...draft.parameters, [voiceParameterKey]: voiceID }, parametersCustomized: true })}
            options={modelVoices.map((voice) => ({ value: voice.voice_id, label: `${voice.name} · ${voice.voice_id}` }))}
            placeholder="输入平台音色 ID，或选择模型页创建的克隆音色"
            value={String(draft.parameters[voiceParameterKey] ?? '')}
          />
          <small>{modelVoices.length ? `当前服务有 ${modelVoices.length} 个已管理音色，也可以直接输入平台内置音色 ID。` : '可以直接输入平台内置音色 ID；支持克隆的服务可先在“模型 → 音色”中创建音色。'}</small>
        </label>}
        {targetDefinition && <div className="generation-parameters">
          <ModelParameterEditor definition={targetDefinition} hiddenKeys={voiceParameterKey ? [voiceParameterKey] : []} onChange={(parameters) => updateDraft({ parameters, parametersCustomized: true })} value={draft.parameters}/>
        </div>}
        <GenerationTaskCenter jobs={taskJobs} onSelect={focusTask} selectedJobID={selectedJobID}/>
        <GenerationProgress capability={capability} job={selectedJob}/>
      </div>

      <div className="generation-result-section">
        <div className="generation-section-heading">
          <div><strong>生成结果</strong></div>
          <div className="generation-view-switch">
            <Button icon={<AppstoreOutlined/>} onClick={() => setViewMode('grid')} type={viewMode === 'grid' ? 'primary' : 'text'}>网格</Button>
            <Button icon={<BarsOutlined/>} onClick={() => setViewMode('list')} type={viewMode === 'list' ? 'primary' : 'text'}>列表</Button>
          </div>
        </div>
        <StagedAssetGallery capability={capability} groups={groupsQuery.data ?? []} onError={onError} projectID={project.id} showProcessedFilter source="generated" viewMode={viewMode}/>
      </div>
    </section>
  </div>
}

function defaultModelPreset(presets: ModelPreset[], model: Model) {
  return presets.find((preset) => preset.model_id === model.id && preset.is_default)
}

function modelVoiceParameter(model: Model) {
  const properties = model.parameter_schema.properties ?? {}
  if (properties.voice_id) return 'voice_id'
  if (properties.voice) return 'voice'
  return undefined
}

function requiresPublishedReferences(model: Model, providers: ModelProvider[]) {
  if (model.features.includes('remote_reference_required')) return true
  const adapterCode = providers.find((provider) => provider.id === model.provider_id)?.adapter_code
  return !adapterCode || !['ollama', 'comfyui', 'siliconflow', 'zhipu', 'tencent_tokenhub', 'moonshot'].includes(adapterCode)
}

function compatibleDraftReferences(references: GenerationReferenceDraft[], inputModalities: MediaType[], publishedOnly: boolean) {
  return references.filter((reference) => inputModalities.includes(reference.mediaType) && (!publishedOnly || reference.source === 'asset'))
}

function discardTemporaryReferences(previous: GenerationReferenceDraft[], retained: GenerationReferenceDraft[]) {
  const retainedKeys = new Set(retained.map((reference) => `${reference.source}:${reference.id}`))
  previous
    .filter((reference) => reference.source === 'upload' && !retainedKeys.has(`${reference.source}:${reference.id}`))
    .forEach((reference) => void api.deleteGenerationReference(reference.id).catch(() => undefined))
}

function workflowTargets(workflows: WorkflowTemplate[], compatibilities: WorkflowCompatibility[], providers: ModelProvider[], capability: GenerationCapability) {
  const providerMap = new Map(providers.filter((provider) => provider.adapter_code === 'comfyui' && provider.enabled).map((provider) => [provider.id, provider]))
  const workflowMap = new Map(workflows.filter((workflow) => workflow.enabled && workflow.capability === capability).map((workflow) => [workflow.id, workflow]))
  return compatibilities
    .filter((item) => item.status === 'ready' && workflowMap.has(item.workflow_template_id) && providerMap.has(item.provider_id))
    .map((item) => ({ workflow: workflowMap.get(item.workflow_template_id)!, provider: providerMap.get(item.provider_id)! }))
}

function canvasReferenceAssets(shot: CanvasNodeDTO, nodes: CanvasNodeDTO[], edges: CanvasEdgeDTO[], assets: Asset[]) {
  const nodeMap = new Map(nodes.map((node) => [node.id, node]))
  const assetMap = new Map(assets.map((asset) => [asset.id, asset]))
  const seen = new Set<string>()
  return edges
    .filter((edge) => edge.target === shot.id && edge.type === 'reference')
    .map((edge) => nodeMap.get(edge.source)?.data.asset_id)
    .filter((id): id is string => !!id && !seen.has(id) && !!seen.add(id))
    .map((id) => assetMap.get(id))
    .filter((asset): asset is Asset => !!asset)
}

function canvasNotes(shot: CanvasNodeDTO, nodes: CanvasNodeDTO[], edges: CanvasEdgeDTO[]) {
  const nodeMap = new Map(nodes.map((node) => [node.id, node]))
  return edges
    .filter((edge) => edge.target === shot.id && edge.type === 'annotation')
    .map((edge) => nodeMap.get(edge.source))
    .filter((node): node is CanvasNodeDTO => node?.data.kind === 'note')
}

function GenerationAssetPreview({ asset }: { asset: Asset }) {
  if (asset.media_type === 'image') return <img alt={asset.name} src={api.assetURL(asset.id)}/>
  if (asset.media_type === 'video') return <video muted preload="metadata" src={api.assetProxyURL(asset.id)}/>
  if (asset.media_type === 'audio') return <AudioOutlined/>
  if (asset.media_type === 'text') return <FileTextOutlined/>
  return <VideoCameraOutlined/>
}

function GenerationTaskCenter({ jobs, selectedJobID, onSelect }: { jobs: GenerationJob[]; selectedJobID?: string; onSelect: (job: GenerationJob) => void }) {
  if (!jobs.length) return null
  const activeCount = jobs.filter((job) => job.status === 'queued' || job.status === 'running').length
  return <div className="generation-task-center">
    <div className="generation-task-heading"><strong>任务中心</strong><span>{activeCount ? `${activeCount} 个进行中` : `${jobs.length} 个本次任务`}</span></div>
    <div className="generation-task-list">
      {jobs.map((job) => <button aria-pressed={selectedJobID === job.id} className={selectedJobID === job.id ? 'active' : ''} key={job.id} onClick={() => onSelect(job)} type="button">
        <i className={job.status}/><span><strong>{job.output_name || `${capabilityMeta[job.capability as GenerationCapability].label}生成`}</strong><small>{generationJobStatus(job)} · {formatJobTime(job.created_at)}</small></span>
      </button>)}
    </div>
  </div>
}

function GenerationProgress({ capability, job }: { capability: GenerationCapability; job?: GenerationJob }) {
  const stages = ['queued', 'requesting', 'generating', 'fetching', 'storing', 'completed']
  const idle = !job
  const current = idle ? -1 : job.status === 'succeeded' ? stages.length : Math.max(0, stages.indexOf(job.stage))
  return <div className={`generation-progress${idle ? ' idle' : ''}${job?.status === 'failed' || job?.status === 'interrupted' ? ' failed' : ''}`}>
    <div><strong>{idle ? '生成进度' : job.status === 'failed' ? '生成失败' : job.status === 'interrupted' ? '生成中断' : job.status === 'succeeded' ? '生成完成' : '正在生成'}</strong><span>{idle ? '提交任务后启用' : `${job.provider_code} / ${job.model_identifier}`}</span></div>
    <Steps
      current={current}
      items={['排队', '提交请求', '模型生成', '获取文件', capability === 'video' ? '写入视频库' : '存入暂存区', '完成'].map((title, index) => ({ title, status: idle ? 'wait' : job.status === 'failed' && index === current ? 'error' : undefined }))}
      size="small"
    />
    {job?.error_message && <small>{job.error_message}</small>}
  </div>
}

function generationJobStatus(job: GenerationJob) {
  if (job.status === 'queued') return '排队中'
  if (job.status === 'running') return '生成中'
  if (job.status === 'succeeded') return '已完成'
  if (job.status === 'failed') return '失败'
  if (job.status === 'interrupted') return '意外中断'
  return '已取消'
}

function formatJobTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '--:--' : date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}
