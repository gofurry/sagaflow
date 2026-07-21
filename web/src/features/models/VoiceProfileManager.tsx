import { useState } from 'react'
import { AudioOutlined, DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SoundOutlined } from '@ant-design/icons'
import { Alert, App, Button, Empty, Form, Input, Modal, Popconfirm, Select, Switch, Upload } from 'antd'
import type { UploadFile } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Model, Project, VoiceProfile } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'

export function VoiceProfileManager({ models, onError, project }: { models: Model[]; onError: (error: unknown) => void; project: Project }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<VoiceProfile | null>(null)
  const [preview, setPreview] = useState<VoiceProfile | null>(null)
  const [sourceFile, setSourceFile] = useState<File | null>(null)
  const [promptFile, setPromptFile] = useState<File | null>(null)
  const voicesQuery = useQuery({ queryKey: ['voice-profiles', project.id], queryFn: () => api.voiceProfiles(project.id) })
  const audioModels = models.filter((model) => model.enabled && model.capability === 'audio' && model.provider_code === 'minimax')
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['voice-profiles', project.id] })
  const closeCreate = () => {
    setCreateOpen(false)
    setSourceFile(null)
    setPromptFile(null)
    form.resetFields()
  }
  const create = useMutation({
    mutationFn: (values: Record<string, unknown>) => {
      const data = new FormData()
      data.append('file', sourceFile!)
      if (promptFile) data.append('prompt_file', promptFile)
      Object.entries(values).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') data.append(key, String(value))
      })
      return api.createVoiceProfile(project.id, data)
    },
    onSuccess: async () => {
      await refresh()
      closeCreate()
      message.success('音色已克隆、激活并保存')
    },
    onError,
  })
  const update = useMutation({
    mutationFn: (values: { name: string; description?: string }) => api.updateVoiceProfile(editing!.id, values),
    onSuccess: async () => {
      await refresh()
      setEditing(null)
      editForm.resetFields()
      message.success('音色信息已更新')
    },
    onError,
  })
  const remove = useMutation({
    mutationFn: api.deleteVoiceProfile,
    onSuccess: async () => {
      await refresh()
      message.success('音色及本地样本已删除')
    },
    onError,
  })
  const sourceList: UploadFile[] = sourceFile ? [{ uid: 'source', name: sourceFile.name, size: sourceFile.size, type: sourceFile.type, status: 'done' }] : []
  const promptList: UploadFile[] = promptFile ? [{ uid: 'prompt', name: promptFile.name, size: promptFile.size, type: promptFile.type, status: 'done' }] : []
  const openCreate = () => {
    form.setFieldsValue({
      model_id: audioModels[0]?.id,
      preview_text: '这是音色创建后的正式试听，用于确认声音效果。',
      need_noise_reduction: false,
      need_volume_normalization: true,
    })
    setCreateOpen(true)
  }
  const openEdit = (profile: VoiceProfile) => {
    setEditing(profile)
    editForm.setFieldsValue({ name: profile.name, description: profile.description })
  }
  return <div className="voice-profile-section">
    <FloatingToolbar ariaLabel="音色工具栏" items={[
      { key: 'create', label: audioModels.length ? '克隆新音色' : '需要先启用 MiniMax 音频模型', icon: <PlusOutlined/>, active: true, disabled: audioModels.length === 0, onClick: openCreate },
      { key: 'refresh', label: '刷新音色列表', icon: <ReloadOutlined/>, loading: voicesQuery.isFetching, onClick: () => void refresh() },
    ]}/>
    <div className="model-flat-list voice-flat-list">
      {(voicesQuery.data ?? []).map((item) => {
        return <article className="model-flat-item" key={item.id}>
          <div className="model-flat-row voice-flat-row">
            <div className="model-row-primary"><strong>{item.name}</strong><code>{item.voice_id}</code></div>
            <div className="voice-row-model"><span>{item.model_identifier}</span><small>{item.description || '暂无说明'}</small></div>
            <Button href={api.voiceProfileSourceURL(item.id)} icon={<AudioOutlined/>} target="_blank" type="text">{item.source_name}</Button>
            <div className="model-row-actions">
              <Button disabled={!item.preview_file_size_bytes} icon={<SoundOutlined/>} onClick={() => setPreview(item)} size="small">试听</Button>
              <Button icon={<EditOutlined/>} onClick={() => openEdit(item)} size="small">编辑</Button>
              <Popconfirm cancelText="取消" description="会同时删除 MiniMax 远端音色和项目内保存的样本与试听文件。" okButtonProps={{ danger: true }} okText="永久删除" onConfirm={() => remove.mutate(item.id)} title="删除这个克隆音色？"><Button danger icon={<DeleteOutlined/>} size="small" type="text"/></Popconfirm>
            </div>
          </div>
        </article>
      })}
      {!voicesQuery.isLoading && !(voicesQuery.data ?? []).length && <Empty description="还没有克隆音色" image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
      {voicesQuery.isLoading && <div className="model-list-loading">正在读取音色…</div>}
    </div>

    <Modal cancelText="取消" confirmLoading={create.isPending} okButtonProps={{ disabled: !sourceFile }} okText="克隆并激活" onCancel={closeCreate} onOk={() => form.submit()} open={createOpen} title="克隆 MiniMax 音色" width={820}>
      <Alert message="创建后会立即使用该音色合成一次正式试听，使音色完成激活；MiniMax 会在首次正式合成时收取音色复刻费用。" showIcon type="info"/>
      <Form form={form} layout="vertical" onFinish={(values) => create.mutate(values)} requiredMark={false} style={{ marginTop: 18 }}>
        <div className="voice-form-grid">
          <Form.Item label="音色名称" name="name" rules={[{ required: true, whitespace: true }]}><Input autoFocus placeholder="例如：林默 · 冷静青年"/></Form.Item>
          <Form.Item label="Voice ID" name="voice_id" rules={[{ required: true }, { pattern: /^[A-Za-z][A-Za-z0-9_-]{6,254}[A-Za-z0-9]$/, message: '8-256 位，以字母开头，只能使用字母、数字、-、_' }]}><Input placeholder="例如 LinMoVoice01"/></Form.Item>
        </div>
        <Form.Item label="语音模型" name="model_id" rules={[{ required: true }]}><Select options={audioModels.map((model) => ({ value: model.id, label: `${model.provider_name} · ${model.display_name}` }))}/></Form.Item>
        <Form.Item label="说明" name="description"><Input placeholder="记录角色、年龄、情绪和适用场景"/></Form.Item>
        <Form.Item extra="mp3、m4a 或 wav；10 秒至 5 分钟；不超过 20 MB。" label="待克隆音频" required>
          <Upload.Dragger accept=".mp3,.m4a,.wav,audio/*" beforeUpload={(file) => { setSourceFile(file); return false }} fileList={sourceList} maxCount={1} onRemove={() => { setSourceFile(null); return true }}>
            <p className="ant-upload-drag-icon"><AudioOutlined/></p><p>拖入或点击选择清晰、单人说话的音频</p>
          </Upload.Dragger>
        </Form.Item>
        <Form.Item extra="可选，小于 8 秒；同时填写下方示例音频对应文本，可提高稳定性。" label="示例音频">
          <Upload.Dragger accept=".mp3,.m4a,.wav,audio/*" beforeUpload={(file) => { setPromptFile(file); return false }} fileList={promptList} maxCount={1} onRemove={() => { setPromptFile(null); return true }}>
            <p>选择一小段示例音频</p>
          </Upload.Dragger>
        </Form.Item>
        {promptFile && <Form.Item label="示例音频文本" name="prompt_text" rules={[{ required: true, whitespace: true }]}><Input.TextArea rows={2}/></Form.Item>}
        <Form.Item label="正式试听文本" name="preview_text" rules={[{ required: true, whitespace: true }]}><Input.TextArea maxLength={1000} rows={3}/></Form.Item>
        <div className="voice-form-grid">
          <Form.Item label="降噪" name="need_noise_reduction" valuePropName="checked"><Switch/></Form.Item>
          <Form.Item label="音量归一化" name="need_volume_normalization" valuePropName="checked"><Switch/></Form.Item>
        </div>
      </Form>
    </Modal>

    <Modal cancelText="取消" confirmLoading={update.isPending} okText="保存" onCancel={() => setEditing(null)} onOk={() => editForm.submit()} open={!!editing} title="编辑音色">
      <Form form={editForm} layout="vertical" onFinish={(values) => update.mutate(values)} requiredMark={false}>
        <Form.Item label="名称" name="name" rules={[{ required: true, whitespace: true }]}><Input autoFocus/></Form.Item>
        <Form.Item label="说明" name="description"><Input.TextArea rows={3}/></Form.Item>
      </Form>
    </Modal>

    <Modal footer={null} onCancel={() => setPreview(null)} open={!!preview} title={preview ? `${preview.name} · 正式试听` : ''} width={680}>
      {preview && <div className="voice-preview-player"><SoundOutlined/><audio autoPlay controls src={api.voiceProfilePreviewURL(preview.id)}/><span>{preview.voice_id}</span></div>}
    </Modal>
  </div>
}
