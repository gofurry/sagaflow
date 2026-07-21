import { useMemo, useState } from 'react'
import { CheckCircleOutlined, CopyOutlined, DeleteOutlined, EditOutlined, PlusOutlined, StarOutlined } from '@ant-design/icons'
import { App, Button, Checkbox, Empty, Input, Modal, Popconfirm } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Model, ModelPreset } from '../../api/types'
import { ModelParameterEditor } from '../../components/ModelParameterEditor'

interface PresetDraft {
  id?: string
  name: string
  parameters: Record<string, unknown>
  isDefault: boolean
}

export function ModelPresetManager({ model, presets, onClose, onError }: { model: Model | null; presets: ModelPreset[]; onClose: () => void; onError: (error: unknown) => void }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [draft, setDraft] = useState<PresetDraft | null>(null)
  const modelPresets = useMemo(() => model ? presets.filter((item) => item.model_id === model.id) : [], [model, presets])
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['presets'] })
  const save = useMutation({
    mutationFn: async (value: PresetDraft) => {
      const input = { model_id: model!.id, name: value.name, parameters: value.parameters, is_default: value.isDefault }
      return value.id ? api.updatePreset(value.id, input) : api.createPreset(input)
    },
    onSuccess: async () => {
      await refresh()
      setDraft(null)
      message.success('参数预设已保存')
    },
    onError,
  })
  const remove = useMutation({
    mutationFn: api.deletePreset,
    onSuccess: async () => {
      await refresh()
      setDraft(null)
      message.success('参数预设已删除')
    },
    onError,
  })
  const setDefault = useMutation({
    mutationFn: (preset: ModelPreset) => api.updatePreset(preset.id, { ...preset, is_default: true }),
    onSuccess: async () => {
      await refresh()
      message.success('默认参数预设已更新')
    },
    onError,
  })

  const startCreate = () => setDraft({ name: '', parameters: model?.default_parameters ?? {}, isDefault: modelPresets.length === 0 })
  const startEdit = (preset: ModelPreset) => setDraft({ id: preset.id, name: preset.name, parameters: preset.parameters, isDefault: preset.is_default })
  const startCopy = (preset: ModelPreset) => setDraft({ name: `${preset.name} 副本`, parameters: structuredClone(preset.parameters), isDefault: false })
  const close = () => {
    setDraft(null)
    onClose()
  }

  return <Modal cancelText="关闭" footer={null} onCancel={close} open={!!model} title={`${model?.display_name ?? ''} · 参数预设`} width={980}>
    <div className="preset-manager">
      <div className="preset-manager-summary">
        <div>
          <strong>{modelPresets.length} 个参数预设</strong>
          <span>每个模型可保存多套参数，生成时默认选择标记为“默认”的预设。</span>
        </div>
        <Button icon={<PlusOutlined/>} onClick={startCreate} type="primary">新建预设</Button>
      </div>
      <div className="preset-flat-list">
        {modelPresets.map((item) => <article className="preset-flat-row" key={item.id}>
          <div className="preset-flat-name"><strong>{item.name}</strong>{item.is_default && <span><CheckCircleOutlined/> 默认</span>}</div>
          <p>{parameterSummary(item.parameters)}</p>
          <div className="preset-flat-actions">
            <Button icon={<EditOutlined/>} onClick={() => startEdit(item)} size="small" type="text">编辑</Button>
            <Button icon={<CopyOutlined/>} onClick={() => startCopy(item)} size="small" type="text">复制</Button>
            {!item.is_default && <Button icon={<StarOutlined/>} loading={setDefault.isPending} onClick={() => setDefault.mutate(item)} size="small" type="text">设默认</Button>}
            <Popconfirm cancelText="取消" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => remove.mutate(item.id)} title="删除这个参数预设？"><Button danger icon={<DeleteOutlined/>} size="small" type="text"/></Popconfirm>
          </div>
        </article>)}
        {!modelPresets.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有参数预设"/>}
      </div>
      {draft && model && <div className="preset-editor-panel">
        <div className="preset-editor-heading">
          <div><strong>{draft.id ? '编辑参数预设' : '新建参数预设'}</strong><span>参数表单来自当前模型的 Schema。</span></div>
          <Button onClick={() => setDraft(null)} type="text">收起</Button>
        </div>
        <div className="preset-editor-meta">
          <label><span>名称</span><Input autoFocus onChange={(event) => setDraft((current) => current ? { ...current, name: event.target.value } : current)} placeholder="例如：竖屏高清" value={draft.name}/></label>
          <Checkbox checked={draft.isDefault} onChange={(event) => setDraft((current) => current ? { ...current, isDefault: event.target.checked } : current)}>设为默认参数预设</Checkbox>
        </div>
        <ModelParameterEditor definition={model} onChange={(parameters) => setDraft((current) => current ? { ...current, parameters } : current)} value={draft.parameters}/>
        <div className="preset-editor-actions">
          <Button onClick={() => setDraft(null)}>取消</Button>
          <Button disabled={!draft.name.trim()} loading={save.isPending} onClick={() => save.mutate(draft)} type="primary">保存参数预设</Button>
        </div>
      </div>}
    </div>
  </Modal>
}

function parameterSummary(parameters: Record<string, unknown>) {
  const entries = Object.entries(parameters)
  if (!entries.length) return '空参数'
  return entries.slice(0, 4).map(([key, value]) => `${key}: ${shortValue(value)}`).join(' · ') + (entries.length > 4 ? ` · +${entries.length - 4}` : '')
}

function shortValue(value: unknown) {
  if (typeof value === 'object' && value !== null) return Array.isArray(value) ? `[${value.length}]` : '{…}'
  const text = String(value)
  return text.length > 18 ? `${text.slice(0, 18)}…` : text
}
