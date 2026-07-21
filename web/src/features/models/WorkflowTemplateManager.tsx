import { useMemo, useState } from 'react'
import { DeleteOutlined, DownOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SafetyCertificateOutlined, UploadOutlined } from '@ant-design/icons'
import { Alert, App, Button, Col, Empty, Form, Input, Modal, Popconfirm, Row, Select, Switch, Tabs, Upload } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Capability, ModelProvider, WorkflowAnalysis, WorkflowCompatibility, WorkflowTemplate } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { JSONCodeEditor } from '../../components/JSONCodeEditor'

const capabilities: Array<Exclude<Capability, 'multimodal'>> = ['text', 'image', 'audio', 'video']

export function WorkflowTemplateManager({ onError, providers }: { onError: (error: unknown) => void; providers: ModelProvider[] }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<WorkflowTemplate | null>(null)
  const [importStep, setImportStep] = useState<0 | 1>(0)
  const [analysis, setAnalysis] = useState<WorkflowAnalysis | null>(null)
  const [analysisProviderID, setAnalysisProviderID] = useState<string>()
  const [expandedID, setExpandedID] = useState<string>()
  const [connectionByWorkflow, setConnectionByWorkflow] = useState<Record<string, string>>({})
  const workflowsQuery = useQuery({ queryKey: ['workflows'], queryFn: () => api.workflows() })
  const compatibilityQuery = useQuery({ queryKey: ['workflow-compatibilities'], queryFn: () => api.workflowCompatibilities() })
  const workflows = workflowsQuery.data ?? []
  const compatibilities = compatibilityQuery.data ?? []
  const connections = useMemo(() => providers.filter((item) => item.adapter_code === 'comfyui' && item.enabled), [providers])
  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['workflows'] }),
    queryClient.invalidateQueries({ queryKey: ['workflow-compatibilities'] }),
    queryClient.invalidateQueries({ queryKey: ['providers'] }),
  ])
  const save = useMutation({
    mutationFn: (values: Record<string, unknown>) => {
      const input = normalizeWorkflowForm(values)
      return editing ? api.updateWorkflow(editing.id, input) : api.createWorkflow(input)
    },
    onSuccess: async () => {
      await refresh()
      setOpen(false)
      setEditing(null)
      setAnalysis(null)
      setImportStep(0)
      form.resetFields()
      message.success('工作流模板已保存，请检查目标连接兼容性')
    },
    onError,
  })
  const analyze = useMutation({
    mutationFn: async () => {
      await form.validateFields(['workflow'])
      return api.analyzeWorkflow({ workflow: parseJSONObject(form.getFieldValue('workflow')), provider_id: analysisProviderID })
    },
    onSuccess: (result) => {
      setAnalysis(result)
      form.setFieldsValue({
        workflow: pretty(result.workflow), capability: result.capability, input_modalities: result.input_modalities,
        parameter_schema: pretty(result.parameter_schema), default_parameters: pretty(result.default_parameters),
        bindings: pretty(result.bindings), outputs: pretty(result.outputs), requirements: pretty(result.requirements),
      })
      setImportStep(1)
      const warnings = result.issues.filter((item) => item.level === 'warning').length
      if (warnings) message.warning(`分析完成，有 ${warnings} 项需要确认`)
      else message.success('分析完成，已生成工作流配置建议')
    },
    onError,
  })
  const toggle = useMutation({
    mutationFn: ({ workflow, enabled }: { workflow: WorkflowTemplate; enabled: boolean }) => api.updateWorkflow(workflow.id, { enabled }),
    onSuccess: refresh,
    onError,
  })
  const remove = useMutation({ mutationFn: api.deleteWorkflow, onSuccess: async () => { await refresh(); message.success('工作流模板已删除') }, onError })
  const check = useMutation({
    mutationFn: ({ workflowID, providerID }: { workflowID: string; providerID: string }) => api.checkWorkflow(workflowID, providerID),
    onSuccess: async (result) => {
      await refresh()
      if (result.status === 'ready') message.success('依赖齐全，可以生成')
      else message.warning(compatibilitySummary(result))
    },
    onError,
  })
  const openEditor = (workflow?: WorkflowTemplate) => {
    setEditing(workflow ?? null)
    setAnalysis(null)
    setImportStep(workflow ? 1 : 0)
    setAnalysisProviderID(undefined)
    form.setFieldsValue(workflow ? {
      ...workflow,
      workflow: pretty(workflow.workflow), parameter_schema: pretty(workflow.parameter_schema),
      default_parameters: pretty(workflow.default_parameters), bindings: pretty(workflow.bindings),
      outputs: pretty(workflow.outputs), requirements: pretty(workflow.requirements),
    } : {
      capability: 'image', input_modalities: [], enabled: true,
      workflow: '{}', parameter_schema: '{\n  "type": "object",\n  "properties": {}\n}',
      default_parameters: '{}', bindings: '{}', outputs: '[]', requirements: '{\n  "nodes": [],\n  "models": []\n}',
    })
    setOpen(true)
  }
  const closeEditor = () => {
    setOpen(false)
    setEditing(null)
    setAnalysis(null)
    setImportStep(0)
    form.resetFields()
  }
  const importJSONFile = async (file: File) => {
    try {
      const parsed = JSON.parse(await file.text()) as unknown
      const basename = file.name.replace(/\.json$/i, '').trim()
      form.setFieldsValue({
        workflow: pretty(parsed),
        code: form.getFieldValue('code') || workflowCode(basename),
        name: form.getFieldValue('name') || basename,
      })
      setAnalysis(null)
      message.success('已读取工作流文件，请开始分析')
    } catch {
      message.error('无法读取这个 JSON 文件')
    }
  }

  return <div className="workflow-template-manager">
    <FloatingToolbar ariaLabel="工作流工具栏" items={[
      { key: 'create', label: '导入工作流', icon: <PlusOutlined/>, active: true, onClick: () => openEditor() },
      { key: 'refresh', label: '刷新工作流', icon: <ReloadOutlined/>, loading: workflowsQuery.isFetching || compatibilityQuery.isFetching, onClick: () => void refresh() },
    ]}/>
    <div className="model-flat-list workflow-flat-list">
      {workflows.map((workflow) => {
        const expanded = workflow.id === expandedID
        const records = compatibilities.filter((item) => item.workflow_template_id === workflow.id)
        const selectedProviderID = connectionByWorkflow[workflow.id] ?? connections[0]?.id
        const selectedRecord = records.find((item) => item.provider_id === selectedProviderID)
        return <article className={`model-flat-item${expanded ? ' expanded' : ''}`} key={workflow.id}>
          <div className="model-flat-row workflow-flat-row">
            <button aria-expanded={expanded} className="model-row-expand" onClick={() => setExpandedID(expanded ? undefined : workflow.id)} type="button"><DownOutlined/></button>
            <div className="model-row-primary"><strong>{workflow.name}</strong><code>{workflow.code}@{workflow.version}</code></div>
            <span className={`model-capability ${workflow.capability}`}>{capabilityLabel(workflow.capability)}</span>
            <div className="workflow-connection-check">
              <Select
                onChange={(providerID) => setConnectionByWorkflow((current) => ({ ...current, [workflow.id]: providerID }))}
                options={connections.map((provider) => ({ value: provider.id, label: provider.display_name }))}
                placeholder={connections.length ? '选择 ComfyUI 连接' : '还没有 ComfyUI 连接'}
                value={selectedProviderID}
              />
              <Button disabled={!selectedProviderID} icon={<SafetyCertificateOutlined/>} loading={check.isPending && check.variables?.workflowID === workflow.id} onClick={() => selectedProviderID && check.mutate({ workflowID: workflow.id, providerID: selectedProviderID })} size="small">检查所选连接</Button>
            </div>
            <label className="model-row-switch"><Switch checked={workflow.enabled} onChange={(enabled) => toggle.mutate({ workflow, enabled })}/><span>{workflow.enabled ? '已启用' : '已停用'}</span></label>
            <div className="model-row-actions">
              <Button icon={<EditOutlined/>} onClick={() => openEditor(workflow)} size="small">编辑</Button>
              <Popconfirm cancelText="取消" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => remove.mutate(workflow.id)} title="删除这个工作流模板？"><Button danger icon={<DeleteOutlined/>} size="small" type="text"/></Popconfirm>
            </div>
          </div>
          {expanded && <div className="workflow-flat-detail">
            <p>{workflow.description || '暂无工作流说明'}</p>
            <div className="workflow-detail-grid">
              <div><span>参考输入</span><strong>{workflow.input_modalities.join(' · ') || '不接收参考'}</strong></div>
              <div><span>动态参数</span><strong>{Object.keys(workflow.parameter_schema.properties ?? {}).join(' · ') || '没有可调参数'}</strong></div>
              <div><span>绑定项</span><strong>{Object.keys(workflow.bindings).join(' · ') || '没有绑定项'}</strong></div>
              <div><span>输出</span><strong>{workflow.outputs.length} 项</strong></div>
            </div>
            <div className={`workflow-selected-compatibility${selectedRecord?.status === 'ready' ? ' ready' : ''}`}>
              <div><span>所选连接的上次检查</span><strong>{selectedRecord ? compatibilitySummary(selectedRecord) : '尚未检查'}</strong></div>
              <small>{selectedRecord ? formatCheckedAt(selectedRecord.checked_at) : '检查结果按“工作流 + 服务连接”分别保存，不代表多个 ComfyUI 同时执行。'}</small>
            </div>
          </div>}
        </article>
      })}
      {!workflows.length && <Empty description="还没有 ComfyUI 工作流模板" image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
    </div>

    <Modal
      centered
      className="workflow-template-modal"
      footer={importStep === 0 ? [
        <Button key="cancel" onClick={closeEditor}>取消</Button>,
        <Button key="analyze" loading={analyze.isPending} onClick={() => analyze.mutate()} type="primary">分析工作流</Button>,
      ] : [
        <Button key="cancel" onClick={closeEditor}>取消</Button>,
        !editing && <Button key="back" onClick={() => setImportStep(0)}>返回修改</Button>,
        <Button key="save" loading={save.isPending} onClick={() => form.submit()} type="primary">保存工作流</Button>,
      ]}
      onCancel={closeEditor}
      open={open}
      title={editing ? '编辑工作流模板' : importStep === 0 ? '导入 ComfyUI 工作流' : '确认工作流配置'}
      width={1120}
    >
      <Form form={form} layout="vertical" onFinish={(values) => save.mutate(values)} requiredMark={false}>
        {importStep === 0 ? <div className="workflow-import-source">
          <div className="workflow-import-intro">
            <div><strong>导入 API 格式工作流</strong><p>助手会识别 Prompt、常用参数、参考图、输出节点以及模型依赖。分析结果保存前都可以修改。</p></div>
            <Upload accept=".json,application/json" beforeUpload={(file) => { void importJSONFile(file); return false }} maxCount={1} showUploadList={false}>
              <Button icon={<UploadOutlined/>}>选择 JSON 文件</Button>
            </Upload>
          </div>
          <label className="workflow-analysis-provider">
            <span>分析依据连接（可选）</span>
            <Select
              allowClear
              onChange={setAnalysisProviderID}
              options={connections.map((provider) => ({ value: provider.id, label: provider.display_name }))}
              placeholder="离线分析，不检查本机节点和模型"
              value={analysisProviderID}
            />
            <small>选择连接后会同时检查节点和模型；不选择也可以完成结构分析。</small>
          </label>
          <JSONField extra="请在 ComfyUI 中使用“保存（API 格式）”，也可以粘贴 /prompt 请求中的 prompt 对象。" label="API 工作流 JSON" name="workflow" rows={23}/>
        </div> : <div className="workflow-import-review">
          <div className="workflow-import-review-head">
            <div><strong>{analysis ? '自动分析结果' : '工作流配置'}</strong><p>{analysis ? '下面是助手生成的建议，请确认警告项和最终输出。' : '当前为已经保存的配置；重新分析会用工作流图重新生成高级配置。'}</p></div>
            <Popconfirm
              cancelText="取消"
              description="参数、绑定、输出和依赖配置会被新的分析结果覆盖。"
              okText="重新分析"
              onConfirm={() => analyze.mutate()}
              title="重新分析当前工作流？"
            >
              <Button loading={analyze.isPending} icon={<ReloadOutlined/>}>重新分析</Button>
            </Popconfirm>
          </div>
          {analysis && <>
            <div className="workflow-analysis-summary">
              <AnalysisStat label="节点" value={analysis.summary.node_count}/>
              <AnalysisStat label="参数" value={analysis.summary.parameter_count}/>
              <AnalysisStat label="绑定" value={analysis.summary.binding_count}/>
              <AnalysisStat label="参考输入" value={analysis.summary.input_count}/>
              <AnalysisStat label="输出" value={analysis.summary.output_count}/>
              <AnalysisStat label="模型依赖" value={analysis.summary.model_count}/>
            </div>
            <div className="workflow-analysis-issues">
              {!analysis.issues.length && <Alert title="没有发现需要人工确认的问题" showIcon type="success"/>}
              {analysis.issues.map((issue, index) => <Alert key={`${issue.code}-${index}`} title={issue.message} showIcon type={issue.level === 'warning' ? 'warning' : 'info'}/>)}
            </div>
          </>}
          <Tabs
            className="workflow-editor-tabs"
            items={[
              { key: 'basic', label: '基础信息', forceRender: true, children: <div className="workflow-basic-fields">
                <Row gutter={12}>
                  <Col span={6}><Form.Item label="模板代码" name="code" rules={[{ required: true, whitespace: true }]}><Input placeholder="anything-v5-basic"/></Form.Item></Col>
                  <Col span={8}><Form.Item label="名称" name="name" rules={[{ required: true, whitespace: true }]}><Input placeholder="Anything V5 基础出图"/></Form.Item></Col>
                  <Col span={5}><Form.Item label="输出类型" name="capability" rules={[{ required: true }]}><Select options={capabilities.map((value) => ({ value, label: capabilityLabel(value) }))}/></Form.Item></Col>
                  <Col span={5}><Form.Item label="状态" name="enabled" valuePropName="checked"><Switch checkedChildren="启用" unCheckedChildren="停用"/></Form.Item></Col>
                </Row>
                <Form.Item label="说明" name="description"><Input placeholder="说明用途、模型和适用范围"/></Form.Item>
                <Form.Item extra="当前 Adapter 对参考资源使用 ComfyUI /upload/image，因此只开放图像参考。" label="参考输入" name="input_modalities"><Select mode="multiple" options={[{ value: 'image', label: '图像' }]}/></Form.Item>
              </div> },
              { key: 'advanced', label: '高级配置', forceRender: true, children: <div className="workflow-advanced-panel">
                <div className="workflow-advanced-panel-head"><strong>JSON 配置</strong><span>自动生成后仍可手工修正</span></div>
                <Tabs
                  className="workflow-json-tabs"
                  items={[
                    { key: 'workflow', label: '工作流', forceRender: true, children: <JSONField name="workflow" height="min(48vh, 560px)"/> },
                    { key: 'schema', label: '参数 Schema', forceRender: true, children: <JSONField name="parameter_schema" height="min(48vh, 560px)"/> },
                    { key: 'bindings', label: '绑定关系', forceRender: true, children: <JSONField name="bindings" height="min(48vh, 560px)"/> },
                    { key: 'outputs', label: '输出选择器', forceRender: true, children: <JSONField name="outputs" height="min(48vh, 560px)"/> },
                    { key: 'defaults', label: '默认参数', forceRender: true, children: <JSONField name="default_parameters" height="min(48vh, 560px)"/> },
                    { key: 'requirements', label: '节点与模型依赖', forceRender: true, children: <JSONField name="requirements" height="min(48vh, 560px)"/> },
                  ]}
                />
              </div> },
            ]}
          />
        </div>}
      </Form>
    </Modal>
  </div>
}

function AnalysisStat({ label, value }: { label: string; value: number }) {
  return <div><strong>{String(value).padStart(2, '0')}</strong><span>{label}</span></div>
}

function JSONField({ extra, height, label, name, rows }: { extra?: string; height?: number | string; label?: string; name: string; rows?: number }) {
  const editorHeight = height ?? Math.max(190, Math.min(520, (rows ?? 14) * 21))
  return <Form.Item extra={extra} label={label} name={name} rules={[jsonRule]}><JSONCodeEditor height={editorHeight}/></Form.Item>
}

function normalizeWorkflowForm(values: Record<string, unknown>) {
  return {
    ...values,
    workflow: parseJSON(values.workflow), parameter_schema: parseJSON(values.parameter_schema),
    default_parameters: parseJSON(values.default_parameters), bindings: parseJSON(values.bindings),
    outputs: parseJSON(values.outputs), requirements: parseJSON(values.requirements),
  } as Partial<WorkflowTemplate>
}

const jsonRule = { validator: (_: unknown, value: string) => { try { JSON.parse(value || '{}'); return Promise.resolve() } catch { return Promise.reject(new Error('请输入有效 JSON')) } } }
function parseJSON(value: unknown) { try { return JSON.parse(String(value ?? '{}')) as unknown } catch { return {} } }
function parseJSONObject(value: unknown) {
  const parsed = JSON.parse(String(value ?? '{}')) as unknown
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error('工作流必须是 JSON 对象')
  return parsed as Record<string, unknown>
}
function pretty(value: unknown) { return JSON.stringify(value ?? {}, null, 2) }
function workflowCode(value: string) {
  const code = value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
  return code || `comfy-workflow-${Date.now().toString(36)}`
}
function capabilityLabel(value: string) { return ({ text: '文本', image: '图像', audio: '音频', video: '视频' } as Record<string, string>)[value] ?? value }
function compatibilitySummary(record: WorkflowCompatibility) {
  if (record.status === 'ready') return '依赖齐全'
  if (record.status === 'missing_nodes') return `缺少节点：${record.report.missing_nodes?.join('、') || '未知'}`
  if (record.status === 'missing_resources') return `缺少模型：${record.report.missing_resources?.map((item) => item.name).join('、') || '未知'}`
  return '连接不兼容'
}
function formatCheckedAt(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '历史检查结果' : `检查于 ${date.toLocaleString('zh-CN', { hour12: false })}`
}
