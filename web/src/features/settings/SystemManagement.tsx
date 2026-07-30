import { CloudOutlined, DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import { App, Button, Empty, Form, Input, Modal, Select, Switch, Typography } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { api } from '../../api/client'
import type { S3Connection } from '../../api/types'

export function AccountManagement({ onError }: { onError: (error: unknown) => void }) {
  const client = useQueryClient()
  const { message } = App.useApp()
  const [profileForm] = Form.useForm()
  const [passwordForm] = Form.useForm()
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  useEffect(() => { if (me.data?.user) profileForm.setFieldsValue(me.data.user) }, [me.data?.user, profileForm])
  const profile = useMutation({ mutationFn: api.updateProfile, onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['me'] }), client.invalidateQueries({ queryKey: ['auth'] })]); message.success('账号资料已保存') }, onError })
  const password = useMutation({ mutationFn: ({ current_password, new_password }: { current_password: string; new_password: string }) => api.changePassword(current_password, new_password), onSuccess: async () => { passwordForm.resetFields(); message.success('密码已更新，请重新登录'); await api.logout(); await client.invalidateQueries({ queryKey: ['auth'] }) }, onError })
  return <section className="settings-section system-management">
    <div className="settings-section-heading"><div><Typography.Title level={3}>账号</Typography.Title></div></div>
    <div className="account-settings-grid">
      <Form form={profileForm} layout="vertical" onFinish={(values) => profile.mutate(values)} requiredMark={false}>
        <Form.Item label="用户名" name="username" rules={[{ required: true }]}><Input/></Form.Item>
        <Form.Item label="显示名称" name="display_name" rules={[{ required: true }]}><Input/></Form.Item>
        <Button htmlType="submit" loading={profile.isPending} type="primary">保存资料</Button>
      </Form>
      <Form form={passwordForm} layout="vertical" onFinish={(values) => password.mutate(values)} requiredMark={false}>
        <Form.Item label="当前密码" name="current_password" rules={[{ required: true }]}><Input.Password/></Form.Item>
        <Form.Item label="新密码" name="new_password" rules={[{ required: true, min: 8 }]}><Input.Password/></Form.Item>
        <Form.Item label="确认新密码" name="confirm" dependencies={['new_password']} rules={[{ required: true }, ({ getFieldValue }) => ({ validator: (_, value) => !value || getFieldValue('new_password') === value ? Promise.resolve() : Promise.reject(new Error('两次密码不一致')) })]}><Input.Password/></Form.Item>
        <Button htmlType="submit" loading={password.isPending}>修改密码</Button>
      </Form>
    </div>
  </section>
}

export function StorageManagement({ onError }: { onError: (error: unknown) => void }) {
  const client = useQueryClient()
  const { message, modal } = App.useApp()
  const [form] = Form.useForm()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<S3Connection>()
  const connections = useQuery({ queryKey: ['s3-connections'], queryFn: api.s3Connections })
  const save = useMutation({
    mutationFn: (values: Record<string, unknown>) => editing ? api.updateS3Connection(editing.id, values) : api.createS3Connection(values),
    onSuccess: async () => { setOpen(false); setEditing(undefined); form.resetFields(); await client.invalidateQueries({ queryKey: ['s3-connections'] }); message.success('S3 连接已保存') },
    onError,
  })
  const edit = (item?: S3Connection) => {
    setEditing(item)
    form.resetFields()
    form.setFieldsValue(item ?? { provider: 's3', prefix: 'sagaflow', enabled: true, is_default: false, force_path_style: false })
    setOpen(true)
  }
  return <section className="settings-section system-management">
    <div className="settings-section-heading"><div><Typography.Title level={3}>S3 手动发布</Typography.Title><Typography.Text type="secondary">本地文件始终是唯一主副本；这里只配置需要主动上传的公网副本。</Typography.Text></div><Button icon={<PlusOutlined/>} onClick={() => edit()} type="primary">添加连接</Button></div>
    <div className="storage-backend-list">{connections.data?.map((item) => <article key={item.id}>
      <div className="storage-driver-mark"><CloudOutlined/></div>
      <div className="storage-backend-info"><strong>{item.name}</strong><span>{providerLabel(item.provider)} · {item.bucket}</span><p>{item.endpoint || 'AWS 标准 Endpoint'}{item.region ? ` · ${item.region}` : ''}</p><div className="storage-backend-state"><span className={item.enabled ? 'ready' : 'disabled'}>{item.enabled ? '可用' : '停用'}</span>{item.is_default && <em>默认发布目标</em>}</div></div>
      <div className="storage-row-actions"><Button onClick={() => void api.testS3Connection(item.id).then(() => message.success('读、写、删除均正常')).catch(onError)}>测试</Button><Button icon={<EditOutlined/>} onClick={() => edit(item)}>编辑</Button><Button danger icon={<DeleteOutlined/>} onClick={() => modal.confirm({ title: '删除 S3 连接？', content: '已有远端发布记录引用时不能删除。', onOk: async () => { await api.deleteS3Connection(item.id); await client.invalidateQueries({ queryKey: ['s3-connections'] }) } })}>删除</Button></div>
    </article>)}</div>
    {!connections.isLoading && !connections.data?.length && <Empty description="尚未配置 S3；不影响本地素材和本地模型使用" image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
    <Modal title={editing ? '编辑 S3 连接' : '添加 S3 连接'} width={760} open={open} onCancel={() => { setOpen(false); setEditing(undefined) }} onOk={() => form.submit()} confirmLoading={save.isPending}>
      <Form form={form} layout="vertical" onFinish={(values) => save.mutate(values)} requiredMark={false}>
        <div className="storage-form-grid">
          <Form.Item label="名称" name="name" rules={[{ required: true }]}><Input placeholder="例如：腾讯云 COS"/></Form.Item>
          <Form.Item label="服务类型" name="provider" rules={[{ required: true }]}><Select options={[{ value: 's3', label: 'AWS / 通用 S3' }, { value: 'tencent_cos', label: '腾讯云 COS（S3 协议）' }, { value: 'aliyun_oss', label: '阿里云 OSS（S3 协议）' }, { value: 'minio', label: 'MinIO' }, { value: 'custom', label: '其他 S3 兼容服务' }]}/></Form.Item>
          <Form.Item label="Region" name="region"><Input placeholder="例如 ap-chengdu"/></Form.Item>
          <Form.Item label="Bucket" name="bucket" rules={[{ required: true }]}><Input/></Form.Item>
          <Form.Item className="storage-form-wide" label="API Endpoint" name="endpoint" extra="SagaFlow 上传和删除时访问的 S3 API 地址；AWS S3 可留空。"><Input placeholder="https://cos.ap-chengdu.myqcloud.com"/></Form.Item>
          <Form.Item className="storage-form-wide" label="公网签名 Endpoint（可选）" name="public_endpoint" extra="仅当上面的 API 地址是内网地址时填写同一服务的公网 API 地址；不是 CDN 下载域名。"><Input/></Form.Item>
          <Form.Item label="对象前缀" name="prefix"><Input placeholder="sagaflow"/></Form.Item>
          <Form.Item name="force_path_style" label="地址形式" valuePropName="checked"><Switch/> <span>Path-style（MinIO 常用）</span></Form.Item>
          <Form.Item label="Access Key ID" name="access_key_id" extra={editing ? '不更换凭证时留空' : undefined}><Input/></Form.Item>
          <Form.Item label="Secret Access Key" name="secret_access_key" extra={editing ? '不更换凭证时留空' : undefined}><Input.Password/></Form.Item>
        </div>
        <div className="storage-switches"><Form.Item name="enabled" valuePropName="checked"><Switch/> <span>启用</span></Form.Item><Form.Item name="is_default" valuePropName="checked"><Switch/> <span>默认发布目标</span></Form.Item></div>
      </Form>
    </Modal>
  </section>
}

function providerLabel(value: string) {
  return ({ s3: '通用 S3', tencent_cos: '腾讯云 COS', aliyun_oss: '阿里云 OSS', minio: 'MinIO', custom: 'S3 兼容' } as Record<string, string>)[value] ?? value
}
