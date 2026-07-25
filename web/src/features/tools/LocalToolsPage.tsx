import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import {
  AudioOutlined,
  CameraOutlined,
  CompressOutlined,
  DeleteOutlined,
  DownloadOutlined,
  FileSearchOutlined,
  HolderOutlined,
  MergeCellsOutlined,
  PictureOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  ScissorOutlined,
  StopOutlined,
} from '@ant-design/icons'
import { Alert, App, Button, Input, InputNumber, Modal, Pagination, Progress, Radio, Select, Slider, Switch } from 'antd'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { queryKeys } from '../../api/queryKeys'
import type { Asset, AssetGroup, MediaJob, MediaJobPage, MediaTool, Project } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { completedJobTransition } from '../jobs/completionTransitions'

interface Draft {
  sourceAssetIDs: string[]
  targetGroupID?: string
  outputName: string
  parameters: Record<string, unknown>
}

type ActiveMediaTool = MediaTool

const toolOrder: ActiveMediaTool[] = ['inspect', 'transcode', 'audio', 'trim', 'merge', 'screenshot']
const STAGING_DESTINATION = '__staging__'
const toolMeta: Record<ActiveMediaTool, { label: string; icon: React.ReactNode }> = {
  inspect: { label: '检查', icon: <FileSearchOutlined/> },
  transcode: { label: '转换', icon: <CompressOutlined/> },
  audio: { label: '音频', icon: <AudioOutlined/> },
  trim: { label: '裁切', icon: <ScissorOutlined/> },
  merge: { label: '合片', icon: <MergeCellsOutlined/> },
  screenshot: { label: '截图', icon: <CameraOutlined/> },
}

const defaultParameters: Record<ActiveMediaTool, Record<string, unknown>> = {
  inspect: {},
  transcode: { format: 'mp4' },
  audio: { audio_mode: 'extract' },
  trim: { start_seconds: 0, duration: 5, fast: false },
  merge: { fast: true },
  screenshot: {
    time_seconds: 0,
    image_format: 'png',
    output_width: 0,
    output_height: 0,
    crop_x: 0,
    crop_y: 0,
    crop_width: 0,
    crop_height: 0,
  },
}

const emptyDraft = (tool: ActiveMediaTool): Draft => ({
  sourceAssetIDs: [],
  outputName: '',
  parameters: { ...defaultParameters[tool] },
})

export function LocalToolsPage({ onError, project }: { onError: (error: unknown) => void; project: Project }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [tool, setTool] = useState<ActiveMediaTool>('inspect')
  const [draft, setDraft] = useState<Draft>(() => emptyDraft('inspect'))
  const [selectedJobID, setSelectedJobID] = useState<string>()
  const [selectedAssets, setSelectedAssets] = useState<Asset[]>([])
  const [installOpen, setInstallOpen] = useState(false)
  const [installMode, setInstallMode] = useState<'direct' | 'proxy' | 'manual'>('direct')
  const [proxyPort, setProxyPort] = useState<number | null>(7897)
  const [proxyUsername, setProxyUsername] = useState('')
  const [proxyPassword, setProxyPassword] = useState('')
  const completedJobIDs = useRef<Set<string> | null>(null)
  const installWasRunning = useRef(false)
  const statusQuery = useQuery({
    queryKey: ['media-tools-status'],
    queryFn: api.mediaToolsStatus,
    retry: false,
    refetchInterval: (query) => query.state.data?.installing ? 500 : false,
  })
  const groupsQuery = useQuery({ queryKey: ['asset-groups', project.id], queryFn: () => api.assetGroups(project.id) })
  const jobsQuery = useQuery({
    queryKey: queryKeys.mediaJobs(project.id),
    queryFn: () => api.mediaJobs(project.id, { page: 1, page_size: 100 }),
    refetchInterval: (query) => (query.state.data?.items ?? []).some((job) => job.status === 'queued' || job.status === 'running') ? 1000 : false,
  })
  const jobs = useMemo(() => jobsQuery.data?.items ?? [], [jobsQuery.data?.items])
  const selectedJob = jobs.find((job) => job.id === selectedJobID) ?? jobs[0]
  const assets = selectedAssets
  const groups = useMemo(() => groupsQuery.data ?? [], [groupsQuery.data])
  const createJob = useMutation({
    mutationFn: () => api.createMediaJob(project.id, {
      tool,
      source_asset_ids: draft.sourceAssetIDs.filter(Boolean),
      target_asset_group_id: tool === 'inspect' ? undefined : draft.targetGroupID,
      output_name: draft.outputName.trim() || undefined,
      parameters: draft.parameters,
    }),
    onSuccess: async (job) => {
      queryClient.setQueryData<MediaJobPage>(queryKeys.mediaJobs(project.id), (current) => ({
        items: [job, ...(current?.items ?? []).filter((item) => item.id !== job.id)].slice(0, 100),
        total: Math.max(current?.total ?? 0, (current?.items.length ?? 0) + 1),
        page: 1,
        page_size: 100,
      }))
      setSelectedJobID(job.id)
      await queryClient.invalidateQueries({ queryKey: ['media-jobs', project.id] })
      message.success('本地处理任务已进入队列')
    },
    onError,
  })
  const cancelJob = useMutation({
    mutationFn: (id: string) => api.cancelMediaJob(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['media-jobs', project.id] }),
    onError,
  })
  const clearJobs = useMutation({
    mutationFn: () => api.clearCompletedMediaJobs(project.id),
    onSuccess: async ({ deleted }) => {
      setSelectedJobID(undefined)
      await queryClient.invalidateQueries({ queryKey: ['media-jobs', project.id] })
      message.success(deleted ? `已清理 ${deleted} 条任务记录` : '没有可清理的任务记录')
    },
    onError,
  })
  const installTools = useMutation({
    mutationFn: () => api.installMediaTools({
      mode: installMode === 'proxy' ? 'proxy' : 'direct',
      proxy_port: installMode === 'proxy' ? proxyPort ?? undefined : undefined,
      proxy_username: installMode === 'proxy' ? proxyUsername.trim() || undefined : undefined,
      proxy_password: installMode === 'proxy' ? proxyPassword || undefined : undefined,
    }),
    onSuccess: (status) => {
      queryClient.setQueryData(['media-tools-status'], status)
      setInstallOpen(false)
      message.info('FFmpeg 已开始后台下载，可以切换到其他页面')
    },
    onError: (error) => {
      void queryClient.invalidateQueries({ queryKey: ['media-tools-status'] })
      onError(error)
    },
  })
  const cancelInstall = useMutation({
    mutationFn: api.cancelMediaToolsInstall,
    onSuccess: (status) => queryClient.setQueryData(['media-tools-status'], status),
    onError,
  })
  const refreshToolchain = useMutation({
    mutationFn: api.refreshMediaTools,
    onSuccess: (status) => {
      queryClient.setQueryData(['media-tools-status'], status)
      if (status.available) {
        setInstallOpen(false)
        message.success('已检测到 FFmpeg')
      } else {
        message.warning('仍未检测到 FFmpeg，请检查文件名和放置目录')
      }
    },
    onError,
  })

  useEffect(() => {
    const transition = completedJobTransition(completedJobIDs.current, jobs, (job) => Boolean(job.output_asset_id || job.output_staged_asset_id))
    completedJobIDs.current = transition.current
    if (!transition.added.length) return
    void queryClient.invalidateQueries({ queryKey: ['assets', project.id] })
    void queryClient.invalidateQueries({ queryKey: ['staged-assets', project.id] })
    void queryClient.invalidateQueries({ queryKey: ['staged-summary', project.id] })
  }, [jobs, project.id, queryClient])

  useEffect(() => {
    const status = statusQuery.data
    if (!status) return
    if (status.installing) {
      installWasRunning.current = true
      return
    }
    if (!installWasRunning.current) return
    installWasRunning.current = false
    if (status.available) message.success(`FFmpeg ${status.install_version} 已安装`)
    else if (status.install_stage === 'canceled') message.info('FFmpeg 下载已取消')
    else if (status.install_stage === 'failed') message.error(status.message)
  }, [message, statusQuery.data])

  useEffect(() => {
    setDraft(emptyDraft(tool))
    setSelectedAssets([])
    setSelectedJobID(undefined)
  }, [tool, project.id])

  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['media-tools-status'] }),
    queryClient.invalidateQueries({ queryKey: ['media-jobs', project.id] }),
    queryClient.invalidateQueries({ queryKey: ['assets', project.id] }),
    queryClient.invalidateQueries({ queryKey: ['asset-groups', project.id] }),
  ])
  const activeJob = jobs.find((job) => job.status === 'running' || job.status === 'queued')
  const selectedSourceCount = draft.sourceAssetIDs.filter(Boolean).length
  const sourceCountValid = tool === 'merge' ? selectedSourceCount >= 2 : selectedSourceCount === 1
  const canRun = Boolean(statusQuery.data?.available && sourceCountValid)
  const updateParameters = (patch: Record<string, unknown>) => setDraft((current) => ({ ...current, parameters: { ...current.parameters, ...patch } }))
  const switchTool = (next: ActiveMediaTool) => setTool(next)

  return <div className="page page-local-tools">
    <FloatingToolbar ariaLabel="本地工具栏" items={[
      { key: 'run', label: `开始${toolMeta[tool].label}`, icon: <PlayCircleOutlined/>, active: true, disabled: !canRun, loading: createJob.isPending, onClick: () => createJob.mutate() },
      { key: 'cancel', label: '取消当前任务', icon: <StopOutlined/>, disabled: !activeJob, loading: cancelJob.isPending, onClick: () => activeJob && cancelJob.mutate(activeJob.id) },
      { key: 'refresh', label: '刷新', icon: <ReloadOutlined/>, loading: jobsQuery.isFetching || statusQuery.isFetching, onClick: () => void refresh() },
      { key: 'clear', label: '清理已结束任务', icon: <DeleteOutlined/>, disabled: !jobs.some((job) => !['queued', 'running'].includes(job.status)), loading: clearJobs.isPending, onClick: () => clearJobs.mutate() },
    ]}/>

    <section className="local-tools-content">
      <div aria-label="本地媒体工具" className="local-tool-tabs" role="tablist">
        {toolOrder.map((item) => <button aria-selected={tool === item} className={tool === item ? 'active' : ''} key={item} onClick={() => switchTool(item)} role="tab" type="button">
          {toolMeta[item].icon}<strong>{toolMeta[item].label}</strong>
        </button>)}
      </div>

      <div className="local-tool-workbench">
        <header>
          <h2>{toolMeta[tool].label}</h2>
          <div className={`local-tool-runtime${statusQuery.data?.available ? ' ready' : ' unavailable'}`} title={statusQuery.data?.version || statusQuery.data?.message}>
            <div className="local-tool-runtime-summary">
              <span>{statusQuery.isLoading ? '正在检查 FFmpeg' : statusQuery.data?.message || 'FFmpeg 状态未知'}</span>
              <i/>
              <small>素材按页搜索，不预加载全部资产</small>
              {!statusQuery.data?.available && !statusQuery.data?.installing && statusQuery.data?.install_supported && <Button icon={<DownloadOutlined/>} onClick={() => setInstallOpen(true)} size="small" type="link">
                {`安装 FFmpeg ${statusQuery.data.install_version} · ${formatBytes(statusQuery.data.download_bytes)}`}
              </Button>}
            </div>
            {statusQuery.data?.installing && <div className="ffmpeg-install-progress">
              <Progress percent={Math.round(statusQuery.data.install_progress * 100)} showInfo size="small" status={statusQuery.data.install_stage === 'canceling' ? 'exception' : 'active'}/>
              <div>
                <span>{installStageLabel(statusQuery.data.install_stage)} · {formatBytes(statusQuery.data.downloaded_bytes)} / {formatBytes(statusQuery.data.download_bytes)}</span>
                <small>{downloadProgressDetail(statusQuery.data.download_speed_bytes, statusQuery.data.eta_seconds, statusQuery.data.install_mode)}</small>
              </div>
              {statusQuery.data.can_cancel && <Button danger loading={cancelInstall.isPending} onClick={() => cancelInstall.mutate()} size="small">取消下载</Button>}
            </div>}
          </div>
        </header>

        <div className={`local-tool-fields${tool === 'merge' ? ' merge-fields' : tool === 'trim' ? ' trim-fields' : ''}`}>
          {tool === 'merge'
            ? <ToolField label="源视频"><AssetSourcePicker groups={groups} multiple onChange={(sourceAssetIDs, nextAssets) => { setDraft((current) => ({ ...current, sourceAssetIDs })); setSelectedAssets(nextAssets) }} projectID={project.id} selectedAssets={selectedAssets} tool={tool} value={draft.sourceAssetIDs}/></ToolField>
            : <ToolField label="源资产"><AssetSourcePicker groups={groups} onChange={(sourceAssetIDs, nextAssets) => { setDraft((current) => ({ ...current, sourceAssetIDs })); setSelectedAssets(nextAssets) }} projectID={project.id} selectedAssets={selectedAssets} tool={tool} value={draft.sourceAssetIDs}/></ToolField>}
          {tool !== 'inspect' && <ToolField label="输出位置"><Select onChange={(destination) => setDraft((current) => ({ ...current, targetGroupID: destination === STAGING_DESTINATION ? undefined : destination }))} optionFilterProp="label" options={[{ value: STAGING_DESTINATION, label: '未处理暂存区 · 稍后手动入库' }, ...groupOptions(groups)]} showSearch value={draft.targetGroupID ?? STAGING_DESTINATION}/></ToolField>}
          {tool !== 'inspect' && <ToolField label="输出名称（可选）"><Input maxLength={160} onChange={(event) => setDraft((current) => ({ ...current, outputName: event.target.value }))} placeholder="未填写时根据源文件命名" value={draft.outputName}/></ToolField>}
          {tool === 'merge' && <ToolField label="快速无损合片"><div className="local-tool-switch"><Switch checked={Boolean(draft.parameters.fast)} onChange={(fast) => updateParameters({ fast })}/><span>编码一致时开启</span></div></ToolField>}
          {tool === 'trim' && <ToolField label="快速无损裁切"><div className="local-tool-switch"><Switch checked={Boolean(draft.parameters.fast)} onChange={(fast) => updateParameters({ fast })}/><span>切点可能受关键帧影响</span></div></ToolField>}
        </div>

        <ToolParameters parameters={draft.parameters} tool={tool} update={updateParameters}/>

        {tool === 'merge' && draft.sourceAssetIDs.length > 0 && <MergeOrderList assets={assets} ids={draft.sourceAssetIDs} onChange={(sourceAssetIDs) => setDraft((current) => ({ ...current, sourceAssetIDs }))}/>}
        {(tool === 'trim' || tool === 'screenshot') && draft.sourceAssetIDs[0] && <VideoTimeline asset={assets.find((item) => item.id === draft.sourceAssetIDs[0])} mode={tool} parameters={draft.parameters} update={updateParameters}/>}
      </div>

      <MediaTaskCenter assets={assets} jobs={jobs} onSelect={setSelectedJobID} selectedJob={selectedJob}/>
    </section>

    <Modal
      cancelText="关闭"
      confirmLoading={installTools.isPending || refreshToolchain.isPending}
      okButtonProps={{ disabled: installMode === 'proxy' && !proxyPort }}
      okText={installMode === 'manual' ? '重新检测' : '开始下载'}
      onCancel={() => setInstallOpen(false)}
      onOk={() => installMode === 'manual' ? refreshToolchain.mutate() : installTools.mutate()}
      open={installOpen}
      title={`安装 FFmpeg ${statusQuery.data?.install_version ?? ''}`}
      width={680}
    >
      <Radio.Group
        buttonStyle="solid"
        onChange={(event) => setInstallMode(event.target.value)}
        optionType="button"
        options={[
          { label: '直接下载', value: 'direct' },
          { label: '代理下载', value: 'proxy' },
          { label: '手动下载', value: 'manual' },
        ]}
        value={installMode}
      />
      {installMode === 'direct' && <div className="ffmpeg-install-option">
        <Alert message="直接连接固定版本下载源，下载完成后会自动校验 SHA-256 并解压。" showIcon type="info"/>
      </div>}
      {installMode === 'proxy' && <div className="ffmpeg-install-option">
        <Alert message="通过本机 HTTP 代理 127.0.0.1 下载；代理信息只用于本次任务，不会保存。" showIcon type="info"/>
        <div className="ffmpeg-proxy-fields">
          <label><span>代理端口</span><InputNumber max={65535} min={1} onChange={setProxyPort} precision={0} value={proxyPort}/></label>
          <label><span>账户（可选）</span><Input autoComplete="off" onChange={(event) => setProxyUsername(event.target.value)} value={proxyUsername}/></label>
          <label><span>密码（可选）</span><Input.Password autoComplete="new-password" onChange={(event) => setProxyPassword(event.target.value)} value={proxyPassword}/></label>
        </div>
      </div>}
      {installMode === 'manual' && <div className="ffmpeg-install-option ffmpeg-manual-install">
        <Alert message="下载适合当前系统架构的固定压缩包，解压后只需保留 FFmpeg 与 FFprobe。" showIcon type="warning"/>
        {(statusQuery.data?.manual_downloads ?? []).map((asset) => <div className="ffmpeg-manual-asset" key={asset.url}>
          <div><strong>{asset.name}</strong><small>{formatBytes(asset.size)}</small></div>
          <Button href={asset.url} icon={<DownloadOutlined/>} target="_blank">打开下载链接</Button>
          <code>SHA-256: {asset.sha256}</code>
        </div>)}
        <ol>
          <li>解压下载的压缩包。</li>
          <li>找到并复制 {(statusQuery.data?.manual_files ?? []).join(' 和 ')}。</li>
          <li>将这两个文件放入：<code>{statusQuery.data?.install_directory}</code></li>
          <li>回到这里点击“重新检测”，无需重启 SagaFlow。</li>
        </ol>
      </div>}
    </Modal>
  </div>
}

function ToolField({ children, className = '', label }: { children: React.ReactNode; className?: string; label: string }) {
  return <label className={className}><span>{label}</span>{children}</label>
}

function ToolParameters({ parameters, tool, update }: { parameters: Record<string, unknown>; tool: ActiveMediaTool; update: (patch: Record<string, unknown>) => void }) {
  if (tool === 'inspect' || tool === 'merge' || tool === 'trim') return null
  return <div className="local-tool-parameters">
    {tool === 'transcode' && <ToolField label="输出格式"><Select onChange={(format) => update({ format })} options={[
      { value: 'mp4', label: 'MP4 · H.264 / AAC' }, { value: 'webm', label: 'WebM · VP9 / Opus' },
      { value: 'mp3', label: 'MP3 音频' }, { value: 'wav', label: 'WAV 无损音频' }, { value: 'm4a', label: 'M4A · AAC' },
    ]} value={String(parameters.format)}/></ToolField>}
    {tool === 'audio' && <ToolField label="处理方式"><Select onChange={(audio_mode) => update({ audio_mode })} options={[
      { value: 'extract', label: '提取为 MP3' }, { value: 'normalize', label: '响度标准化为 WAV' }, { value: 'mute', label: '移除视频音轨' },
    ]} value={String(parameters.audio_mode)}/></ToolField>}
    {tool === 'screenshot' && <>
      <ToolField label="图片格式"><Select onChange={(image_format) => update({ image_format })} options={[{ value: 'png', label: 'PNG' }, { value: 'jpg', label: 'JPG' }]} value={String(parameters.image_format)}/></ToolField>
      <ToolField label="输出宽度"><InputNumber max={7680} min={16} onChange={(output_width) => update({ output_width: output_width ?? 0 })} placeholder="保持原宽度" value={Number(parameters.output_width) || null}/></ToolField>
      <ToolField label="输出高度"><InputNumber max={7680} min={16} onChange={(output_height) => update({ output_height: output_height ?? 0 })} placeholder="保持原高度" value={Number(parameters.output_height) || null}/></ToolField>
      <ToolField label="裁剪左边距"><InputNumber min={0} onChange={(crop_x) => update({ crop_x: crop_x ?? 0 })} value={Number(parameters.crop_x)}/></ToolField>
      <ToolField label="裁剪上边距"><InputNumber min={0} onChange={(crop_y) => update({ crop_y: crop_y ?? 0 })} value={Number(parameters.crop_y)}/></ToolField>
      <ToolField label="裁剪尺寸"><div className="local-tool-size-pair"><InputNumber min={0} onChange={(crop_width) => update({ crop_width: crop_width ?? 0 })} placeholder="宽度" value={Number(parameters.crop_width) || null}/><span>×</span><InputNumber min={0} onChange={(crop_height) => update({ crop_height: crop_height ?? 0 })} placeholder="高度" value={Number(parameters.crop_height) || null}/></div></ToolField>
    </>}
  </div>
}

function AssetSourcePicker({ groups, multiple = false, onChange, projectID, selectedAssets, tool, value }: {
  groups: AssetGroup[]
  multiple?: boolean
  onChange: (ids: string[], assets: Asset[]) => void
  projectID: string
  selectedAssets: Asset[]
  tool: ActiveMediaTool
  value: string[]
}) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState<string[]>([])
  const [pendingAssets, setPendingAssets] = useState<Record<string, Asset>>({})
  const [query, setQuery] = useState('')
  const [groupID, setGroupID] = useState<string>()
  const [page, setPage] = useState(1)
  const deferredQuery = useDeferredValue(query.trim())
  const pageSize = 24
  const paths = useMemo(() => assetGroupPaths(groups), [groups])
  const pickerQuery = useQuery({
    queryKey: ['assets', 'picker', projectID, { tool, groupID, name: deferredQuery, page, pageSize }],
    queryFn: () => api.assetPage(projectID, {
      group_id: groupID,
      exclude_status: 'discarded',
      media_types: acceptedMediaTypes(tool).join(','),
      name: deferredQuery || undefined,
      page,
      page_size: pageSize,
    }),
    enabled: open,
    placeholderData: (previous) => previous,
  })
  const pageAssets = pickerQuery.data?.items ?? []
  const total = pickerQuery.data?.total ?? 0
  useEffect(() => setPage(1), [groupID, query])
  const show = () => {
    setPending(value)
    setPendingAssets(Object.fromEntries(selectedAssets.map((asset) => [asset.id, asset])))
    setQuery('')
    setGroupID(undefined)
    setPage(1)
    setOpen(true)
  }
  const toggle = (asset: Asset) => {
    setPending((current) => {
      if (!multiple) return [asset.id]
      return current.includes(asset.id) ? current.filter((item) => item !== asset.id) : [...current, asset.id]
    })
    setPendingAssets((current) => {
      const next = { ...current }
      if (multiple && pending.includes(asset.id)) delete next[asset.id]
      else next[asset.id] = asset
      return next
    })
  }
  const selected = value.map((id) => selectedAssets.find((asset) => asset.id === id)).filter((asset): asset is Asset => Boolean(asset))
  const confirmDisabled = multiple ? pending.length < 2 : pending.length !== 1
  return <>
    <button className={`asset-source-trigger${selected.length ? ' selected' : ''}`} onClick={show} type="button">
      <span className="asset-source-trigger-icon">{selected[0] ? mediaIcon(selected[0].media_type) : <SearchOutlined/>}</span>
      <span>
        <strong>{selected.length ? (multiple ? `已选择 ${selected.length} 个视频` : selected[0].name) : (multiple ? '从资产库选择视频' : '从资产库选择')}</strong>
        <small>{selected.length ? (multiple ? '打开后可以继续搜索、增删和排序' : `${mediaLabel(selected[0].media_type)} · ${formatBytes(selected[0].file_size_bytes)}`) : '支持按名称和分组分页搜索'}</small>
      </span>
    </button>
    <Modal cancelText="取消" okButtonProps={{ disabled: confirmDisabled }} okText={multiple ? '确认选择' : '使用这个资产'} onCancel={() => setOpen(false)} onOk={() => { onChange(pending, pending.map((id) => pendingAssets[id]).filter((asset): asset is Asset => Boolean(asset))); setOpen(false) }} open={open} title={multiple ? '选择参与合片的视频' : '选择源资产'} width={980}>
      <div className="asset-source-picker-toolbar">
        <Input allowClear prefix={<SearchOutlined/>} onChange={(event) => setQuery(event.target.value)} placeholder="搜索资产名称" value={query}/>
        <Select allowClear onChange={setGroupID} optionFilterProp="label" options={groupOptions(groups)} placeholder="全部分组" showSearch value={groupID}/>
        <span>{total} 个结果{pending.length ? ` · 已选 ${pending.length}` : ''}</span>
      </div>
      <div className="merge-picker-grid asset-source-picker-grid">
        {pageAssets.map((asset) => {
          const order = pending.indexOf(asset.id)
          return <button aria-pressed={order >= 0} className={order >= 0 ? 'selected' : ''} key={asset.id} onClick={() => toggle(asset)} type="button">
            <AssetPickerPreview asset={asset}/>
            {order >= 0 && <i>{multiple ? order + 1 : '✓'}</i>}
            <span><strong>{asset.name}</strong><small>{paths.get(asset.group_id ?? '') || mediaLabel(asset.media_type)} · {formatBytes(asset.file_size_bytes)}</small></span>
          </button>
        })}
        {!pickerQuery.isLoading && !pageAssets.length && <div className="merge-picker-empty">当前工具没有匹配的可用资产</div>}
      </div>
      {total > pageSize && <Pagination current={page} onChange={setPage} pageSize={pageSize} showSizeChanger={false} total={total}/>}
    </Modal>
  </>
}

function AssetPickerPreview({ asset }: { asset: Asset }) {
  if (asset.media_type === 'image') return <img alt="" loading="lazy" src={api.assetURL(asset.id)}/>
  if (asset.media_type === 'video') return <video muted preload="metadata" src={api.assetURL(asset.id)}/>
  return <div className="asset-source-picker-audio"><AudioOutlined/><span>音频</span></div>
}

function MergeOrderList({ assets, ids, onChange }: { assets: Asset[]; ids: string[]; onChange: (ids: string[]) => void }) {
  const [dragging, setDragging] = useState<string>()
  const probes = useQueries({
    queries: ids.map((id) => ({
      queryKey: ['media-info', id],
      queryFn: () => api.mediaInfo(id),
      staleTime: Number.POSITIVE_INFINITY,
      retry: false,
    })),
  })
  const move = (targetID: string) => {
    if (!dragging || dragging === targetID) return
    const next = [...ids]
    const sourceIndex = next.indexOf(dragging)
    const targetIndex = next.indexOf(targetID)
    if (sourceIndex < 0 || targetIndex < 0) return
    next.splice(sourceIndex, 1)
    next.splice(targetIndex, 0, dragging)
    onChange(next)
  }
  return <div className="merge-order-list">
    <div className="merge-order-heading"><strong>合片顺序</strong><span>拖动行调整先后</span></div>
    {ids.map((id, index) => {
      const asset = assets.find((item) => item.id === id)
      if (!asset) return null
      return <div
        className={dragging === id ? 'dragging' : ''}
        draggable
        key={id}
        onDragEnd={() => setDragging(undefined)}
        onDragOver={(event) => event.preventDefault()}
        onDragStart={() => setDragging(id)}
        onDrop={() => move(id)}
      >
        <HolderOutlined/>
        <em>{String(index + 1).padStart(2, '0')}</em>
        <video muted preload="metadata" src={api.assetURL(asset.id)}/>
        <span><strong>{asset.name}</strong><small>{probeSummaryLine(probes[index]?.data, probes[index]?.isLoading)}</small></span>
        <small>{formatBytes(asset.file_size_bytes)}</small>
      </div>
    })}
  </div>
}

function VideoTimeline({ asset, mode, parameters, update }: { asset?: Asset; mode: 'trim' | 'screenshot'; parameters: Record<string, unknown>; update: (patch: Record<string, unknown>) => void }) {
  if (!asset) return null
  return mode === 'trim'
    ? <TrimTimeline asset={asset} parameters={parameters} update={update}/>
    : <ScreenshotTimeline asset={asset} parameters={parameters} update={update}/>
}

function ScreenshotTimeline({ asset, parameters, update }: { asset: Asset; parameters: Record<string, unknown>; update: (patch: Record<string, unknown>) => void }) {
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const [duration, setDuration] = useState(0)
  const [playhead, setPlayhead] = useState(Number(parameters.time_seconds ?? 0))
  const max = duration > 0 ? duration : 1
  const renderPreview = useCallback(() => {
    const video = videoRef.current
    const canvas = canvasRef.current
    if (!video || !canvas || !video.videoWidth || !video.videoHeight) return
    const cropEnabled = Number(parameters.crop_width) > 0 && Number(parameters.crop_height) > 0
    const sourceX = cropEnabled ? Math.min(Number(parameters.crop_x) || 0, Math.max(0, video.videoWidth - 2)) : 0
    const sourceY = cropEnabled ? Math.min(Number(parameters.crop_y) || 0, Math.max(0, video.videoHeight - 2)) : 0
    const sourceWidth = cropEnabled ? Math.min(Number(parameters.crop_width), video.videoWidth - sourceX) : video.videoWidth
    const sourceHeight = cropEnabled ? Math.min(Number(parameters.crop_height), video.videoHeight - sourceY) : video.videoHeight
    const requestedWidth = Number(parameters.output_width) || 0
    const requestedHeight = Number(parameters.output_height) || 0
    const targetRatio = requestedWidth > 0 && requestedHeight > 0
      ? requestedWidth / requestedHeight
      : sourceWidth / sourceHeight
    const previewWidth = Math.min(requestedWidth || sourceWidth, 960)
    const previewHeight = Math.max(1, Math.round(previewWidth / targetRatio))
    canvas.width = previewWidth
    canvas.height = previewHeight
    const context = canvas.getContext('2d')
    context?.clearRect(0, 0, previewWidth, previewHeight)
    context?.drawImage(video, sourceX, sourceY, sourceWidth, sourceHeight, 0, 0, previewWidth, previewHeight)
  }, [parameters.crop_height, parameters.crop_width, parameters.crop_x, parameters.crop_y, parameters.output_height, parameters.output_width])
  useEffect(() => renderPreview(), [renderPreview])
  const seek = (value: number) => {
    setPlayhead(value)
    if (videoRef.current) videoRef.current.currentTime = value
  }
  const loaded = () => {
    const nextDuration = videoRef.current?.duration ?? 0
    if (!Number.isFinite(nextDuration) || nextDuration <= 0) return
    setDuration(nextDuration)
    const next = Math.min(Number(parameters.time_seconds ?? 0), nextDuration)
    seek(next)
    update({ time_seconds: next })
  }
  const cropEnabled = Number(parameters.crop_width) > 0 && Number(parameters.crop_height) > 0
  const outputSize = Number(parameters.output_width) > 0 || Number(parameters.output_height) > 0
    ? `${Number(parameters.output_width) || '自动'} × ${Number(parameters.output_height) || '自动'}`
    : '保持裁剪后尺寸'
  return <div className="screenshot-timeline-editor">
    <div className="screenshot-preview-grid">
      <figure className="video-timeline-preview">
        <video
          controls
          onLoadedData={renderPreview}
          onLoadedMetadata={loaded}
          onPause={() => update({ time_seconds: playhead })}
          onSeeked={() => {
            const current = videoRef.current?.currentTime ?? 0
            setPlayhead(current)
            update({ time_seconds: current })
            renderPreview()
          }}
          onTimeUpdate={() => {
            const current = videoRef.current?.currentTime ?? 0
            setPlayhead(current)
            renderPreview()
          }}
          preload="metadata"
          ref={videoRef}
          src={api.assetURL(asset.id)}
        />
        <figcaption>源视频</figcaption>
      </figure>
      <figure className="screenshot-output-preview">
        <canvas ref={canvasRef}/>
        <figcaption><strong>截图预览</strong><span>{cropEnabled ? '已裁剪' : '完整画面'} · {outputSize}</span></figcaption>
      </figure>
    </div>
    <div className="video-timeline-track">
      <div><strong>截图位置</strong><span>{formatTimestamp(playhead)}</span></div>
      <Slider max={max} min={0} onChange={(value) => seek(value)} onChangeComplete={(value) => update({ time_seconds: value })} step={.001} tooltip={{ formatter: formatTimestamp }} value={Math.min(playhead, max)}/>
      <div className="video-timeline-scale"><span>00:00.000</span><span>{formatTimestamp(max)}</span></div>
    </div>
  </div>
}

function TrimTimeline({ asset, parameters, update }: { asset: Asset; parameters: Record<string, unknown>; update: (patch: Record<string, unknown>) => void }) {
  const [duration, setDuration] = useState(0)
  const durationRef = useRef(0)
  const max = duration > 0 ? duration : 1
  const start = Math.min(Number(parameters.start_seconds ?? 0), Math.max(0, max - .001))
  const end = Math.min(Math.max(start + Number(parameters.duration ?? 5), start + .001), max)
  const registerDuration = (nextDuration: number) => {
    if (!Number.isFinite(nextDuration) || nextDuration <= 0 || Math.abs(durationRef.current - nextDuration) < .001) return
    durationRef.current = nextDuration
    setDuration(nextDuration)
    const nextStart = Math.min(Number(parameters.start_seconds ?? 0), Math.max(0, nextDuration - .001))
    const nextEnd = Math.min(nextStart + Number(parameters.duration ?? 5), nextDuration)
    update({ start_seconds: nextStart, duration: Math.max(.001, nextEnd - nextStart) })
  }
  return <div className="video-trim-editor">
    <div className="trim-frame-pair">
      <FramePreview asset={asset} label="起点" onDuration={registerDuration} time={start}/>
      <FramePreview asset={asset} label="终点" onDuration={registerDuration} time={end}/>
    </div>
    <div className="video-timeline-track">
      <div><strong>保留片段</strong><span>{formatTimestamp(start)} — {formatTimestamp(end)}</span></div>
      <Slider max={max} min={0} onChange={(value) => { const [nextStart, nextEnd] = value; update({ start_seconds: nextStart, duration: Math.max(.001, nextEnd - nextStart) }) }} range step={.001} tooltip={{ formatter: formatTimestamp }} value={[start, end]}/>
      <div className="video-timeline-scale"><span>00:00.000</span><span>{formatTimestamp(max)}</span></div>
    </div>
  </div>
}

function FramePreview({ asset, label, onDuration, time }: { asset: Asset; label: string; onDuration: (duration: number) => void; time: number }) {
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const seek = () => {
    const video = videoRef.current
    if (!video || !Number.isFinite(video.duration)) return
    const target = Math.min(Math.max(0, time), Math.max(0, video.duration - .001))
    if (Math.abs(video.currentTime - target) >= .001) video.currentTime = target
  }
  useEffect(() => {
    videoRef.current?.load()
  }, [asset.id])
  useEffect(() => {
    const video = videoRef.current
    if (!video || !Number.isFinite(video.duration)) return
    const target = Math.min(Math.max(0, time), Math.max(0, video.duration - .001))
    if (Math.abs(video.currentTime - target) >= .001) video.currentTime = target
  }, [time])
  return <figure className="trim-frame-preview">
    <video
      aria-hidden
      muted
      onLoadedData={seek}
      onLoadedMetadata={() => {
        const duration = videoRef.current?.duration ?? 0
        onDuration(duration)
        seek()
      }}
      preload="auto"
      ref={videoRef}
      src={api.assetURL(asset.id)}
    />
    <figcaption><strong>{label}</strong><span>{formatTimestamp(time)}</span></figcaption>
  </figure>
}

function MediaTaskCenter({ assets, jobs, onSelect, selectedJob }: { assets: Asset[]; jobs: MediaJob[]; onSelect: (id: string) => void; selectedJob?: MediaJob }) {
  return <section className="local-media-tasks">
    <div className="local-media-task-heading"><strong>处理任务</strong><span>{jobs.length ? `${jobs.length} 条记录` : '还没有本地处理任务'}</span></div>
    {jobs.length > 0 && <div className="local-media-task-list">
      {jobs.map((job) => <button className={selectedJob?.id === job.id ? 'active' : ''} key={job.id} onClick={() => onSelect(job.id)} type="button">
        <i className={job.status}/><span><strong>{mediaToolLabel(job.tool)} · {job.output_name || sourceNames(job, assets)}</strong><small>{statusLabel(job.status)} · {new Date(job.created_at).toLocaleString()}</small></span>
      </button>)}
    </div>}
    <MediaTaskDetail assets={assets} job={selectedJob}/>
  </section>
}

function MediaTaskDetail({ assets, job }: { assets: Asset[]; job?: MediaJob }) {
  if (!job) return <div className="local-media-progress idle"><div><strong>任务进度</strong><span>提交任务后启用</span></div><Progress percent={0} showInfo={false}/></div>
  const running = job.status === 'queued' || job.status === 'running'
  return <div className={`local-media-progress${running ? ' running' : ''}`}>
    <div><strong>{mediaToolLabel(job.tool)} · {sourceNames(job, assets)}</strong><span>{stageLabel(job.stage)}</span></div>
    <Progress percent={Math.round(job.progress * 100)} status={job.status === 'failed' ? 'exception' : job.status === 'succeeded' ? 'success' : 'active'}/>
    {job.error_message && <p className="local-media-error">{job.error_message}</p>}
    {job.tool === 'inspect' && job.status === 'succeeded' && <ProbeSummary value={job.probe_snapshot}/>}
    {job.output_asset_id && <Button icon={<PlayCircleOutlined/>} onClick={() => window.open(api.assetURL(job.output_asset_id!), '_blank')}>查看输出资产</Button>}
    {job.output_staged_asset_id && <Button icon={<PlayCircleOutlined/>} onClick={() => window.open(api.stagedAssetURL(job.output_staged_asset_id!), '_blank')}>查看暂存结果</Button>}
  </div>
}

function ProbeSummary({ value }: { value: Record<string, unknown> }) {
  const format = (value.format ?? {}) as Record<string, unknown>
  const streams = Array.isArray(value.streams) ? value.streams as Array<Record<string, unknown>> : []
  const video = streams.find((stream) => stream.codec_type === 'video')
  const audio = streams.find((stream) => stream.codec_type === 'audio')
  return <div className="local-probe-summary">
    <div><span>容器</span><strong>{String(format.format_long_name ?? format.format_name ?? '—')}</strong></div>
    <div><span>时长</span><strong>{formatDuration(Number(format.duration ?? 0))}</strong></div>
    <div><span>画面</span><strong>{video ? `${video.width ?? '—'} × ${video.height ?? '—'} · ${video.codec_name ?? '—'}` : '无视频流'}</strong></div>
    <div><span>音频</span><strong>{audio ? `${audio.codec_name ?? '—'} · ${audio.sample_rate ?? '—'} Hz` : '无音频流'}</strong></div>
  </div>
}

function acceptedMediaTypes(tool: MediaTool): Asset['media_type'][] {
  if (tool === 'inspect') return ['image', 'audio', 'video']
  if (tool === 'transcode' || tool === 'audio') return ['audio', 'video']
  return ['video']
}

function groupOptions(groups: AssetGroup[]) {
  const paths = assetGroupPaths(groups)
  return groups.map((group) => ({ value: group.id, label: `${kindLabel(group.kind)} · ${paths.get(group.id)}` }))
}

function assetGroupPaths(groups: AssetGroup[]) {
  const byID = new Map(groups.map((group) => [group.id, group]))
  const paths = new Map<string, string>()
  for (const group of groups) {
    const names = [group.name]
    let parentID = group.parent_id
    const visited = new Set<string>()
    while (parentID && !visited.has(parentID)) {
      visited.add(parentID)
      const parent = byID.get(parentID)
      if (!parent) break
      names.unshift(parent.name)
      parentID = parent.parent_id
    }
    paths.set(group.id, names.join(' / '))
  }
  return paths
}

function mediaLabel(mediaType: Asset['media_type']) {
  return ({ image: '图像', audio: '音频', video: '视频', text: '文本', file: '文件' })[mediaType]
}

function mediaIcon(mediaType: Asset['media_type']) {
  if (mediaType === 'audio') return <AudioOutlined/>
  if (mediaType === 'video') return <PlayCircleOutlined/>
  return <PictureOutlined/>
}

function sourceNames(job: MediaJob, assets: Asset[]) {
  const names = (job.source_asset_ids ?? []).map((id) => assets.find((asset) => asset.id === id)?.name).filter(Boolean)
  return names.join(' + ') || '源资产'
}

function kindLabel(kind: AssetGroup['kind']) {
  return ({ character: '人物', scene: '场景', prop: '道具', material: '素材' })[kind]
}

function mediaToolLabel(tool: MediaTool) {
  return toolMeta[tool].label
}

function statusLabel(status: MediaJob['status']) {
  return ({ queued: '排队', running: '处理中', succeeded: '已完成', failed: '失败', canceled: '已取消', interrupted: '已中断' })[status]
}

function stageLabel(stage: string) {
  return ({ queued: '等待处理', preparing: '准备文件', probing: '读取媒体信息', processing: 'FFmpeg 处理', storing: '存入本地资产', completed: '处理完成', failed: '处理失败', canceled: '已取消', interrupted: '已中断' } as Record<string, string>)[stage] ?? stage
}

function installStageLabel(stage: string) {
  return ({
    connecting: '正在连接',
    downloading: '正在下载',
    retrying: '正在重试',
    extracting: '正在解压',
    verifying: '正在校验',
    canceling: '正在取消',
  } as Record<string, string>)[stage] ?? '正在准备'
}

function downloadProgressDetail(speed: number, etaSeconds: number, mode: string) {
  const via = mode === 'proxy' ? '代理下载' : '直接下载'
  if (speed <= 0) return `${via} · 可以切换页面，任务会在后台继续`
  const eta = etaSeconds > 0 ? ` · 约 ${formatDuration(etaSeconds)}` : ''
  return `${via} · ${formatBytes(speed)}/s${eta} · 可以切换页面`
}

function probeSummaryLine(value?: Record<string, unknown>, loading?: boolean) {
  if (loading) return '正在读取编码信息…'
  if (!value) return '无法读取编码信息'
  const streams = Array.isArray(value.streams) ? value.streams as Array<Record<string, unknown>> : []
  const video = streams.find((stream) => stream.codec_type === 'video')
  const audio = streams.find((stream) => stream.codec_type === 'audio')
  const details = [
    video && `${video.codec_name ?? '未知编码'} · ${video.width ?? '—'}×${video.height ?? '—'}${frameRate(video.avg_frame_rate)}`,
    audio && `${audio.codec_name ?? '未知音频'} · ${audio.sample_rate ?? '—'} Hz`,
  ].filter(Boolean)
  return details.join(' / ') || '未发现音视频流'
}

function frameRate(value: unknown) {
  if (typeof value !== 'string' || !value.includes('/')) return ''
  const [top, bottom] = value.split('/').map(Number)
  if (!top || !bottom) return ''
  return ` · ${(top / bottom).toFixed(2).replace(/\.00$/, '')} fps`
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MB`
  return `${(value / 1024 ** 3).toFixed(1)} GB`
}

function formatDuration(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const rest = Math.floor(seconds % 60)
  return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}` : `${minutes}:${String(rest).padStart(2, '0')}`
}

function formatTimestamp(value?: number) {
  const seconds = Number.isFinite(value) ? Math.max(0, value ?? 0) : 0
  const minutes = Math.floor(seconds / 60)
  const rest = seconds - minutes * 60
  return `${String(minutes).padStart(2, '0')}:${rest.toFixed(3).padStart(6, '0')}`
}
