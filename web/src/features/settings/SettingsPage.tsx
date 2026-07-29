import { CloudOutlined, DeleteOutlined, EyeOutlined, FileTextOutlined, ReloadOutlined, SaveOutlined, SearchOutlined, SettingOutlined, UserOutlined } from '@ant-design/icons'
import { Alert, App, Button, Empty, Form, Input, InputNumber, Modal, Pagination, Progress, Select, Spin, Timeline, Typography } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useDeferredValue, useEffect, useMemo, useState } from 'react'
import { api } from '../../api/client'
import type { GenerationInvocation, GenerationJob, Project } from '../../api/types'
import { queryKeys } from '../../api/queryKeys'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { AccountManagement, StorageManagement } from './SystemManagement'

type SettingsSection = 'project' | 'jobs' | 'account' | 'storage'
const JOB_PAGE_SIZE = 8

export function SettingsPage({ project, onError }: { project?: Project; onError: (error: unknown) => void }) {
  const queryClient = useQueryClient()
  const { message, modal } = App.useApp()
  const [form] = Form.useForm()
  const [section, setSection] = useState<SettingsSection>('project')
  const [selectedJobID, setSelectedJobID] = useState<string>()
  const [jobSearch, setJobSearch] = useState('')
  const [jobStatus, setJobStatus] = useState<string>()
  const [jobCapability, setJobCapability] = useState<string>()
  const [jobPage, setJobPage] = useState(1)
  const deferredJobSearch = useDeferredValue(jobSearch.trim())
  const jobFilters = useMemo(() => ({
    search: deferredJobSearch || undefined,
    status: jobStatus,
    capability: jobCapability,
    page: jobPage,
    page_size: JOB_PAGE_SIZE,
  }), [deferredJobSearch, jobCapability, jobPage, jobStatus])
  const jobsQuery = useQuery({
    queryKey: queryKeys.generationJobs(project?.id ?? '', jobFilters),
    queryFn: () => api.jobs(project!.id, jobFilters),
    enabled: Boolean(project) && section === 'jobs',
    placeholderData: (previous) => previous,
  })
  const invocationQuery = useQuery({
    queryKey: ['generation-invocation', selectedJobID],
    queryFn: () => api.generationInvocation(selectedJobID!),
    enabled: Boolean(selectedJobID),
    retry: false,
  })
  const update = useMutation({
    mutationFn: (values: { title: string; description: string; aspect_ratio?: string; resolution?: string; frame_rate?: number | null }) => api.updateProject(project!.id, values),
    onSuccess: async () => {
      await Promise.all([queryClient.invalidateQueries({ queryKey: ['projects'] }), queryClient.invalidateQueries({ queryKey: ['models'] }), queryClient.invalidateQueries({ queryKey: ['workflows'] })])
      message.success('项目设置已保存')
    },
    onError,
  })
  const remove = useMutation({
    mutationFn: () => api.deleteProject(project!.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['projects'] })
      message.success('项目已删除')
    },
    onError,
  })
  const jobs = useMemo(() => jobsQuery.data?.items ?? [], [jobsQuery.data?.items])
  const totalJobs = jobsQuery.data?.total ?? 0
  const selectedJob = jobs.find((item) => item.id === selectedJobID)
  useEffect(() => { if (project) form.setFieldsValue({ title: project.title, description: project.description, aspect_ratio: project.aspect_ratio, resolution: project.resolution, frame_rate: project.frame_rate }) }, [form, project])
  useEffect(() => {
	if (!project && (section === 'project' || section === 'jobs')) setSection('account')
	}, [project, section])
  useEffect(() => {
    const lastPage = Math.max(1, Math.ceil(totalJobs / JOB_PAGE_SIZE))
    if (jobPage > lastPage) setJobPage(lastPage)
  }, [jobPage, totalJobs])
  const confirmDelete = () => modal.confirm({
    title: '永久删除当前项目？',
	content: '项目、分集、画布和本地资产记录都会删除；存在 S3 副本时需先从资产页删除副本。',
    okText: '删除项目',
    okButtonProps: { danger: true },
    cancelText: '取消',
    onOk: () => remove.mutateAsync(),
  })
  const toolbarItems = section === 'project' && project ? [
    { key: 'save', label: '保存项目设置', icon: <SaveOutlined/>, active: true, loading: update.isPending, onClick: () => form.submit() },
    { key: 'delete', label: '删除项目', icon: <DeleteOutlined/>, danger: true, loading: remove.isPending, onClick: confirmDelete },
  ] : section === 'jobs' ? [
    { key: 'refresh', label: '刷新调用记录', icon: <ReloadOutlined/>, active: true, loading: jobsQuery.isFetching, onClick: () => { void jobsQuery.refetch() } },
  ] : []

  return <div className="page page-settings">
    {toolbarItems.length > 0 && <FloatingToolbar ariaLabel="设置页工具栏" items={toolbarItems}/>}
    <div aria-label="设置分类" className="settings-tabs" role="tablist">
	  {project && <button aria-selected={section === 'project'} className={section === 'project' ? 'active' : ''} onClick={() => setSection('project')} role="tab" type="button"><SettingOutlined/><span>项目</span></button>}
	  {project && <button aria-selected={section === 'jobs'} className={section === 'jobs' ? 'active' : ''} onClick={() => setSection('jobs')} role="tab" type="button"><FileTextOutlined/><span>调用记录</span>{jobsQuery.data && <em>{totalJobs}</em>}</button>}
	  <button aria-selected={section === 'account'} className={section === 'account' ? 'active' : ''} onClick={() => setSection('account')} role="tab" type="button"><UserOutlined/><span>账号</span></button>
	  <button aria-selected={section === 'storage'} className={section === 'storage' ? 'active' : ''} onClick={() => setSection('storage')} role="tab" type="button"><CloudOutlined/><span>S3 发布</span></button>
    </div>
    <main className="settings-workspace">
      {section === 'project' && project && <section className="settings-section">
        <SettingsSectionHeading title="项目设置"/>
        <Form className="settings-project-form" form={form} layout="vertical" onFinish={(values) => update.mutate(values)} requiredMark={false}>
          <Form.Item label="项目名称" name="title" rules={[{ required: true, whitespace: true, message: '请输入项目名称' }]}><Input maxLength={120}/></Form.Item>
          <Form.Item label="项目说明" name="description"><Input.TextArea autoSize={{ minRows: 5, maxRows: 10 }} maxLength={2000} showCount/></Form.Item>
          <div className="settings-project-specs">
            <Form.Item label="画幅" name="aspect_ratio"><Input maxLength={32} placeholder="例如 9:16"/></Form.Item>
            <Form.Item label="分辨率" name="resolution"><Input maxLength={64} placeholder="例如 1080 × 1920"/></Form.Item>
            <Form.Item label="帧率" name="frame_rate"><InputNumber max={240} min={1} placeholder="例如 24" precision={3}/></Form.Item>
          </div>
          <Typography.Text className="settings-project-spec-note" type="secondary">制作规格只作为项目记录展示，不会自动修改模型参数或最终导出设置。</Typography.Text>
        </Form>
        <div className="settings-project-facts">
          <div><span>项目 ID</span><code>{project.id}</code></div>
          <div><span>创建时间</span><strong>{formatDateTime(project.created_at)}</strong></div>
          <div><span>最后更新</span><strong>{formatDateTime(project.updated_at)}</strong></div>
        </div>
      </section>}

	  {section === 'jobs' && project && <section className="settings-section settings-jobs-section">
        <SettingsSectionHeading title="生成调用记录" count={`${totalJobs}`}/>
        <div className="settings-job-filters">
          <Input allowClear onChange={(event) => { setJobSearch(event.target.value); setJobPage(1) }} placeholder="搜索 Provider、模型、Prompt 或错误" prefix={<SearchOutlined/>} value={jobSearch}/>
          <Select allowClear onChange={(value) => { setJobCapability(value); setJobPage(1) }} options={['text', 'image', 'audio', 'video'].map((value) => ({ value, label: capabilityLabel(value) }))} placeholder="全部能力" value={jobCapability}/>
          <Select allowClear onChange={(value) => { setJobStatus(value); setJobPage(1) }} options={['queued', 'running', 'succeeded', 'failed', 'canceled', 'interrupted'].map((value) => ({ value, label: jobStatusLabel(value) }))} placeholder="全部状态" value={jobStatus}/>
        </div>
        {jobsQuery.isLoading ? <div className="settings-jobs-loading"><Spin/></div> : jobs.length === 0 ? <Empty description={jobSearch || jobStatus || jobCapability ? '没有符合筛选条件的调用记录' : '还没有生成调用记录'} image={Empty.PRESENTED_IMAGE_SIMPLE}/> : <div className="settings-job-list">
          {jobs.map((job) => <article className="settings-job-item" key={job.id}>
            <time>{formatDateTime(job.created_at)}</time>
            <div className="settings-job-target"><strong>{job.model_identifier || '未指定目标'}</strong><span>{job.provider_code || '未知 Provider'}</span></div>
            <span className={`settings-job-capability ${job.capability}`}>{capabilityLabel(job.capability)}</span>
            <JobStatusTag job={job}/>
            <div className="settings-job-output"><strong>{job.output_staged_asset_ids.length}</strong><span>个输出</span></div>
            <Button icon={<EyeOutlined/>} onClick={() => setSelectedJobID(job.id)} size="small" type="text">详情</Button>
            {job.error_message && <p>{job.error_message}</p>}
          </article>)}
        </div>}
        {totalJobs > JOB_PAGE_SIZE && <Pagination className="settings-job-pagination" current={jobPage} hideOnSinglePage onChange={setJobPage} pageSize={JOB_PAGE_SIZE} showSizeChanger={false} total={totalJobs}/>}
      </section>}
	  {section === 'account' && <AccountManagement onError={onError}/>}
	  {section === 'storage' && <StorageManagement onError={onError}/>}
    </main>
    <InvocationModal
      job={selectedJob}
      invocation={invocationQuery.data}
      loading={invocationQuery.isLoading}
      missing={invocationQuery.isError}
      open={Boolean(selectedJobID)}
      onClose={() => setSelectedJobID(undefined)}
    />
  </div>
}

function JobStatusTag({ job }: { job: GenerationJob }) {
  return <span className={`settings-job-status ${job.status}`}>{jobStatusLabel(job.status)} · {stageLabel(job.stage)}</span>
}

function SettingsSectionHeading({ count, title }: { count?: string; title: string }) {
  return <div className="settings-section-heading"><div><Typography.Title level={3}>{title}</Typography.Title></div>{count && <strong>{count}</strong>}</div>
}

function InvocationModal({ job, invocation, loading, missing, open, onClose }: {
  job?: GenerationJob
  invocation?: GenerationInvocation
  loading: boolean
  missing: boolean
  open: boolean
  onClose: () => void
}) {
  const request = invocation?.request_snapshot
  const response = invocation?.response_snapshot
  const providerRequests = invocation?.events.filter((event) => event.stage === 'provider_request' && Object.keys(event.detail_snapshot ?? {}).length > 0) ?? []
  return <Modal centered className="invocation-modal" title="生成调用详情" width={1080} open={open} onCancel={onClose} footer={<Button onClick={onClose}>关闭</Button>}>
    {loading ? <div className="invocation-loading"><Spin/></div> : missing ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该任务创建于调用追踪启用之前，没有详细记录"/> : invocation ? <div className="invocation-detail">
      <div className="invocation-summary">
        <SummaryItem label="Provider" value={invocation.provider_code}/>
        <SummaryItem label="生成目标" value={invocation.model_identifier}/>
        <SummaryItem label="能力" value={capabilityLabel(invocation.capability)}/>
        <SummaryItem label="状态" value={jobStatusLabel(invocation.status)}/>
        <SummaryItem label="总耗时" value={formatDuration(invocation.duration_ms)}/>
        <SummaryItem label="凭据来源" value={invocation.credential_source === 'database' ? '本机加密凭据' : '无需凭据'}/>
        <SummaryItem label="Provider 任务 ID" value={invocation.provider_job_id || '—'} wide/>
        <SummaryItem label="请求端点" value={request?.endpoint || '—'} wide/>
      </div>

      <TraceSection title="生成内容">
        <Typography.Paragraph className="invocation-prompt" copyable>{request?.prompt || job?.prompt || '—'}</Typography.Paragraph>
      </TraceSection>
      <TraceSection title="最终请求参数">
        <JSONView value={request?.parameters ?? job?.parameters ?? {}}/>
      </TraceSection>
      <TraceSection title={`实际 Provider 请求 · ${providerRequests.length}`}>
        {providerRequests.length ? <div className="invocation-provider-requests">{providerRequests.map((event) => <div key={event.id}>
          <Typography.Text type="secondary">{event.message || 'Provider request'}</Typography.Text>
          <JSONView value={event.detail_snapshot}/>
        </div>)}</div> : <Typography.Text type="secondary">尚未提交 Provider 请求</Typography.Text>}
      </TraceSection>
      <TraceSection title={`参考输入 · ${request?.inputs?.length ?? 0}`}>
        {request?.inputs?.length ? <div className="invocation-inputs">{request.inputs.map((input) => <div className="invocation-input" key={input.id}>
          <strong>{input.name || input.id}</strong>
          <span>{input.media_type} · {input.mime_type || '未知格式'}</span>
          <span>{input.provider_url ? 'Provider 可访问 URL' : '无外部 URL'} · {input.content_stream ? '可读取内容流' : '无内容流'}</span>
        </div>)}</div> : <Typography.Text type="secondary">无参考输入</Typography.Text>}
      </TraceSection>
      <TraceSection title="阶段时间线">
        <Timeline items={invocation.events.map((event) => ({
          color: event.stage === 'failed' ? 'red' : event.stage === 'completed' ? 'green' : 'blue',
          content: <div className="invocation-event">
            <div><strong>{stageLabel(event.stage)}</strong><span>+{formatDuration(event.elapsed_ms)}</span></div>
            <Progress percent={Math.round(event.progress * 100)} showInfo={false} size="small"/>
            {event.message && <Typography.Text type="secondary">{event.message}</Typography.Text>}
          </div>,
        }))}/>
      </TraceSection>
      <TraceSection title={`Provider 产物 · ${response?.artifacts?.length ?? 0}`}>
        {response?.artifacts?.length ? <div className="invocation-artifacts">{response.artifacts.map((artifact, index) => <div key={`${artifact.media_type}-${index}`}>
          <strong>{artifact.media_type}</strong><span>{artifact.mime_type || '未知格式'}</span>
          {artifact.source_url && <Typography.Text copyable ellipsis>{artifact.source_url}</Typography.Text>}
        </div>)}</div> : <Typography.Text type="secondary">尚无 Provider 产物</Typography.Text>}
      </TraceSection>
      <TraceSection title="用量">
        <JSONView value={invocation.usage_snapshot ?? {}}/>
      </TraceSection>
      <TraceSection title="入库链路">
        <div className="invocation-output-links">
          <SummaryItem label="暂存素材 ID" value={invocation.output_staged_asset_ids.length ? invocation.output_staged_asset_ids.join('、') : '—'} wide/>
          <SummaryItem label="资产 ID" value={invocation.output_asset_ids.length ? invocation.output_asset_ids.join('、') : '—'} wide/>
        </div>
      </TraceSection>
      {invocation.error_message && <Alert
        type="error"
        showIcon
        message={`${invocation.error_kind || 'error'}${invocation.error_status_code ? ` · HTTP ${invocation.error_status_code}` : ''}`}
        description={`${invocation.error_message}${invocation.retryable ? '（可重试）' : ''}`}
      />}
    </div> : null}
  </Modal>
}

function SummaryItem({ label, value, wide }: { label: string; value: string; wide?: boolean }) {
  return <div className={wide ? 'wide' : ''}><span>{label}</span><strong>{value}</strong></div>
}

function TraceSection({ title, children }: { title: string; children: React.ReactNode }) {
  return <section className="invocation-section"><Typography.Title level={5}>{title}</Typography.Title>{children}</section>
}

function JSONView({ value }: { value: unknown }) {
  return <pre className="invocation-json">{JSON.stringify(value ?? {}, null, 2)}</pre>
}

function formatDuration(milliseconds: number) {
  if (milliseconds < 1000) return `${milliseconds} ms`
  if (milliseconds < 60_000) return `${(milliseconds / 1000).toFixed(1)} 秒`
  return `${Math.floor(milliseconds / 60_000)} 分 ${Math.round((milliseconds % 60_000) / 1000)} 秒`
}

function formatDateTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false })
}

function capabilityLabel(value: string) {
  return ({ text: '文本', image: '图像', audio: '音频', video: '视频', multimodal: '多模态' } as Record<string, string>)[value] ?? value
}

function jobStatusLabel(value: string) {
  return ({ queued: '排队', running: '进行中', succeeded: '已完成', failed: '失败', canceled: '已取消', interrupted: '意外中断' } as Record<string, string>)[value] ?? value
}

function stageLabel(stage: string) {
  return ({ requesting: '组装请求', generating: '提交模型', provider_request: 'Provider 请求', queued: 'Provider 排队', running: '模型生成', fetching: '获取文件', storing: '存入暂存区', completed: '完成', failed: '失败', canceled: '已取消', interrupted: '服务中断' } as Record<string, string>)[stage] ?? stage
}
