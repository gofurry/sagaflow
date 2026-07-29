import { useMemo, useState } from 'react'
import { AudioOutlined, DeleteOutlined, EditOutlined, LinkOutlined, PlusOutlined, ReloadOutlined, SoundOutlined } from '@ant-design/icons'
import { Alert, App, Button, Empty, Form, Input, Modal, Popconfirm, Radio, Select, Switch, Tag, Upload } from 'antd'
import type { UploadFile } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Model, VoiceBinding, VoiceProfile } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'

type ProfileForm = {
  name: string
  description?: string
  kind: 'clone' | 'design'
  reference_text?: string
  design_prompt?: string
}

type BindingForm = {
  model_id: string
  voice_id: string
  preview_text: string
  source_url?: string
  need_noise_reduction?: boolean
  need_volume_normalization?: boolean
}

const operationLabel = { clone: '音色克隆', design: '音色设计' } as const

export function VoiceProfileManager({ models, onError }: { models: Model[]; onError: (error: unknown) => void }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [profileForm] = Form.useForm<ProfileForm>()
  const [bindingForm] = Form.useForm<BindingForm>()
  const [editForm] = Form.useForm()
  const [createOpen, setCreateOpen] = useState(false)
  const [bindingProfile, setBindingProfile] = useState<VoiceProfile | null>(null)
  const [editing, setEditing] = useState<VoiceProfile | null>(null)
  const [preview, setPreview] = useState<{ profile: VoiceProfile; binding: VoiceBinding } | null>(null)
  const [sourceFile, setSourceFile] = useState<File | null>(null)
  const kind = (Form.useWatch('kind', profileForm) ?? 'clone') as 'clone' | 'design'
  const selectedModelID = Form.useWatch('model_id', bindingForm) as string | undefined
  const voicesQuery = useQuery({ queryKey: ['voice-profiles'], queryFn: api.voiceProfiles })
  const capabilitiesQuery = useQuery({ queryKey: ['voice-capabilities'], queryFn: api.voiceCapabilities })
  const capabilities = useMemo(() => capabilitiesQuery.data ?? [], [capabilitiesQuery.data])
  const capabilityByProvider = useMemo(() => new Map(capabilities.map((item) => [item.provider_code, item])), [capabilities])
  const eligibleModels = useMemo(() => {
    if (!bindingProfile) return []
    return models.filter((model) => model.enabled && model.capability === 'audio'
      && model.features.includes(`voice_${bindingProfile.kind}`)
      && capabilityByProvider.get(model.provider_code)?.operations.includes(bindingProfile.kind))
  }, [bindingProfile, capabilityByProvider, models])
  const selectedModel = eligibleModels.find((model) => model.id === selectedModelID)
  const selectedCapability = selectedModel ? capabilityByProvider.get(selectedModel.provider_code) : undefined
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['voice-profiles'] })

  const createProfile = useMutation({
    mutationFn: (values: ProfileForm) => {
      const data = new FormData()
      Object.entries(values).forEach(([key, value]) => {
        if (value !== undefined && value !== '') data.append(key, String(value))
      })
      if (values.kind === 'clone' && sourceFile) data.append('file', sourceFile)
      return api.createVoiceProfile(data)
    },
    onSuccess: async (profile) => {
      await refresh()
      setCreateOpen(false)
      setSourceFile(null)
      profileForm.resetFields()
      message.success('音色档案已保存，可继续绑定任一兼容模型')
      openBinding(profile)
    },
    onError,
  })
  const createBinding = useMutation({
    mutationFn: (values: BindingForm) => api.createVoiceBinding(bindingProfile!.id, values),
    onSuccess: async () => {
      await refresh()
      setBindingProfile(null)
      bindingForm.resetFields()
      message.success('模型音色已创建并完成试听校验')
    },
    onError,
  })
  const update = useMutation({
    mutationFn: (values: { name: string; description?: string }) => api.updateVoiceProfile(editing!.id, values),
    onSuccess: async () => {
      await refresh()
      setEditing(null)
      editForm.resetFields()
      message.success('音色档案已更新')
    },
    onError,
  })
  const removeProfile = useMutation({
    mutationFn: api.deleteVoiceProfile,
    onSuccess: async () => {
      await refresh()
      message.success('音色档案、远端绑定及本地文件已删除')
    },
    onError,
  })
  const removeBinding = useMutation({
    mutationFn: api.deleteVoiceBinding,
    onSuccess: async () => {
      await refresh()
      message.success('远端音色绑定已删除')
    },
    onError,
  })

  const sourceList: UploadFile[] = sourceFile
    ? [{ uid: 'source', name: sourceFile.name, size: sourceFile.size, type: sourceFile.type, status: 'done' }]
    : []
  const openCreate = () => {
    profileForm.setFieldsValue({ kind: 'clone' })
    setCreateOpen(true)
  }
  const closeCreate = () => {
    setCreateOpen(false)
    setSourceFile(null)
    profileForm.resetFields()
  }
  const openBinding = (profile: VoiceProfile) => {
    const candidates = models.filter((model) => model.enabled && model.capability === 'audio'
      && model.features.includes(`voice_${profile.kind}`)
      && capabilityByProvider.get(model.provider_code)?.operations.includes(profile.kind))
    bindingForm.setFieldsValue({
      model_id: candidates[0]?.id,
      preview_text: '这是音色创建后的正式试听，用于确认声音效果。',
      need_noise_reduction: false,
      need_volume_normalization: true,
    })
    setBindingProfile(profile)
  }
  const openEdit = (profile: VoiceProfile) => {
    setEditing(profile)
    editForm.setFieldsValue({ name: profile.name, description: profile.description })
  }

  return <div className="voice-profile-section">
    <FloatingToolbar ariaLabel="音色工具栏" items={[
      { key: 'create', label: '新建音色档案', icon: <PlusOutlined/>, active: true, onClick: openCreate },
      { key: 'refresh', label: '刷新音色列表', icon: <ReloadOutlined/>, loading: voicesQuery.isFetching, onClick: () => void refresh() },
    ]}/>

    <div className="model-flat-list voice-flat-list">
      {(voicesQuery.data ?? []).map((profile) => <article className="model-flat-item voice-profile-card" key={profile.id}>
        <div className="voice-profile-heading">
          <div>
            <div className="model-row-primary"><strong>{profile.name}</strong><Tag color={profile.kind === 'design' ? 'purple' : 'orange'}>{operationLabel[profile.kind]}</Tag></div>
            <small>{profile.description || (profile.kind === 'clone' ? profile.reference_text : profile.design_prompt)}</small>
          </div>
          <div className="model-row-actions">
            <Button icon={<LinkOutlined/>} onClick={() => openBinding(profile)} size="small">绑定模型</Button>
            <Button icon={<EditOutlined/>} onClick={() => openEdit(profile)} size="small">编辑</Button>
            <Popconfirm cancelText="取消" description="会先删除各服务中的远端音色，再删除本地档案和文件。" okButtonProps={{ danger: true }} okText="永久删除" onConfirm={() => removeProfile.mutate(profile.id)} title="删除整个音色档案？">
              <Button danger icon={<DeleteOutlined/>} size="small" type="text"/>
            </Popconfirm>
          </div>
        </div>
        {profile.kind === 'clone' && <Button href={api.voiceProfileSourceURL(profile.id)} icon={<AudioOutlined/>} target="_blank" type="link">{profile.source_name}</Button>}
        <div className="voice-binding-list">
          {profile.bindings.map((binding) => <div className="voice-binding-row" key={binding.id}>
            <div><strong>{binding.provider_name} · {binding.model_name}</strong><code>{binding.voice_id}</code></div>
            <Tag color="green">已就绪</Tag>
            <div className="model-row-actions">
              <Button disabled={!binding.preview_file_size_bytes} icon={<SoundOutlined/>} onClick={() => setPreview({ profile, binding })} size="small">试听</Button>
              <Popconfirm cancelText="取消" description="会删除服务端音色与本机试听，但保留本地音色档案。" okButtonProps={{ danger: true }} okText="删除绑定" onConfirm={() => removeBinding.mutate(binding.id)} title="删除这个模型绑定？">
                <Button danger icon={<DeleteOutlined/>} size="small" type="text"/>
              </Popconfirm>
            </div>
          </div>)}
          {!profile.bindings.length && <div className="voice-binding-empty">尚未绑定服务模型，档案不会产生外部费用。</div>}
        </div>
      </article>)}
      {!voicesQuery.isLoading && !(voicesQuery.data ?? []).length && <Empty description="还没有音色档案" image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
      {voicesQuery.isLoading && <div className="model-list-loading">正在读取音色…</div>}
    </div>

    <Modal cancelText="取消" confirmLoading={createProfile.isPending} okButtonProps={{ disabled: kind === 'clone' && !sourceFile }} okText="保存档案" onCancel={closeCreate} onOk={() => profileForm.submit()} open={createOpen} title="新建音色档案" width={760}>
      <Alert message="档案只保存可复用的音色意图和本地参考素材；保存后再选择 MiniMax、阿里云、硅基流动或智谱等兼容模型创建绑定。" showIcon type="info"/>
      <Form form={profileForm} layout="vertical" onFinish={(values) => createProfile.mutate(values)} requiredMark={false} style={{ marginTop: 18 }}>
        <Form.Item label="方式" name="kind" rules={[{ required: true }]}><Radio.Group options={[{ label: '克隆已有声音', value: 'clone' }, { label: '用文字设计声音', value: 'design' }]}/></Form.Item>
        <div className="voice-form-grid">
          <Form.Item label="档案名称" name="name" rules={[{ required: true, whitespace: true }]}><Input autoFocus placeholder="例如：林默 · 冷静青年"/></Form.Item>
          <Form.Item label="说明" name="description"><Input placeholder="角色、年龄、情绪和适用场景"/></Form.Item>
        </div>
        {kind === 'clone' ? <>
          <Form.Item extra="mp3、m4a 或 wav，不超过 20 MB。创建具体模型绑定时会按平台能力再次校验。" label="参考音频" required>
            <Upload.Dragger accept=".mp3,.m4a,.wav,audio/*" beforeUpload={(file) => { setSourceFile(file); return false }} fileList={sourceList} maxCount={1} onRemove={() => { setSourceFile(null); return true }}>
              <p className="ant-upload-drag-icon"><AudioOutlined/></p><p>拖入或点击选择清晰、单人说话的音频</p>
            </Upload.Dragger>
          </Form.Item>
          <Form.Item extra="逐字填写参考音频的说话内容，确保同一档案可绑定要求文本对齐的平台。" label="参考音频对应文本" name="reference_text" rules={[{ required: true, whitespace: true }]}><Input.TextArea rows={3}/></Form.Item>
        </> : <Form.Item extra="描述性别、年龄、音色、语速、口音、情绪和表达风格，不绑定具体模型。" label="声音设计提示词" name="design_prompt" rules={[{ required: true, whitespace: true }]}><Input.TextArea placeholder="年轻女性，声音温暖清晰，普通话自然，语速中等，叙述时克制而有画面感。" rows={5}/></Form.Item>}
      </Form>
    </Modal>

    <Modal cancelText="取消" confirmLoading={createBinding.isPending} okButtonProps={{ disabled: !eligibleModels.length }} okText="创建并试听" onCancel={() => { setBindingProfile(null); bindingForm.resetFields() }} onOk={() => bindingForm.submit()} open={!!bindingProfile} title={bindingProfile ? `绑定模型 · ${bindingProfile.name}` : ''} width={760}>
      {bindingProfile && !eligibleModels.length && <Alert message={`当前没有已启用且支持${operationLabel[bindingProfile.kind]}的模型，请先在模型目录启用模型并配置凭证。`} showIcon type="warning"/>}
      <Form form={bindingForm} layout="vertical" onFinish={(values) => createBinding.mutate(values)} requiredMark={false}>
        <Form.Item label="服务与模型" name="model_id" rules={[{ required: true }]}><Select options={eligibleModels.map((model) => ({ value: model.id, label: `${model.provider_name} · ${model.display_name}` }))}/></Form.Item>
        {selectedCapability && <Alert message={`${selectedCapability.provider_name}：${selectedCapability.operations.map((item) => operationLabel[item]).join('、')}`} description={selectedCapability.voice_id_hint} showIcon type="info"/>}
        <Form.Item label={selectedCapability?.voice_id_label || 'Voice ID / 名称'} name="voice_id" rules={[{ required: true, whitespace: true }]} style={{ marginTop: 16 }}><Input placeholder={selectedCapability?.voice_id_hint}/></Form.Item>
        {bindingProfile?.kind === 'clone' && selectedCapability?.clone_requires_public_url && <Form.Item extra="阿里云服务端必须能直接访问该音频；本机文件不会自动公开。" label="参考音频公网 URL" name="source_url" rules={[{ required: true, type: 'url' }]}><Input placeholder="https://example.com/reference.wav"/></Form.Item>}
        <Form.Item label="试听文本" name="preview_text" rules={[{ required: true, whitespace: true }]}><Input.TextArea maxLength={1000} rows={3}/></Form.Item>
        {(selectedCapability?.supports_noise_reduction || selectedCapability?.supports_normalization) && <div className="voice-form-grid">
          {selectedCapability.supports_noise_reduction && <Form.Item label="降噪" name="need_noise_reduction" valuePropName="checked"><Switch/></Form.Item>}
          {selectedCapability.supports_normalization && <Form.Item label="音量归一化" name="need_volume_normalization" valuePropName="checked"><Switch/></Form.Item>}
        </div>}
      </Form>
    </Modal>

    <Modal cancelText="取消" confirmLoading={update.isPending} okText="保存" onCancel={() => setEditing(null)} onOk={() => editForm.submit()} open={!!editing} title="编辑音色档案">
      <Form form={editForm} layout="vertical" onFinish={(values) => update.mutate(values)} requiredMark={false}>
        <Form.Item label="名称" name="name" rules={[{ required: true, whitespace: true }]}><Input autoFocus/></Form.Item>
        <Form.Item label="说明" name="description"><Input.TextArea rows={3}/></Form.Item>
      </Form>
    </Modal>

    <Modal footer={null} onCancel={() => setPreview(null)} open={!!preview} title={preview ? `${preview.profile.name} · ${preview.binding.provider_name} 试听` : ''} width={680}>
      {preview && <div className="voice-preview-player"><SoundOutlined/><audio autoPlay controls src={api.voiceBindingPreviewURL(preview.binding.id)}/><span>{preview.binding.voice_id}</span></div>}
    </Modal>
  </div>
}
