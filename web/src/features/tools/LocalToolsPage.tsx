import { useEffect, useMemo, useState } from 'react'
import {
  AudioOutlined,
  CameraOutlined,
  CheckCircleOutlined,
  ColumnWidthOutlined,
  CompressOutlined,
  DeleteOutlined,
  FileSearchOutlined,
  FontSizeOutlined,
  MergeCellsOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  ScissorOutlined,
  StopOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { App, Button, Input, InputNumber, Progress, Select, Switch } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Asset, AssetGroup, MediaJob, MediaTool, Project } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'

interface Draft {
  sourceAssetIDs: string[]
  targetGroupID?: string
  outputName: string
  parameters: Record<string, unknown>
}

const toolOrder: MediaTool[] = ['inspect', 'transcode', 'aspect', 'audio', 'trim', 'merge', 'subtitle', 'screenshot']
const STAGING_DESTINATION = '__staging__'
const toolMeta: Record<MediaTool, { label: string; icon: React.ReactNode; hint: string }> = {
  inspect: { label: '检查', icon: <FileSearchOutlined/>, hint: '读取编码、画幅、时长、码率与媒体流信息，不产生新资产。' },
  transcode: { label: '转换', icon: <CompressOutlined/>, hint: '在常用视频和音频格式之间转换，源文件保持不变。' },
  aspect: { label: '画幅', icon: <ColumnWidthOutlined/>, hint: '按目标尺寸完整显示或裁切铺满，生成新的 MP4 资产。' },
  audio: { label: '音频', icon: <AudioOutlined/>, hint: '提取音轨、标准化响度，或移除视频中的音轨。' },
  trim: { label: '裁切', icon: <ScissorOutlined/>, hint: '按起始时间和片段时长截取内容。' },
  merge: { label: '合片', icon: <MergeCellsOutlined/>, hint: '按照选择顺序将多个视频衔接为一个文件。' },
  subtitle: { label: '字幕', icon: <FontSizeOutlined/>, hint: '将 SRT/ASS 字幕封装为软字幕，或直接烧录到画面。' },
  screenshot: { label: '截图', icon: <CameraOutlined/>, hint: '从视频指定时间截取 PNG 或 JPG 静帧。' },
}

const defaultParameters: Record<MediaTool, Record<string, unknown>> = {
  inspect: {},
  transcode: { format: 'mp4' },
  aspect: { width: 1920, height: 1080, fit: 'contain', background: '#F5EDE2' },
  audio: { audio_mode: 'extract' },
  trim: { start_seconds: 0, duration: 5, fast: false },
  merge: { fast: true },
  subtitle: { subtitle_mode: 'soft' },
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
  const sourceCountValid = tool === 'merge' ? selectedSourceCount >= 2 : tool === 'subtitle' ? selectedSourceCount === 2 : selectedSourceCount === 1
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
        <div>
          <strong>{statusQuery.data?.available ? 'FFmpeg 已就绪' : 'FFmpeg 未就绪'}</strong>
          <span>{statusQuery.data?.available ? shortVersion(statusQuery.data.version) : statusQuery.data?.message ?? '正在检查本地工具…'}</span>
        </div>
        {statusQuery.data?.available && <small>{runtimeSourceLabel(statusQuery.data.source)}</small>}
      </div>

      <div className="local-tool-workbench">
        <header>
          <div><h2>{toolMeta[tool].label}</h2><p>{toolMeta[tool].hint}</p></div>
          <span>{eligibleAssets.length} 个可用素材</span>
        </header>

        <div className="local-tool-fields">
          {tool === 'subtitle'
            ? <>
              <ToolField label="视频资产"><SelectAsset assets={assets.filter((asset) => asset.media_type === 'video')} onChange={(value) => setDraft((current) => ({ ...current, sourceAssetIDs: [value ?? '', current.sourceAssetIDs[1] ?? ''] }))} placeholder="选择视频" value={draft.sourceAssetIDs[0]}/></ToolField>
              <ToolField label="字幕资产"><SelectAsset assets={assets.filter((asset) => asset.media_type === 'text' || asset.media_type === 'file')} onChange={(value) => setDraft((current) => ({ ...current, sourceAssetIDs: [current.sourceAssetIDs[0] ?? '', value ?? ''] }))} placeholder="选择 SRT 或 ASS 字幕" value={draft.sourceAssetIDs[1]}/></ToolField>
            </>
            : <ToolField className={tool === 'merge' ? 'wide' : ''} label={tool === 'merge' ? '源视频（选择顺序就是合片顺序）' : '源资产'}>
              <Select
                allowClear
                maxTagCount="responsive"
                mode={tool === 'merge' ? 'multiple' : undefined}
                onChange={(value) => setDraft((current) => ({ ...current, sourceAssetIDs: Array.isArray(value) ? value : value ? [value] : [] }))}
                optionFilterProp="label"
                options={eligibleAssets.map(assetOption)}
                placeholder={tool === 'merge' ? '依次选择至少两个视频' : '选择本地资产'}
                showSearch
                value={tool === 'merge' ? draft.sourceAssetIDs : draft.sourceAssetIDs[0]}
              />
            </ToolField>}
          {tool !== 'inspect' && <ToolField label="输出位置"><Select onChange={(destination) => setDraft((current) => ({ ...current, targetGroupID: destination === STAGING_DESTINATION ? undefined : destination }))} optionFilterProp="label" options={[{ value: STAGING_DESTINATION, label: '未处理暂存区 · 稍后手动入库' }, ...groupOptions(groups)]} showSearch value={draft.targetGroupID ?? STAGING_DESTINATION}/></ToolField>}
          {tool !== 'inspect' && <ToolField label="输出名称（可选）"><Input maxLength={160} onChange={(event) => setDraft((current) => ({ ...current, outputName: event.target.value }))} placeholder="未填写时根据源文件命名" value={draft.outputName}/></ToolField>}
        </div>

        <ToolParameters parameters={draft.parameters} tool={tool} update={updateParameters}/>

        {draft.sourceAssetIDs.length > 0 && <SourceStrip assets={assets} ids={draft.sourceAssetIDs}/>}
      </div>

      <MediaTaskCenter assets={assets} jobs={jobs} onSelect={setSelectedJobID} selectedJob={selectedJob}/>
    </section>
  </div>
}

function ToolField({ children, className = '', label }: { children: React.ReactNode; className?: string; label: string }) {
  return <label className={className}><span>{label}</span>{children}</label>
}

function SelectAsset({ assets, onChange, placeholder, value }: { assets: Asset[]; onChange: (value?: string) => void; placeholder: string; value?: string }) {
  return <Select allowClear onChange={onChange} optionFilterProp="label" options={assets.map(assetOption)} placeholder={placeholder} showSearch value={value}/>
}

function ToolParameters({ parameters, tool, update }: { parameters: Record<string, unknown>; tool: MediaTool; update: (patch: Record<string, unknown>) => void }) {
  if (tool === 'inspect') return null
  return <div className="local-tool-parameters">
    {tool === 'transcode' && <ToolField label="输出格式"><Select onChange={(format) => update({ format })} options={[
      { value: 'mp4', label: 'MP4 · H.264 / AAC' }, { value: 'webm', label: 'WebM · VP9 / Opus' },
      { value: 'mp3', label: 'MP3 音频' }, { value: 'wav', label: 'WAV 无损音频' }, { value: 'm4a', label: 'M4A · AAC' },
    ]} value={String(parameters.format)}/></ToolField>}
    {tool === 'aspect' && <>
      <ToolField label="宽度"><InputNumber max={7680} min={64} onChange={(width) => update({ width: width ?? 1920 })} value={Number(parameters.width)}/></ToolField>
      <ToolField label="高度"><InputNumber max={4320} min={64} onChange={(height) => update({ height: height ?? 1080 })} value={Number(parameters.height)}/></ToolField>
      <ToolField label="适配方式"><Select onChange={(fit) => update({ fit })} options={[{ value: 'contain', label: '完整显示 · 留边' }, { value: 'cover', label: '裁切铺满' }]} value={String(parameters.fit)}/></ToolField>
      {parameters.fit === 'contain' && <ToolField label="留边颜色"><Input onChange={(event) => update({ background: event.target.value })} type="color" value={String(parameters.background)}/></ToolField>}
    </>}
    {tool === 'audio' && <ToolField label="处理方式"><Select onChange={(audio_mode) => update({ audio_mode })} options={[
      { value: 'extract', label: '提取为 MP3' }, { value: 'normalize', label: '响度标准化为 WAV' }, { value: 'mute', label: '移除视频音轨' },
    ]} value={String(parameters.audio_mode)}/></ToolField>}
    {tool === 'trim' && <>
      <ToolField label="起始时间（秒）"><InputNumber min={0} onChange={(start_seconds) => update({ start_seconds: start_seconds ?? 0 })} precision={3} value={Number(parameters.start_seconds)}/></ToolField>
      <ToolField label="片段时长（秒）"><InputNumber min={0.001} onChange={(duration) => update({ duration: duration ?? 5 })} precision={3} value={Number(parameters.duration)}/></ToolField>
      <ToolField label="快速无损裁切"><div className="local-tool-switch"><Switch checked={Boolean(parameters.fast)} onChange={(fast) => update({ fast })}/><span>速度更快，切点可能受关键帧影响</span></div></ToolField>
    </>}
    {tool === 'merge' && <ToolField label="快速无损合片"><div className="local-tool-switch"><Switch checked={Boolean(parameters.fast)} onChange={(fast) => update({ fast })}/><span>源视频编码一致时推荐开启</span></div></ToolField>}
    {tool === 'subtitle' && <ToolField label="字幕方式"><Select onChange={(subtitle_mode) => update({ subtitle_mode })} options={[{ value: 'soft', label: '软字幕 · 可开关' }, { value: 'burn', label: '烧录字幕 · 固定画面' }]} value={String(parameters.subtitle_mode)}/></ToolField>}
    {tool === 'screenshot' && <>
      <ToolField label="截图时间（秒）"><InputNumber min={0} onChange={(time_seconds) => update({ time_seconds: time_seconds ?? 0 })} precision={3} value={Number(parameters.time_seconds)}/></ToolField>
      <ToolField label="图片格式"><Select onChange={(image_format) => update({ image_format })} options={[{ value: 'png', label: 'PNG' }, { value: 'jpg', label: 'JPG' }]} value={String(parameters.image_format)}/></ToolField>
    </>}
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
    {(job.command_snapshot ?? []).length > 0 && <details className="local-media-command"><summary>查看 FFmpeg 参数</summary><code>ffmpeg {(job.command_snapshot ?? []).join(' ')}</code></details>}
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

function runtimeSourceLabel(source: string) {
  return ({ executable_directory: '随程序交付', bundled_tools: '随程序工具目录', working_directory: '当前目录', repository_tools: '仓库开发工具', path: '系统 PATH' } as Record<string, string>)[source] ?? source
}

function shortVersion(value: string) {
  return value.replace(/^ffmpeg version\s*/i, 'FFmpeg ').split(' Copyright')[0]
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
