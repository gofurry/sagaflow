import { useMemo, useState } from 'react'
import { ApiOutlined, AudioOutlined, CheckCircleOutlined, CloudServerOutlined, CopyOutlined, DeleteOutlined, DeploymentUnitOutlined, DownOutlined, EditOutlined, FileTextOutlined, ImportOutlined, KeyOutlined, PlusOutlined, ReloadOutlined, SearchOutlined, SettingOutlined, SyncOutlined, UploadOutlined } from '@ant-design/icons'
import { App, Button, Checkbox, Col, Empty, Form, Input, InputNumber, Modal, Popconfirm, Row, Select, Switch, Upload } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Capability, Model, ModelLifecycleStatus, ModelProvider, ModelSupportStatus, OllamaDiscovery, PromptPreset, SiliconFlowDiscovery } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { JSONCodeEditor } from '../../components/JSONCodeEditor'
import { MarkdownEditor, MarkdownPreview } from '../../components/Markdown'
import { ModelPresetManager } from './ModelPresetManager'
import { VoiceProfileManager } from './VoiceProfileManager'
import { WorkflowTemplateManager } from './WorkflowTemplateManager'

type Section = 'models' | 'prompts' | 'voices'
type ModelView = 'catalog' | 'providers' | 'workflows' | 'credentials'
const capabilities: Capability[] = ['text', 'image', 'audio', 'video', 'multimodal']
const promptCapabilities: PromptPreset['capability'][] = ['text', 'image', 'audio', 'video']
const catalogCapabilities: Capability[] = ['text', 'image', 'audio', 'video']
const ollamaRecommendations = [
  { modelID: 'qwen3.5:4b', name: 'Qwen3.5 4B', summary: '3.4 GB · 视觉理解 · 工具调用 · 256K 上下文', fit: '轻量默认，适合日常开发验证' },
  { modelID: 'qwen3.5:9b', name: 'Qwen3.5 9B', summary: '6.6 GB · 视觉理解 · 工具调用 · 256K 上下文', fit: '质量与显存占用较均衡' },
  { modelID: 'gemma3:4b', name: 'Gemma 3 4B', summary: '3.3 GB · 视觉理解 · 128K 上下文', fit: '轻量多模态备选' },
  { modelID: 'deepseek-r1:8b', name: 'DeepSeek R1 8B', summary: '5.2 GB · 推理 · 128K 上下文', fit: '本地推理与长链路 Prompt 验证' },
  { modelID: 'llama3.2-vision:11b', name: 'Llama 3.2 Vision 11B', summary: '7.8 GB · 视觉理解 · 128K 上下文', fit: '显存允许时的视觉备选' },
] as const

export function ModelHubPage({ onError }: { onError: (error: unknown) => void }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [section, setSection] = useState<Section>('models')
  const [view, setView] = useState<ModelView>('catalog')
  const [providerOpen, setProviderOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<ModelProvider | null>(null)
  const [discoveryProvider, setDiscoveryProvider] = useState<ModelProvider | null>(null)
  const [discovery, setDiscovery] = useState<OllamaDiscovery | SiliconFlowDiscovery | null>(null)
  const [selectedRemoteModels, setSelectedRemoteModels] = useState<string[]>([])
  const [modelOpen, setModelOpen] = useState(false)
  const [credentialOpen, setCredentialOpen] = useState(false)
  const [presetModel, setPresetModel] = useState<Model | null>(null)
  const [editingPrompt, setEditingPrompt] = useState<PromptPreset | null>(null)
  const [promptOpen, setPromptOpen] = useState(false)
  const [expandedModelID, setExpandedModelID] = useState<string>()
  const [expandedProviderID, setExpandedProviderID] = useState<string>()
  const [expandedPromptID, setExpandedPromptID] = useState<string>()
  const [promptSearch, setPromptSearch] = useState('')
  const [promptType, setPromptType] = useState<string>()
  const [promptModelFilter, setPromptModelFilter] = useState<string>()
  const [catalogSearch, setCatalogSearch] = useState('')
  const [catalogProvider, setCatalogProvider] = useState<string>()
  const [catalogCapability, setCatalogCapability] = useState<string>()
  const [catalogStatus, setCatalogStatus] = useState<string>()
  const [catalogSupport, setCatalogSupport] = useState<ModelSupportStatus>()
  const [catalogLifecycle, setCatalogLifecycle] = useState<ModelLifecycleStatus>()
  const [catalogUpdateOpen, setCatalogUpdateOpen] = useState(false)
  const [catalogUpdateJSON, setCatalogUpdateJSON] = useState('')
  const [catalogUpdateFile, setCatalogUpdateFile] = useState('')
  const [providerHealth, setProviderHealth] = useState<Record<string, 'online' | 'offline'>>({})
  const [providerForm] = Form.useForm()
  const [modelForm] = Form.useForm()
  const [credentialForm] = Form.useForm()
  const [promptForm] = Form.useForm()
  const promptCapability = Form.useWatch('capability', promptForm) as PromptPreset['capability'] | undefined
  const promptModelID = Form.useWatch('model_id', promptForm) as string | undefined
  const providerAdapter = Form.useWatch('adapter_code', providerForm) as string | undefined
  const providersQuery = useQuery({ queryKey: ['providers'], queryFn: api.providers })
  const modelsQuery = useQuery({ queryKey: ['models', 'catalog'], queryFn: () => api.models() })
  const catalogUpdateStatusQuery = useQuery({ queryKey: ['model-catalog-update'], queryFn: api.modelCatalogUpdateStatus })
  const credentialsQuery = useQuery({ queryKey: ['credentials'], queryFn: () => api.credentials() })
  const presetsQuery = useQuery({ queryKey: ['presets'], queryFn: () => api.presets() })
  const promptsQuery = useQuery({ queryKey: ['prompt-presets'], queryFn: () => api.promptPresets() })
  const voicesQuery = useQuery({ queryKey: ['voice-profiles'], queryFn: api.voiceProfiles })
  const providers = providersQuery.data ?? []
  const catalogModels = modelsQuery.data ?? []
  const models = catalogModels
  const credentials = credentialsQuery.data ?? []
  const presets = presetsQuery.data ?? []
  const prompts = useMemo(() => promptsQuery.data ?? [], [promptsQuery.data])
  const catalogProviderOptions = useMemo(() => providers
    .filter((provider) => catalogModels.some((model) => model.provider_id === provider.id))
    .map((provider) => ({
      value: provider.id,
      label: `${provider.display_name} · ${catalogModels.filter((model) => model.provider_id === provider.id).length}`,
    })), [catalogModels, providers])
  const filteredModels = useMemo(() => catalogModels.filter((model) => {
    const keyword = catalogSearch.trim().toLocaleLowerCase()
    if (keyword && !`${model.display_name} ${model.model_id} ${model.provider_name}`.toLocaleLowerCase().includes(keyword)) return false
    if (catalogProvider && model.provider_id !== catalogProvider) return false
    if (catalogCapability && model.capability !== catalogCapability) return false
    if (catalogStatus === 'enabled' && (!model.enabled || !model.available)) return false
    if (catalogStatus === 'disabled' && model.enabled) return false
    if (catalogStatus === 'unavailable' && model.available) return false
    if (catalogSupport && modelSupportStatus(model) !== catalogSupport) return false
    if (catalogLifecycle && modelLifecycleStatus(model) !== catalogLifecycle) return false
    return true
  }), [catalogCapability, catalogLifecycle, catalogProvider, catalogSearch, catalogStatus, catalogSupport, catalogModels])
  const filteredPrompts = useMemo(() => prompts.filter((preset) => {
    const keyword = promptSearch.trim().toLocaleLowerCase()
    if (keyword && !`${preset.name} ${preset.description} ${preset.content}`.toLocaleLowerCase().includes(keyword)) return false
    if (promptType && preset.capability !== promptType) return false
    return !promptModelFilter || preset.model_id === promptModelFilter
  }), [promptModelFilter, promptSearch, promptType, prompts])
  const refresh = (...keys: string[][]) => Promise.all(keys.map((key) => queryClient.invalidateQueries({ queryKey: key })))

  const saveProvider = useMutation({
    mutationFn: ({ max_concurrency, ...values }: Record<string, unknown>) => {
      const metadata = { ...(editingProvider?.metadata ?? {}), ...(isLocalAdapter(String(values.adapter_code)) ? { max_concurrency } : {}) }
      const input = { ...values, enabled: editingProvider?.enabled ?? true, metadata }
      return editingProvider ? api.updateProvider(editingProvider.id, input) : api.createProvider(input)
    },
    onSuccess: async () => {
      await refresh(['providers'], ['models'])
      setProviderOpen(false)
      setEditingProvider(null)
      providerForm.resetFields()
      message.success(editingProvider ? '服务连接已更新' : '服务连接已创建')
    },
    onError,
  })
  const removeProvider = useMutation({
    mutationFn: api.deleteProvider,
    onSuccess: async () => { await refresh(['providers'], ['models'], ['credentials'], ['workflow-compatibilities']); message.success('服务连接已删除') },
    onError,
  })
  const toggleProvider = useMutation({ mutationFn: ({ provider, enabled }: { provider: ModelProvider; enabled: boolean }) => api.updateProvider(provider.id, { ...provider, enabled }), onSuccess: () => refresh(['providers']), onError })
  const updateProviderConcurrency = useMutation({
    mutationFn: ({ provider, maxConcurrency }: { provider: ModelProvider; maxConcurrency: number }) => api.updateProvider(provider.id, { ...provider, metadata: { ...provider.metadata, max_concurrency: maxConcurrency } }),
    onSuccess: async () => { await refresh(['providers']); message.success('连接并发设置已更新') },
    onError,
  })
  const createModel = useMutation({ mutationFn: (values: Record<string, unknown>) => api.createModel({ ...values, parameter_schema: parseJSON(values.parameter_schema as string), default_parameters: parseJSON(values.default_parameters as string) }), onSuccess: async () => { await refresh(['models']); setModelOpen(false); modelForm.resetFields(); message.success('模型已加入目录') }, onError })
  const toggleModel = useMutation({ mutationFn: ({ model, enabled }: { model: Model; enabled: boolean }) => api.updateModel(model.id, { ...model, enabled }), onSuccess: () => refresh(['models']), onError })
  const importCatalog = useMutation({
    mutationFn: (manifest: Record<string, unknown>) => api.importModelCatalog(manifest),
    onSuccess: async (result) => {
      await refresh(['models'], ['model-catalog-update'])
      setCatalogUpdateOpen(false)
      message.success(`目录 ${result.overlay.catalog_version} 已更新 · 新增 ${result.sync.created} · 更新 ${result.sync.updated}`)
    },
    onError,
  })
  const createCredential = useMutation({ mutationFn: api.createCredential, onSuccess: async () => { await refresh(['credentials']); setCredentialOpen(false); credentialForm.resetFields(); message.success('凭证已加密保存') }, onError })
  const activate = useMutation({ mutationFn: api.activateCredential, onSuccess: () => refresh(['credentials']), onError })
  const test = useMutation({ mutationFn: api.testCredential, onSuccess: (result) => message.success(result.message), onError })
  const removeCredential = useMutation({ mutationFn: api.deleteCredential, onSuccess: () => refresh(['credentials']), onError })
  const testProvider = useMutation({
    mutationFn: api.testProvider,
    onSuccess: (server, providerID) => {
      setProviderHealth((current) => ({ ...current, [providerID]: 'online' }))
      message.success('node_count' in server
        ? `连接正常 · ComfyUI ${server.version}`
        : 'provider' in server
          ? `连接正常 · 硅基流动当前可见 ${server.model_count} 个生成模型`
          : `连接正常 · Ollama ${server.version} · ${server.model_count} 个模型`)
    },
    onError: (error, providerID) => {
      setProviderHealth((current) => ({ ...current, [providerID]: 'offline' }))
      onError(error)
    },
  })
  const discoverModels = useMutation({
    mutationFn: ({ provider }: { provider: ModelProvider }) => api.discoverProviderModels(provider.id),
    onSuccess: (result, { provider }) => {
      if (!Array.isArray(result.models)) return
      const compatibleResult = result as OllamaDiscovery | SiliconFlowDiscovery
      setDiscoveryProvider(provider)
      setDiscovery(compatibleResult)
      const imported = new Set(models.filter((model) => model.provider_id === provider.id).map((model) => model.model_id))
      setSelectedRemoteModels(compatibleResult.models
        .filter((model) => remoteModelSupported(model) && !imported.has(model.name))
        .map((model) => model.name))
    },
    onError: (error, { provider }) => {
      setProviderHealth((current) => ({ ...current, [provider.id]: 'offline' }))
      onError(error)
    },
  })
  const syncModels = useMutation({
    mutationFn: ({ providerID, modelIDs }: { providerID: string; modelIDs: string[] }) => api.syncProviderModels(providerID, modelIDs),
    onSuccess: async (result) => {
      await refresh(['providers'], ['models'], ['presets'])
      setDiscovery(null)
      setDiscoveryProvider(null)
      message.success(`同步完成 · 新增 ${result.imported} · 更新 ${result.updated}`)
    },
    onError,
  })
  const savePrompt = useMutation({
    mutationFn: (values: { name: string; description?: string; capability: PromptPreset['capability']; model_id?: string; model_preset_id?: string; content: string }) => {
      const input = { ...values, model_id: values.model_id || null, model_preset_id: values.model_preset_id || null }
      return editingPrompt ? api.updatePromptPreset(editingPrompt.id, input) : api.createPromptPreset(input)
    },
    onSuccess: async () => {
      await refresh(['prompt-presets'])
      setPromptOpen(false)
      setEditingPrompt(null)
      promptForm.resetFields()
      message.success('Prompt 预设已保存')
    },
    onError,
  })
  const deletePrompt = useMutation({ mutationFn: api.deletePromptPreset, onSuccess: async () => { await refresh(['prompt-presets']); message.success('Prompt 预设已删除') }, onError })

  const openPrompt = (preset?: PromptPreset) => {
    setEditingPrompt(preset ?? null)
    promptForm.setFieldsValue(preset
      ? { ...preset }
      : { capability: 'text', model_id: undefined, model_preset_id: undefined, content: '', description: '' })
    setPromptOpen(true)
  }
  const openProviderEditor = (provider?: ModelProvider) => {
    setEditingProvider(provider ?? null)
    providerForm.setFieldsValue(provider ? {
      code: provider.code, adapter_code: provider.adapter_code, display_name: provider.display_name,
      base_url: provider.base_url, auth_type: provider.auth_type, capabilities: provider.capabilities,
      max_concurrency: providerMaxConcurrency(provider),
    } : {
      code: undefined, display_name: undefined, adapter_code: 'ollama', auth_type: 'none',
      base_url: 'http://127.0.0.1:11434', capabilities: ['text'], max_concurrency: 1,
    })
    setProviderOpen(true)
  }
  const closeProviderEditor = () => {
    setProviderOpen(false)
    setEditingProvider(null)
    providerForm.resetFields()
  }
  const refreshing = providersQuery.isFetching || modelsQuery.isFetching || credentialsQuery.isFetching || presetsQuery.isFetching || promptsQuery.isFetching
  const refreshCurrent = () => {
    if (section === 'prompts') return refresh(['prompt-presets'], ['models'], ['presets'])
    if (view === 'providers') return refresh(['providers'], ['models'], ['credentials'])
    if (view === 'workflows') return refresh(['workflows'], ['workflow-compatibilities'], ['providers'])
    if (view === 'credentials') return refresh(['credentials'], ['providers'])
    return refresh(['models'], ['presets'])
  }
  const openCurrentCreate = () => {
    if (section === 'prompts') return openPrompt()
    if (view === 'providers') return openProviderEditor()
    if (view === 'credentials') return setCredentialOpen(true)
    setModelOpen(true)
  }
  const createLabel = section === 'prompts' ? '新建 Prompt 预设' : view === 'providers' ? '添加服务连接' : view === 'credentials' ? '添加凭证' : '添加模型'
  const canCreateCurrent = section === 'prompts' || section === 'models'
  const openCatalogUpdate = () => {
    setCatalogUpdateJSON('')
    setCatalogUpdateFile('')
    setCatalogUpdateOpen(true)
  }
  const submitCatalogUpdate = () => {
    try {
      const manifest = JSON.parse(catalogUpdateJSON) as Record<string, unknown>
      importCatalog.mutate(manifest)
    } catch {
      message.error('请输入完整、有效的模型目录 JSON')
    }
  }

  return <div className="page page-models">
    {section !== 'voices' && !(section === 'models' && view === 'workflows') && <FloatingToolbar ariaLabel="模型页工具栏" items={[
      ...(canCreateCurrent ? [{ key: 'create', label: createLabel, icon: <PlusOutlined/>, active: true, onClick: openCurrentCreate }] : []),
      ...(section === 'models' && view === 'catalog' ? [{ key: 'catalog-update', label: '更新模型目录', icon: <ImportOutlined/>, onClick: openCatalogUpdate }] : []),
      { key: 'refresh', label: '刷新当前列表', icon: <ReloadOutlined/>, loading: refreshing, onClick: () => void refreshCurrent() },
    ]}/>}
    <section className="model-page-content">
      <div aria-label="模型功能" className="model-hub-tabs" role="tablist">
        <button aria-selected={section === 'models'} className={section === 'models' ? 'active' : ''} onClick={() => setSection('models')} role="tab" type="button"><ApiOutlined/><span>模型</span><em>{models.length}</em></button>
        <button aria-selected={section === 'prompts'} className={section === 'prompts' ? 'active' : ''} onClick={() => setSection('prompts')} role="tab" type="button"><FileTextOutlined/><span>Prompt 预设</span><em>{prompts.length}</em></button>
        <button aria-selected={section === 'voices'} className={section === 'voices' ? 'active' : ''} onClick={() => setSection('voices')} role="tab" type="button"><AudioOutlined/><span>音色</span><em>{voicesQuery.data?.length ?? 0}</em></button>
      </div>

      {section === 'models' ? <>
        <div aria-label="模型管理分类" className="model-subnav" role="tablist">
          {([
            { value: 'catalog', label: '模型目录', icon: <ApiOutlined/> },
            { value: 'providers', label: '服务连接', icon: <CloudServerOutlined/> },
            { value: 'workflows', label: '工作流', icon: <DeploymentUnitOutlined/> },
            { value: 'credentials', label: '凭证', icon: <KeyOutlined/> },
          ] as const).map((item) => <button aria-selected={view === item.value} className={view === item.value ? 'active' : ''} key={item.value} onClick={() => setView(item.value)} role="tab" type="button">{item.icon}<span>{item.label}</span></button>)}
        </div>
        {view === 'catalog' && <div className="model-catalog-section">
          <div className="model-catalog-filters">
            <Input allowClear onChange={(event) => setCatalogSearch(event.target.value)} placeholder="搜索名称、Model ID 或服务商" prefix={<SearchOutlined/>} value={catalogSearch}/>
            <Select allowClear onChange={setCatalogProvider} options={catalogProviderOptions} placeholder="全部服务商" value={catalogProvider}/>
            <Select allowClear onChange={setCatalogCapability} options={catalogCapabilities.map((value) => ({ value, label: capabilityLabel(value) }))} placeholder="全部类型" value={catalogCapability}/>
            <Select allowClear onChange={setCatalogSupport} options={[
              { value: 'verified', label: '已验证' },
              { value: 'compatible', label: '动态兼容' },
              { value: 'experimental', label: '实验性' },
            ]} placeholder="全部支持级别" value={catalogSupport}/>
            <Select allowClear onChange={setCatalogLifecycle} options={[
              { value: 'active', label: '正常使用' },
              { value: 'deprecated', label: '即将弃用' },
              { value: 'retired', label: '已经下线' },
            ]} placeholder="全部生命周期" value={catalogLifecycle}/>
            <Select allowClear onChange={setCatalogStatus} options={[{ value: 'enabled', label: '已启用' }, { value: 'disabled', label: '已停用' }, { value: 'unavailable', label: '不可用' }]} placeholder="全部状态" value={catalogStatus}/>
            <span>{filteredModels.length} / {models.length} 个模型</span>
          </div>
          <div className="model-flat-list">
          {filteredModels.map((item) => {
            const isExpanded = expandedModelID === item.id
            const modelPresets = presets.filter((preset) => preset.model_id === item.id)
            return <article className={`model-flat-item${isExpanded ? ' expanded' : ''}`} key={item.id}>
              <div className="model-flat-row model-catalog-row">
                <button aria-expanded={isExpanded} aria-label={`${isExpanded ? '收起' : '展开'} ${item.display_name}`} className="model-row-expand" onClick={() => setExpandedModelID(isExpanded ? undefined : item.id)} type="button"><DownOutlined/></button>
                <div className="model-row-primary"><strong>{item.display_name}</strong><code>{item.model_id}</code></div>
                <span className="model-row-provider">{item.provider_name}<em className={`model-support-status ${modelSupportStatus(item)}`}>{modelSupportLabel(modelSupportStatus(item))}</em>{modelLifecycleStatus(item) !== 'active' && <em className={`model-lifecycle-status ${modelLifecycleStatus(item)}`}>{modelLifecycleLabel(modelLifecycleStatus(item))}</em>}</span>
                <span className={`model-capability ${item.capability}`}>{capabilityLabel(item.capability)}</span>
                <label className="model-row-switch"><Switch checked={item.enabled} onChange={(enabled) => toggleModel.mutate({ model: item, enabled })}/><span>{item.enabled ? '已启用' : '已停用'}</span></label>
                <Button icon={<SettingOutlined/>} onClick={() => setPresetModel(item)} size="small">参数预设 {modelPresets.length}</Button>
              </div>
              {isExpanded && <div className="model-flat-detail">
                <ModelDetailGroup empty="跟随模型服务默认值" label="默认参数" values={parameterEntries(item.default_parameters)}/>
                <ModelDetailGroup empty="没有额外可调参数" label="可调参数" values={schemaEntries(item.parameter_schema.properties ?? {})}/>
                <ModelDetailGroup empty="仅 Prompt" label="可接收内容" values={item.input_modalities.map((value) => [mediaTypeLabel(value), value])}/>
                <ModelDetailGroup empty={`${capabilityLabel(item.capability)}生成`} label="原生能力" note="由模型服务报告；“工具调用”尚未接入当前生成流程。" values={item.features.map((value) => [modelFeatureLabel(value), value])}/>
                <ModelDetailGroup empty="未标注" label="支持级别" note={modelSupportNote(modelSupportStatus(item))} values={[[modelSupportLabel(modelSupportStatus(item)), modelSupportStatus(item)]]}/>
                <ModelDetailGroup empty="正常使用" label="生命周期" note={modelLifecycleNote(item)} values={[[modelLifecycleLabel(modelLifecycleStatus(item)), modelLifecycleStatus(item)]]}/>
              </div>}
            </article>
          })}
          {!filteredModels.length && <Empty description={models.length ? '没有符合条件的模型' : '还没有模型'} image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
          </div>
        </div>}

        {view === 'providers' && <div className="model-flat-list">
          {providers.map((provider) => {
            const isExpanded = expandedProviderID === provider.id
            return <article className={`model-flat-item${isExpanded ? ' expanded' : ''}`} key={provider.id}>
              <div className="model-flat-row provider-flat-row">
                <button aria-expanded={isExpanded} aria-label={`${isExpanded ? '收起' : '展开'} ${provider.display_name}`} className="model-row-expand" onClick={() => setExpandedProviderID(isExpanded ? undefined : provider.id)} type="button"><DownOutlined/></button>
                <div className="provider-name"><div className="model-row-primary"><strong>{provider.display_name}</strong><code>{provider.code}</code></div></div>
                <div className="model-capabilities">{provider.capabilities.map((capability) => <span className={`model-capability ${capability}`} key={capability}>{capabilityLabel(capability)}</span>)}</div>
                <span className="model-row-count">{provider.adapter_code === 'comfyui' ? 'ComfyUI · 工作流运行时' : `${provider.adapter_code} · ${catalogModels.filter((model) => model.provider_id === provider.id).length} 个模型`}</span>
                <label className="model-row-switch"><Switch checked={provider.enabled} onChange={(enabled) => toggleProvider.mutate({ provider, enabled })}/><span>{provider.enabled ? '已启用' : '已停用'}</span></label>
                <div className="model-row-actions">
                  <Button icon={<EditOutlined/>} onClick={() => openProviderEditor(provider)} size="small">编辑</Button>
                  <Popconfirm cancelText="取消" description="未被生成历史引用的连接会连同模型目录与凭证一起删除。" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => removeProvider.mutate(provider.id)} title="删除这个服务连接？"><Button danger icon={<DeleteOutlined/>} size="small" type="text"/></Popconfirm>
                </div>
              </div>
              {isExpanded && <div className="model-flat-detail provider-detail">
                <div><span>Base URL</span><code>{provider.base_url || '未配置'}</code></div>
                <div><span>连接 / Adapter</span><code>{provider.code} / {provider.adapter_code}</code></div>
                <div><span>认证方式</span><p>{provider.auth_type === 'none' ? '无需认证' : provider.auth_type}</p></div>
                {(provider.adapter_code === 'ollama' || provider.adapter_code === 'comfyui') && <div className="provider-concurrency-setting"><span>连接最大并发</span><Select
                  disabled={updateProviderConcurrency.isPending}
                  onChange={(maxConcurrency) => updateProviderConcurrency.mutate({ provider, maxConcurrency })}
                  options={Array.from({ length: 8 }, (_, index) => ({ value: index + 1, label: `${index + 1} 个任务` }))}
                  value={providerMaxConcurrency(provider)}
                /></div>}
                {(provider.adapter_code === 'ollama' || provider.adapter_code === 'siliconflow') && <div className="provider-connection-actions"><span>{provider.adapter_code === 'ollama' ? 'Ollama 服务' : '硅基流动服务'}</span><div>
                  <em className={`provider-health ${providerHealth[provider.id] ?? 'unknown'}`}>{providerHealth[provider.id] === 'online' ? '已连接' : providerHealth[provider.id] === 'offline' ? '未检测到服务' : '尚未检测'}</em>
                  <Button icon={<CheckCircleOutlined/>} loading={testProvider.isPending && testProvider.variables === provider.id} onClick={() => testProvider.mutate(provider.id)} size="small">测试连接</Button>
                  <Button icon={<SyncOutlined/>} loading={discoverModels.isPending && discoverModels.variables?.provider.id === provider.id} onClick={() => discoverModels.mutate({ provider })} size="small" type="primary">同步模型</Button>
                </div></div>}
                {provider.adapter_code === 'ollama' && <div className="ollama-recommendations">
                  <span>推荐安装</span>
                  <p>这里只提供经过官方模型库核对的常用标签；安装完成后使用“同步模型”读取真实能力。</p>
                  <div>{ollamaRecommendations.map((recommendation) => {
                    const installed = catalogModels.some((model) => model.provider_id === provider.id && model.model_id === recommendation.modelID)
                    return <article key={recommendation.modelID}>
                      <div><strong>{recommendation.name}</strong><code>{recommendation.modelID}</code></div>
                      <span>{recommendation.summary}</span>
                      <small>{recommendation.fit}</small>
                      {installed
                        ? <em><CheckCircleOutlined/> 已安装</em>
                        : <Button icon={<CopyOutlined/>} onClick={() => void copyText(`ollama pull ${recommendation.modelID}`).then(() => message.success('拉取命令已复制'))} size="small">复制命令</Button>}
                    </article>
                  })}</div>
                </div>}
                {provider.adapter_code === 'comfyui' && <div className="provider-connection-actions"><span>ComfyUI 工作流服务</span><div>
                  <em className={`provider-health ${providerHealth[provider.id] ?? 'unknown'}`}>{providerHealth[provider.id] === 'online' ? '已连接' : providerHealth[provider.id] === 'offline' ? '未检测到服务' : '尚未检测'}</em>
                  <Button icon={<CheckCircleOutlined/>} loading={testProvider.isPending && testProvider.variables === provider.id} onClick={() => testProvider.mutate(provider.id)} size="small" type="primary">测试连接</Button>
                </div></div>}
              </div>}
            </article>
          })}
          {!providers.length && <Empty description="还没有模型服务连接" image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
        </div>}

        {view === 'workflows' && <WorkflowTemplateManager onError={onError} providers={providers}/>}

        {view === 'credentials' && <><div className="credential-scope-notice"><strong>模型服务凭证</strong><span>凭证加密保存在本机数据库中，每个服务连接使用一条当前凭证；不会从配置文件或环境变量读取模型密钥。</span></div><div className="model-flat-list credential-flat-list">
          {credentials.map((item) => <article className="model-flat-item" key={item.id}>
            <div className="model-flat-row credential-flat-row">
              <span className="credential-provider">{item.provider_code}</span>
              <div className="model-row-primary"><strong>{item.name}</strong><code>{item.key_hint}</code></div>
              <div className="model-row-actions">
                <Button loading={test.isPending && test.variables === item.id} onClick={() => test.mutate(item.id)} size="small">测试</Button>
                {!item.is_active && <Button loading={activate.isPending && activate.variables === item.id} onClick={() => activate.mutate(item.id)} size="small">设为当前凭证</Button>}
                <Popconfirm cancelText="取消" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => removeCredential.mutate(item.id)} title="删除这个凭证？"><Button danger icon={<DeleteOutlined/>} size="small" type="text"/></Popconfirm>
              </div>
            </div>
          </article>)}
          {!credentials.length && <Empty description="还没有服务商凭证" image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
        </div></>}
      </> : section === 'prompts' ? <div className="prompt-preset-section">
        <div className="model-filter-bar">
          <Input allowClear onChange={(event) => setPromptSearch(event.target.value)} placeholder="搜索名称或 Prompt 内容" prefix={<SearchOutlined/>} value={promptSearch}/>
          <Select allowClear onChange={setPromptType} options={promptCapabilities.map((value) => ({ value, label: capabilityLabel(value) }))} placeholder="全部类型" value={promptType}/>
          <Select allowClear onChange={setPromptModelFilter} options={models.map((model) => ({ value: model.id, label: model.display_name }))} placeholder="全部模型" showSearch optionFilterProp="label" value={promptModelFilter}/>
          <span>{filteredPrompts.length} 项</span>
        </div>
        <div className="model-flat-list prompt-flat-list">
          {filteredPrompts.map((item) => {
            const isExpanded = expandedPromptID === item.id
            const model = models.find((candidate) => candidate.id === item.model_id)
            const parameterPreset = presets.find((candidate) => candidate.id === item.model_preset_id)
            return <article className={`model-flat-item${isExpanded ? ' expanded' : ''}`} key={item.id}>
              <div className="model-flat-row prompt-flat-row">
                <button aria-expanded={isExpanded} aria-label={`${isExpanded ? '收起' : '展开'} ${item.name}`} className="model-row-expand" onClick={() => setExpandedPromptID(isExpanded ? undefined : item.id)} type="button"><DownOutlined/></button>
                <div className="model-row-primary"><strong>{item.name}</strong><span>{item.description || '暂无说明'}</span></div>
                <span className={`model-capability ${item.capability}`}>{capabilityLabel(item.capability)}</span>
                <div className="prompt-bindings"><span>{model?.display_name ?? '不指定模型'}</span><small>{parameterPreset?.name ?? '跟随模型默认参数'}</small></div>
                <div className="model-row-actions">
                  <Button icon={<EditOutlined/>} onClick={() => openPrompt(item)} size="small">编辑</Button>
                  <Popconfirm cancelText="取消" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => deletePrompt.mutate(item.id)} title="删除这个 Prompt 预设？"><Button danger icon={<DeleteOutlined/>} size="small" type="text"/></Popconfirm>
                </div>
              </div>
              {isExpanded && <div className="prompt-flat-preview"><MarkdownPreview emptyText="这个预设还没有 Prompt 内容" value={item.content}/></div>}
            </article>
          })}
          {!filteredPrompts.length && <Empty description={prompts.length ? '没有符合条件的 Prompt 预设' : '还没有 Prompt 预设'} image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
        </div>
      </div> : <VoiceProfileManager models={models} onError={onError}/>}
    </section>

    <Modal title={editingProvider ? '编辑模型服务连接' : '添加模型服务连接'} open={providerOpen} onCancel={closeProviderEditor} onOk={() => providerForm.submit()} confirmLoading={saveProvider.isPending} width={680}><Form form={providerForm} layout="vertical" onFinish={(values) => saveProvider.mutate(values)} requiredMark={false}><Row gutter={12}><Col span={12}><Form.Item label="连接代码" name="code" rules={[{ required: true }]}><Input placeholder="例如 comfyui-official-local"/></Form.Item></Col><Col span={12}><Form.Item label="显示名称" name="display_name" rules={[{ required: true }]}><Input placeholder="例如 本机官方 ComfyUI"/></Form.Item></Col></Row><Row gutter={12}><Col span={12}><Form.Item label="Adapter" name="adapter_code" rules={[{ required: true }]}><Select onChange={(value) => providerForm.setFieldsValue(value === 'comfyui' ? { base_url: 'http://127.0.0.1:8188', capabilities: ['image', 'video'], max_concurrency: 1, auth_type: 'none' } : value === 'ollama' ? { base_url: 'http://127.0.0.1:11434', capabilities: ['text'], max_concurrency: 1, auth_type: 'none' } : value === 'siliconflow' ? { base_url: 'https://api.siliconflow.cn/v1', capabilities: ['text', 'image', 'audio', 'video', 'multimodal'], auth_type: 'api_key' } : value === 'aliyun_bailian' ? { base_url: 'https://dashscope.aliyuncs.com', capabilities: ['text', 'image', 'audio', 'video'], auth_type: 'api_key' } : {})} options={['ollama', 'comfyui', 'siliconflow', 'openai_chat', 'openai_responses', 'deepseek', 'volcengine', 'minimax', 'aliyun_bailian'].map((value) => ({ value, label: value }))}/></Form.Item></Col><Col span={12}><Form.Item label="认证方式" name="auth_type" rules={[{ required: true }]}><Select options={[{ value: 'none', label: '无需认证' }, { value: 'api_key', label: 'API Key' }, { value: 'bearer', label: 'Bearer Token' }]}/></Form.Item></Col></Row><Row gutter={12}><Col span={isLocalAdapter(providerAdapter) ? 16 : 24}><Form.Item label="Base URL" name="base_url" rules={[{ required: true }, { type: 'url' }]}><Input placeholder="http://127.0.0.1:8188"/></Form.Item></Col>{isLocalAdapter(providerAdapter) && <Col span={8}><Form.Item label="最大并发" name="max_concurrency" rules={[{ required: true }]}><InputNumber min={1} max={8} precision={0} style={{ width: '100%' }}/></Form.Item></Col>}</Row><Form.Item label="输出能力" name="capabilities" rules={[{ required: true }]}><Select mode="multiple" options={capabilities.map((value) => ({ value, label: capabilityLabel(value) }))}/></Form.Item></Form></Modal>
    <Modal title="添加模型" open={modelOpen} onCancel={() => setModelOpen(false)} onOk={() => modelForm.submit()} width={980} confirmLoading={createModel.isPending}><Form form={modelForm} layout="vertical" onFinish={(values) => createModel.mutate(values)} requiredMark={false} initialValues={{ enabled: true, parameter_schema: '{\n  "type": "object",\n  "properties": {}\n}', default_parameters: '{}' }}><Row gutter={12}><Col span={12}><Form.Item label="服务连接" name="provider_id" rules={[{ required: true }]}><Select options={providers.map((item) => ({ value: item.id, label: item.display_name }))}/></Form.Item></Col><Col span={12}><Form.Item label="输出能力" name="capability" rules={[{ required: true }]}><Select options={capabilities.map((value) => ({ value, label: capabilityLabel(value) }))}/></Form.Item></Col></Row><Row gutter={12}><Col span={12}><Form.Item label="Model ID" name="model_id" rules={[{ required: true }]}><Input/></Form.Item></Col><Col span={12}><Form.Item label="显示名称" name="display_name" rules={[{ required: true }]}><Input/></Form.Item></Col></Row><Row gutter={16}><Col span={12}><Form.Item extra="描述生成页如何把参数映射为表单组件。" label="参数 JSON Schema" name="parameter_schema" rules={[jsonRule]}><JSONCodeEditor height={330}/></Form.Item></Col><Col span={12}><Form.Item extra="只写需要由 SagaFlow 主动覆盖的默认值。" label="默认参数" name="default_parameters" rules={[jsonRule]}><JSONCodeEditor height={330}/></Form.Item></Col></Row></Form></Modal>
    <Modal cancelText="取消" confirmLoading={importCatalog.isPending} okButtonProps={{ disabled: !catalogUpdateJSON.trim() }} okText="校验并更新" onCancel={() => setCatalogUpdateOpen(false)} onOk={submitCatalogUpdate} open={catalogUpdateOpen} title="更新模型目录" width={1040}>
      <div className="catalog-update-heading">
        <div>
          <strong>{catalogUpdateStatusQuery.data?.overlay.installed ? `当前更新包 ${catalogUpdateStatusQuery.data.overlay.catalog_version}` : '当前使用二进制内置目录'}</strong>
          <span>导入后立即同步模型；已有启用状态、历史记录和参数预设不会被覆盖。</span>
        </div>
        <div>
          <Upload accept=".json,application/json" beforeUpload={(file) => { void file.text().then((content) => { setCatalogUpdateJSON(content); setCatalogUpdateFile(file.name) }); return false }} maxCount={1} showUploadList={false}>
            <Button icon={<UploadOutlined/>}>选择 JSON 文件</Button>
          </Upload>
          {catalogUpdateStatusQuery.data?.source_url && <Button href={catalogUpdateStatusQuery.data.source_url} rel="noreferrer" target="_blank">打开更新文件</Button>}
        </div>
      </div>
      <div className="catalog-update-editor">
        <span>{catalogUpdateFile || '粘贴完整的 model-catalog.json'}</span>
        <JSONCodeEditor height={500} onChange={setCatalogUpdateJSON} value={catalogUpdateJSON}/>
      </div>
    </Modal>
    <Modal title="添加服务凭证" open={credentialOpen} onCancel={() => setCredentialOpen(false)} onOk={() => credentialForm.submit()} confirmLoading={createCredential.isPending}><Form form={credentialForm} layout="vertical" onFinish={(values) => createCredential.mutate({ ...values, activate: true })} requiredMark={false}><Form.Item label="服务连接" name="provider_id" rules={[{ required: true }]}><Select options={providers.filter((item) => item.auth_type !== 'none').map((item) => ({ value: item.id, label: item.display_name }))}/></Form.Item><Form.Item label="名称" name="name" initialValue="Primary credential" rules={[{ required: true }]}><Input/></Form.Item><Form.Item label="API Key / Token" name="api_key" rules={[{ required: true }]}><Input.Password autoComplete="new-password"/></Form.Item></Form></Modal>
    <Modal cancelText="取消" confirmLoading={syncModels.isPending} okButtonProps={{ disabled: !discoveryProvider }} okText="同步所选模型" onCancel={() => { setDiscovery(null); setDiscoveryProvider(null) }} onOk={() => discoveryProvider && syncModels.mutate({ providerID: discoveryProvider.id, modelIDs: selectedRemoteModels })} open={!!discovery && !!discoveryProvider} title={discoveryProvider ? `同步 ${discoveryProvider.display_name} 中的模型` : '同步模型'} width={760}>
      {discovery && discoveryProvider && <div className="ollama-discovery">
        {isSiliconFlowDiscovery(discovery)
          ? <div className="ollama-server-summary"><span>硅基流动在线目录</span><span>{discovery.server.model_count} 个生成模型</span><span>动态兼容模型可按需导入</span></div>
          : <div className="ollama-server-summary"><span>Ollama {discovery.server.version}</span><span>{discovery.server.model_count} 个已安装</span><span>{discovery.server.running_count} 个已加载</span></div>}
        <Checkbox.Group onChange={(values) => setSelectedRemoteModels(values as string[])} value={selectedRemoteModels}>
          {discovery.models.map((remote) => {
            const imported = models.some((model) => model.provider_id === discoveryProvider.id && model.model_id === remote.name)
            const supported = remoteModelSupported(remote)
            return <label className={`ollama-model-option${supported ? '' : ' unsupported'}`} key={remote.name}>
              <Checkbox disabled={imported || !supported} value={remote.name}/>
              <div><strong>{remote.name}</strong><span>{remoteModelSummary(remote)}</span><small>{remoteModelCapabilities(remote)}{imported ? ' · 已导入，将更新元数据' : !supported ? ' · 当前阶段不支持这种输出类型' : ''}</small></div>
            </label>
          })}
        </Checkbox.Group>
      </div>}
    </Modal>
    <ModelPresetManager model={presetModel} onClose={() => setPresetModel(null)} onError={onError} presets={presets}/>
    <Modal cancelText="取消" confirmLoading={savePrompt.isPending} okText="保存预设" onCancel={() => setPromptOpen(false)} onOk={() => promptForm.submit()} open={promptOpen} title={editingPrompt ? '编辑 Prompt 预设' : '新建 Prompt 预设'} width={1040}>
      <Form form={promptForm} layout="vertical" onFinish={(values) => savePrompt.mutate(values)} requiredMark={false}>
        <Row gutter={14}>
          <Col span={10}><Form.Item label="名称" name="name" rules={[{ required: true, whitespace: true }]}><Input autoFocus placeholder="例如：人物设定图"/></Form.Item></Col>
          <Col span={7}><Form.Item label="资源类型" name="capability" rules={[{ required: true }]}><Select onChange={() => promptForm.setFieldsValue({ model_id: undefined, model_preset_id: undefined })} options={promptCapabilities.map((value) => ({ value, label: capabilityLabel(value) }))}/></Form.Item></Col>
          <Col span={7}><Form.Item label="默认模型" name="model_id"><Select allowClear onChange={() => promptForm.setFieldValue('model_preset_id', undefined)} options={models.filter((model) => model.capability === promptCapability).map((model) => ({ value: model.id, label: model.display_name }))} placeholder="不指定"/></Form.Item></Col>
        </Row>
        <Form.Item extra="可选。选择 Prompt 预设时会推荐这套参数，仍可在生成页切换或自定义。" label="推荐参数预设" name="model_preset_id">
          <Select allowClear disabled={!promptModelID} options={presets.filter((preset) => preset.model_id === promptModelID).map((preset) => ({ value: preset.id, label: preset.is_default ? `${preset.name} · 默认` : preset.name }))} placeholder={promptModelID ? '跟随模型默认参数预设' : '请先选择默认模型'}/>
        </Form.Item>
        <Form.Item label="说明" name="description"><Input placeholder="说明这个预设适合生成什么内容"/></Form.Item>
        <Form.Item label="Prompt" name="content" rules={[{ required: true, whitespace: true }]}><MarkdownFormEditor/></Form.Item>
      </Form>
    </Modal>
  </div>
}

function MarkdownFormEditor({ value, onChange }: { value?: string; onChange?: (value: string) => void }) {
  return <MarkdownEditor height={360} onChange={onChange} placeholder="使用 Markdown 编写可复用的 Prompt…" value={value}/>
}

function ModelDetailGroup({ empty, label, note, values }: { empty: string; label: string; note?: string; values: Array<[string, string]> }) {
  return <div className="model-detail-group">
    <span>{label}</span>
    {values.length ? <div className="model-detail-values">{values.map(([title, raw]) => <span className="model-detail-value" key={`${raw}:${title}`}><strong>{title}</strong>{raw !== title && <code>{raw}</code>}</span>)}</div> : <p>{empty}</p>}
    {note && <small>{note}</small>}
  </div>
}

const jsonRule = { validator: (_: unknown, value: string) => { try { JSON.parse(value || '{}'); return Promise.resolve() } catch { return Promise.reject(new Error('请输入有效 JSON')) } } }
function parseJSON(value: string) { try { return JSON.parse(value || '{}') as Record<string, unknown> } catch { return {} } }
function parameterEntries(parameters: Record<string, unknown>): Array<[string, string]> { return Object.entries(parameters).map(([key, value]) => [key, typeof value === 'object' ? JSON.stringify(value) : String(value)]) }
function schemaEntries(properties: NonNullable<Model['parameter_schema']['properties']>): Array<[string, string]> { return Object.entries(properties).map(([key, schema]) => [schema.title || key, key]) }
function capabilityLabel(value: string) { return ({ text: '文本', image: '图像', audio: '音频', video: '视频', multimodal: '多模态' } as Record<string, string>)[value] ?? value }
function mediaTypeLabel(value: string) { return ({ text: '文本', image: '图像', audio: '音频', video: '视频', file: '文件' } as Record<string, string>)[value] ?? value }
function modelFeatureLabel(value: string) { return ({
  thinking: '深度思考', reasoning: '推理', tools: '工具调用（待接入）', vision: '视觉理解', completion: '基础生成',
  coding: '代码能力', agent: 'Agent 任务', rolling_release: '持续更新', responses_api: 'Responses API', roleplay: '角色扮演',
  dialogue: '对白生成', long_context: '长上下文', fast: '高速生成', image_generation: '图像生成', image_edit: '图像编辑',
  multi_reference: '多参考', sequential_images: '组图生成', knowledge_grounding: '知识增强', video_generation: '视频生成',
  audio_generation: '同步声音', speech_generation: '语音生成', speech_recognition: '语音识别', voice_clone: '音色克隆',
  dynamic_voice: '动态音色', two_speaker_dialogue: '双人对话', long_audio: '长音频', text_to_video: '文生视频',
  image_to_video: '图生视频', structured_output: '结构化输出', chinese_text: '中文文字',
} as Record<string, string>)[value] ?? value }
function isLocalAdapter(value?: string) { return value === 'ollama' || value === 'comfyui' }
function providerMaxConcurrency(provider: ModelProvider) {
  const value = Number(provider.metadata.max_concurrency)
  return Number.isInteger(value) && value >= 1 && value <= 8 ? value : 1
}
function modelDetailSummary(model: OllamaDiscovery['models'][number]) {
  const details = model.details as { parameter_size?: string; quantization_level?: string }
  const parts = [details.parameter_size, details.quantization_level, model.context_length ? `${model.context_length.toLocaleString()} 上下文` : undefined]
  return parts.filter(Boolean).join(' · ') || `${(model.size / 1024 / 1024 / 1024).toFixed(1)} GB`
}
function modelSupportStatus(model: Model): ModelSupportStatus {
  const status = model.metadata.support_status
  return status === 'verified' || status === 'experimental' ? status : 'compatible'
}
function modelSupportLabel(status: ModelSupportStatus) {
  return ({ verified: '已验证', compatible: '动态兼容', experimental: '实验性' } as const)[status]
}
function modelSupportNote(status: ModelSupportStatus) {
  return ({
    verified: '已由当前 SagaFlow 版本完成真实请求验证。',
    compatible: '参数来自兼容协议或在线发现；平台变更时可通过目录更新包修正，无需升级二进制。',
    experimental: '已识别到模型，但能力或参数尚未完整验证，建议先做低成本调用。',
  } as const)[status]
}
function modelLifecycleStatus(model: Model): ModelLifecycleStatus {
  const status = model.metadata.lifecycle_status
  return status === 'deprecated' || status === 'retired' ? status : 'active'
}
function modelLifecycleLabel(status: ModelLifecycleStatus) {
  return ({ active: '正常使用', deprecated: '即将弃用', retired: '已经下线' } as const)[status]
}
function modelLifecycleNote(model: Model) {
  const status = modelLifecycleStatus(model)
  const parts = [
    status === 'active' ? '平台仍在正常提供该模型。' : status === 'deprecated' ? '模型仍可调用，但不建议用于新项目。' : '模型已停止用于新任务，历史记录和参数预设会继续保留。',
    typeof model.metadata.lifecycle_message === 'string' ? model.metadata.lifecycle_message : '',
    typeof model.metadata.sunset_at === 'string' && model.metadata.sunset_at ? `预计下线：${new Date(model.metadata.sunset_at).toLocaleDateString()}` : '',
    replacementLabel(model),
  ]
  return parts.filter(Boolean).join(' ')
}
function replacementLabel(model: Model) {
  const replacement = model.metadata.replacement
  if (!replacement || typeof replacement !== 'object') return ''
  const value = replacement as Record<string, unknown>
  return typeof value.model_id === 'string' && value.model_id ? `建议迁移到 ${value.model_id}。` : ''
}
function isSiliconFlowDiscovery(discovery: OllamaDiscovery | SiliconFlowDiscovery): discovery is SiliconFlowDiscovery {
  return 'provider' in discovery.server
}
function remoteModelSupported(model: OllamaDiscovery['models'][number] | SiliconFlowDiscovery['models'][number]) {
  return 'supports_generation' in model ? model.supports_generation : model.supports_text_output
}
function remoteModelSummary(model: OllamaDiscovery['models'][number] | SiliconFlowDiscovery['models'][number]) {
  return 'supports_generation' in model
    ? `${capabilityLabel(model.capability)} · ${modelSupportLabel(model.support_status)} · ${model.task}`
    : modelDetailSummary(model)
}
function remoteModelCapabilities(model: OllamaDiscovery['models'][number] | SiliconFlowDiscovery['models'][number]) {
  return ('supports_generation' in model ? model.features : model.capabilities).map(modelFeatureLabel).join(' · ')
}
async function copyText(value: string) {
  await navigator.clipboard.writeText(value)
}
