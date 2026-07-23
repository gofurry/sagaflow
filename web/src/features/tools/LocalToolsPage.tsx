import { useEffect, useMemo, useRef, useState } from 'react'
import {
  AudioOutlined,
  CameraOutlined,
  CheckCircleOutlined,
  ColumnWidthOutlined,
  CompressOutlined,
  DeleteOutlined,
  FileSearchOutlined,
  HolderOutlined,
  MergeCellsOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  ScissorOutlined,
  StopOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { App, Button, Checkbox, Input, InputNumber, Modal, Progress, Select, Slider, Switch } from 'antd'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Asset, AssetGroup, MediaJob, MediaTool, Project } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'

interface Draft {
  sourceAssetIDs: string[]
  targetGroupID?: string
  outputName: string
  parameters: Record<string, unknown>
}

const toolOrder: MediaTool[] = ['inspect', 'transcode', 'aspect', 'audio', 'trim', 'merge', 'screenshot']
const STAGING_DESTINATION = '__staging__'
const toolMeta: Record<MediaTool, { label: string; icon: React.ReactNode }> = {
  inspect: { label: '检查', icon: <FileSearchOutlined/> },
  transcode: { label: '转换', icon: <CompressOutlined/> },
  aspect: { label: '画幅', icon: <ColumnWidthOutlined/> },
  audio: { label: '音频', icon: <AudioOutlined/> },
  trim: { label: '裁切', icon: <ScissorOutlined/> },
  merge: { label: '合片', icon: <MergeCellsOutlined/> },
  screenshot: { label: '截图', icon: <CameraOutlined/> },
}

const defaultParameters: Record<MediaTool, Record<string, unknown>> = {
  inspect: {},
  transcode: { format: 'mp4' },
  aspect: { width: 1920, height: 1080, fit: 'contain', background: '#F5EDE2' },
  audio: { audio_mode: 'extract' },
  trim: { start_seconds: 0, duration: 5, fast: false },
  merge: { fast: true },
  screenshot: { time_seconds: 0, image_format: 'png' },
}

const emptyDraft = (tool: MediaTool): Draft => ({
  sourceAssetIDs: [],
  outputName: '',
  parameters: { ...defaultParameters[tool] },
})

export function LocalToolsPage({ onError, project }: { onError: (error: unknown) => void; project: Project }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [tool, setTool] = useState<MediaTool>('inspect')
  const [draft, setDraft] = useState<Draft>(() => emptyDraft('inspect'))
  const [selectedJobID, setSelectedJobID] = useState<string>()
  const statusQuery = useQuery({ queryKey: ['media-tools-status'], queryFn: api.mediaToolsStatus, retry: false })
  const assetsQuery = useQuery({ queryKey: ['assets', project.id], queryFn: () => api.assets(project.id) })
  const groupsQuery = useQuery({ queryKey: ['asset-groups', project.id], queryFn: () => api.assetGroups(project.id) })
  const jobsQuery = useQuery({
    queryKey: ['media-jobs', project.id],
    queryFn: () => api.mediaJobs(project.id),
    refetchInterval: (query) => (query.state.data ?? []).some((job) => job.status === 'queued' || job.status === 'running') ? 1000 : false,
  })
  const jobs = useMemo(() => jobsQuery.data ?? [], [jobsQuery.data])
  const selectedJob = jobs.find((job) => job.id === selectedJobID) ?? jobs[0]
  const assets = useMemo(() => (assetsQuery.data ?? []).filter((asset) => asset.status !== 'discarded'), [assetsQuery.data])
  const groups = useMemo(() => groupsQuery.data ?? [], [groupsQuery.data])
  const eligibleAssets = useMemo(() => assets.filter((asset) => acceptsAsset(tool, asset)), [assets, tool])
  const createJob = useMutation({
    mutationFn: () => api.createMediaJob(project.id, {
      tool,
      source_asset_ids: draft.sourceAssetIDs.filter(Boolean),
      target_asset_group_id: tool === 'inspect' ? undefined : draft.targetGroupID,
      output_name: draft.outputName.trim() || undefined,
      parameters: draft.parameters,
    }),
    onSuccess: async (job) => {
      queryClient.setQueryData<MediaJob[]>(['media-jobs', project.id], (current = []) => [job, ...current.filter((item) => item.id !== job.id)])
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

  useEffect(() => {
    const completed = jobs.some((job) => job.status === 'succeeded' && (job.output_asset_id || job.output_staged_asset_id))
    if (completed) {
      void queryClient.invalidateQueries({ queryKey: ['assets', project.id] })
      void queryClient.invalidateQueries({ queryKey: ['staged-assets', project.id] })
      void queryClient.invalidateQueries({ queryKey: ['staged-summary', project.id] })
    }
  }, [jobs, project.id, queryClient])

  useEffect(() => {
    setDraft(emptyDraft(tool))
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
  const switchTool = (next: MediaTool) => setTool(next)

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

      <div className={`local-tool-runtime${statusQuery.data?.available ? ' ready' : ' missing'}`}>
        {statusQuery.data?.available ? <CheckCircleOutlined/> : <WarningOutlined/>}
        <strong>{statusQuery.data?.available ? 'FFmpeg 已就绪' : statusQuery.data?.message ?? '正在检查本地工具…'}</strong>
      </div>

      <div className="local-tool-workbench">
        <header>
          <h2>{toolMeta[tool].label}</h2>
          <span>{eligibleAssets.length} 个可用素材</span>
        </header>

        <div className={`local-tool-fields${tool === 'merge' ? ' merge-fields' : ''}`}>
          {tool === 'merge'
            ? <ToolField label="源视频"><MergeVideoPicker assets={eligibleAssets} onChange={(sourceAssetIDs) => setDraft((current) => ({ ...current, sourceAssetIDs }))} value={draft.sourceAssetIDs}/></ToolField>
            : <ToolField label="源资产">
              <Select
                allowClear
                onChange={(value) => setDraft((current) => ({ ...current, sourceAssetIDs: value ? [value] : [] }))}
                optionFilterProp="label"
                options={eligibleAssets.map(assetOption)}
                placeholder="选择本地资产"
                showSearch
                value={draft.sourceAssetIDs[0]}
              />
            </ToolField>}
          {tool !== 'inspect' && <ToolField label="输出位置"><Select onChange={(destination) => setDraft((current) => ({ ...current, targetGroupID: destination === STAGING_DESTINATION ? undefined : destination }))} optionFilterProp="label" options={[{ value: STAGING_DESTINATION, label: '未处理暂存区 · 稍后手动入库' }, ...groupOptions(groups)]} showSearch value={draft.targetGroupID ?? STAGING_DESTINATION}/></ToolField>}
          {tool !== 'inspect' && <ToolField label="输出名称（可选）"><Input maxLength={160} onChange={(event) => setDraft((current) => ({ ...current, outputName: event.target.value }))} placeholder="未填写时根据源文件命名" value={draft.outputName}/></ToolField>}
          {tool === 'merge' && <ToolField label="快速无损合片"><div className="local-tool-switch"><Switch checked={Boolean(draft.parameters.fast)} onChange={(fast) => updateParameters({ fast })}/><span>编码一致时开启</span></div></ToolField>}
        </div>

        <ToolParameters parameters={draft.parameters} tool={tool} update={updateParameters}/>

        {tool === 'merge' && draft.sourceAssetIDs.length > 0 && <MergeOrderList assets={assets} ids={draft.sourceAssetIDs} onChange={(sourceAssetIDs) => setDraft((current) => ({ ...current, sourceAssetIDs }))}/>}
        {(tool === 'trim' || tool === 'screenshot') && draft.sourceAssetIDs[0] && <VideoTimeline asset={assets.find((item) => item.id === draft.sourceAssetIDs[0])} mode={tool} parameters={draft.parameters} update={updateParameters}/>}
        {tool === 'aspect' && draft.sourceAssetIDs[0] && <AspectPreview asset={assets.find((item) => item.id === draft.sourceAssetIDs[0])} parameters={draft.parameters}/>}
        {!['merge', 'trim', 'screenshot', 'aspect'].includes(tool) && draft.sourceAssetIDs.length > 0 && <SourceStrip assets={assets} ids={draft.sourceAssetIDs}/>}
      </div>

      <MediaTaskCenter assets={assets} jobs={jobs} onSelect={setSelectedJobID} selectedJob={selectedJob}/>
    </section>
  </div>
}

function ToolField({ children, className = '', label }: { children: React.ReactNode; className?: string; label: string }) {
  return <label className={className}><span>{label}</span>{children}</label>
}

function ToolParameters({ parameters, tool, update }: { parameters: Record<string, unknown>; tool: MediaTool; update: (patch: Record<string, unknown>) => void }) {
  if (tool === 'inspect' || tool === 'merge') return null
  return <div className="local-tool-parameters">
    {tool === 'transcode' && <ToolField label="输出格式"><Select onChange={(format) => update({ format })} options={[
      { value: 'mp4', label: 'MP4 · H.264 / AAC' }, { value: 'webm', label: 'WebM · VP9 / Opus' },
      { value: 'mp3', label: 'MP3 音频' }, { value: 'wav', label: 'WAV 无损音频' }, { value: 'm4a', label: 'M4A · AAC' },
    ]} value={String(parameters.format)}/></ToolField>}
    {tool === 'aspect' && <>
      <ToolField label="宽度"><InputNumber max={7680} min={64} onChange={(width) => update({ width: width ?? 1920 })} value={Number(parameters.width)}/></ToolField>
      <ToolField label="高度"><InputNumber max={4320} min={64} onChange={(height) => update({ height: height ?? 1080 })} value={Number(parameters.height)}/></ToolField>
      <ToolField label="适配方式"><Select onChange={(fit) => update({ fit })} options={[{ value: 'contain', label: '完整显示 · 留边' }, { value: 'cover', label: '裁切铺满' }]} value={String(parameters.fit)}/></ToolField>
      {parameters.fit === 'contain' && <ToolField label="留边颜色"><input aria-label="选择留边颜色" className="local-tool-color-swatch" onChange={(event) => update({ background: event.target.value })} type="color" value={String(parameters.background)}/></ToolField>}
    </>}
    {tool === 'audio' && <ToolField label="处理方式"><Select onChange={(audio_mode) => update({ audio_mode })} options={[
      { value: 'extract', label: '提取为 MP3' }, { value: 'normalize', label: '响度标准化为 WAV' }, { value: 'mute', label: '移除视频音轨' },
    ]} value={String(parameters.audio_mode)}/></ToolField>}
    {tool === 'trim' && <ToolField label="快速无损裁切"><div className="local-tool-switch"><Switch checked={Boolean(parameters.fast)} onChange={(fast) => update({ fast })}/><span>切点可能受关键帧影响</span></div></ToolField>}
    {tool === 'screenshot' && <ToolField label="图片格式"><Select onChange={(image_format) => update({ image_format })} options={[{ value: 'png', label: 'PNG' }, { value: 'jpg', label: 'JPG' }]} value={String(parameters.image_format)}/></ToolField>}
  </div>
}

function MergeVideoPicker({ assets, onChange, value }: { assets: Asset[]; onChange: (ids: string[]) => void; value: string[] }) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState<string[]>([])
  const show = () => {
    setPending(value)
    setOpen(true)
  }
  const toggle = (id: string, checked: boolean) => setPending((current) => checked ? [...current, id] : current.filter((item) => item !== id))
  return <>
    <Button block onClick={show}>{value.length ? `已选择 ${value.length} 个视频` : '选择视频'}</Button>
    <Modal cancelText="取消" okButtonProps={{ disabled: pending.length < 2 }} okText="确认顺序" onCancel={() => setOpen(false)} onOk={() => { onChange(pending); setOpen(false) }} open={open} title="选择参与合片的视频" width={860}>
      <div className="merge-picker-grid">
        {assets.map((asset) => {
          const order = pending.indexOf(asset.id)
          return <label className={order >= 0 ? 'selected' : ''} key={asset.id}>
            <video muted preload="metadata" src={api.assetURL(asset.id)}/>
            {order >= 0 && <i>{order + 1}</i>}
            <span><strong>{asset.name}</strong><small>{formatBytes(asset.file_size_bytes)}</small></span>
            <Checkbox checked={order >= 0} onChange={(event) => toggle(asset.id, event.target.checked)}/>
          </label>
        })}
        {!assets.length && <div className="merge-picker-empty">资产库中还没有视频</div>}
      </div>
    </Modal>
  </>
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
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const [duration, setDuration] = useState(0)
  const [playhead, setPlayhead] = useState(Number(parameters.time_seconds ?? parameters.start_seconds ?? 0))
  if (!asset) return null
  const max = duration > 0 ? duration : 1
  const start = Math.min(Number(parameters.start_seconds ?? 0), max)
  const end = Math.min(Math.max(start + Number(parameters.duration ?? 5), start + .001), max)
  const seek = (value: number) => {
    setPlayhead(value)
    if (videoRef.current) videoRef.current.currentTime = value
  }
  const loaded = () => {
    const nextDuration = videoRef.current?.duration ?? 0
    if (!Number.isFinite(nextDuration) || nextDuration <= 0) return
    setDuration(nextDuration)
    if (mode === 'screenshot') {
      const next = Math.min(Number(parameters.time_seconds ?? 0), nextDuration)
      seek(next)
      update({ time_seconds: next })
    } else {
      const nextStart = Math.min(Number(parameters.start_seconds ?? 0), Math.max(0, nextDuration - .001))
      const nextEnd = Math.min(nextStart + Number(parameters.duration ?? 5), nextDuration)
      update({ start_seconds: nextStart, duration: Math.max(.001, nextEnd - nextStart) })
      seek(nextStart)
    }
  }
  return <div className="video-timeline-editor">
    <div className="video-timeline-preview">
      <video
        controls
        onLoadedMetadata={loaded}
        onPause={() => mode === 'screenshot' && update({ time_seconds: playhead })}
        onSeeked={() => {
          const current = videoRef.current?.currentTime ?? 0
          setPlayhead(current)
          if (mode === 'screenshot') update({ time_seconds: current })
        }}
        onTimeUpdate={() => {
          const current = videoRef.current?.currentTime ?? 0
          setPlayhead(current)
          if (mode === 'trim' && current > end) {
            videoRef.current?.pause()
            seek(start)
          }
        }}
        preload="metadata"
        ref={videoRef}
        src={api.assetURL(asset.id)}
      />
    </div>
    <div className="video-timeline-track">
      <div><strong>{mode === 'screenshot' ? '截图位置' : '保留片段'}</strong><span>{mode === 'screenshot' ? formatTimestamp(playhead) : `${formatTimestamp(start)} — ${formatTimestamp(end)}`}</span></div>
      {mode === 'screenshot'
        ? <Slider max={max} min={0} onChange={(value) => seek(value)} onChangeComplete={(value) => update({ time_seconds: value })} step={.001} tooltip={{ formatter: formatTimestamp }} value={Math.min(playhead, max)}/>
        : <Slider max={max} min={0} onChange={(value) => { const [nextStart, nextEnd] = value; update({ start_seconds: nextStart, duration: Math.max(.001, nextEnd - nextStart) }); seek(nextStart) }} range step={.001} tooltip={{ formatter: formatTimestamp }} value={[start, end]}/>}
      <div className="video-timeline-scale"><span>00:00.000</span><span>{formatTimestamp(max)}</span></div>
    </div>
  </div>
}

function AspectPreview({ asset, parameters }: { asset?: Asset; parameters: Record<string, unknown> }) {
  if (!asset) return null
  const width = Number(parameters.width ?? 1920)
  const height = Number(parameters.height ?? 1080)
  const fit = parameters.fit === 'cover' ? 'cover' : 'contain'
  return <div className="aspect-preview">
    <div><strong>输出预览</strong><span>{width} × {height}</span></div>
    <div className="aspect-preview-frame" style={{ aspectRatio: `${width} / ${height}`, backgroundColor: String(parameters.background ?? '#F5EDE2') }}>
      <video controls preload="metadata" src={api.assetURL(asset.id)} style={{ objectFit: fit }}/>
    </div>
  </div>
}

function SourceStrip({ assets, ids }: { assets: Asset[]; ids: string[] }) {
  return <div className="local-tool-sources">
    {ids.map((id, index) => {
      const asset = assets.find((item) => item.id === id)
      if (!asset) return null
      return <div key={id}><em>{String(index + 1).padStart(2, '0')}</em><span>{asset.name}</span><small>{asset.media_type} · {formatBytes(asset.file_size_bytes)}</small></div>
    })}
  </div>
}

function MediaTaskCenter({ assets, jobs, onSelect, selectedJob }: { assets: Asset[]; jobs: MediaJob[]; onSelect: (id: string) => void; selectedJob?: MediaJob }) {
  return <section className="local-media-tasks">
    <div className="local-media-task-heading"><strong>处理任务</strong><span>{jobs.length ? `${jobs.length} 条记录` : '还没有本地处理任务'}</span></div>
    {jobs.length > 0 && <div className="local-media-task-list">
      {jobs.map((job) => <button className={selectedJob?.id === job.id ? 'active' : ''} key={job.id} onClick={() => onSelect(job.id)} type="button">
        <i className={job.status}/><span><strong>{toolMeta[job.tool].label} · {job.output_name || sourceNames(job, assets)}</strong><small>{statusLabel(job.status)} · {new Date(job.created_at).toLocaleString()}</small></span>
      </button>)}
    </div>}
    <MediaTaskDetail assets={assets} job={selectedJob}/>
  </section>
}

function MediaTaskDetail({ assets, job }: { assets: Asset[]; job?: MediaJob }) {
  if (!job) return <div className="local-media-progress idle"><div><strong>任务进度</strong><span>提交任务后启用</span></div><Progress percent={0} showInfo={false}/></div>
  const running = job.status === 'queued' || job.status === 'running'
  return <div className={`local-media-progress${running ? ' running' : ''}`}>
    <div><strong>{toolMeta[job.tool].label} · {sourceNames(job, assets)}</strong><span>{stageLabel(job.stage)}</span></div>
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

function acceptsAsset(tool: MediaTool, asset: Asset) {
  if (tool === 'inspect') return ['image', 'audio', 'video'].includes(asset.media_type)
  if (tool === 'transcode') return ['audio', 'video'].includes(asset.media_type)
  if (tool === 'audio') return ['audio', 'video'].includes(asset.media_type)
  return asset.media_type === 'video'
}

function assetOption(asset: Asset) {
  return { value: asset.id, label: `${asset.name} · ${asset.media_type} · ${formatBytes(asset.file_size_bytes)}` }
}

function groupOptions(groups: AssetGroup[]) {
  const byID = new Map(groups.map((group) => [group.id, group]))
  const path = (group: AssetGroup) => {
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
    return names.join(' / ')
  }
  return groups.map((group) => ({ value: group.id, label: `${kindLabel(group.kind)} · ${path(group)}` }))
}

function sourceNames(job: MediaJob, assets: Asset[]) {
  const names = (job.source_asset_ids ?? []).map((id) => assets.find((asset) => asset.id === id)?.name).filter(Boolean)
  return names.join(' + ') || '源资产'
}

function kindLabel(kind: AssetGroup['kind']) {
  return ({ character: '人物', scene: '场景', prop: '道具', material: '素材' })[kind]
}

function statusLabel(status: MediaJob['status']) {
  return ({ queued: '排队', running: '处理中', succeeded: '已完成', failed: '失败', canceled: '已取消', interrupted: '已中断' })[status]
}

function stageLabel(stage: string) {
  return ({ queued: '等待处理', preparing: '准备文件', probing: '读取媒体信息', processing: 'FFmpeg 处理', storing: '存入本地资产', completed: '处理完成', failed: '处理失败', canceled: '已取消', interrupted: '已中断' } as Record<string, string>)[stage] ?? stage
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
