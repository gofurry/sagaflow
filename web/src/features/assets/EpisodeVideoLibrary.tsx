import { useEffect, useMemo, useState } from 'react'
import { CheckCircleOutlined, CloudOutlined, CloudUploadOutlined, DeleteOutlined, EditOutlined, EyeOutlined, HddOutlined, InboxOutlined, PlayCircleOutlined } from '@ant-design/icons'
import { App, Button, Empty, Input, Modal, Pagination, Popconfirm, Skeleton, Tag, Tooltip } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Asset, AssetRemoteExport, Episode } from '../../api/types'
import { MaterialViewerModal } from '../../components/MaterialViewerModal'
import type { ResultViewMode } from '../generation/StagedAssetGallery'

const PAGE_SIZE = 12

export function EpisodeVideoLibrary({ episode, exportsByAsset, onError, onPublish, projectID, videos, viewMode }: {
  episode: Episode | null
  exportsByAsset: Map<string, AssetRemoteExport[]>
  onError: (error: unknown) => void
  onPublish: (asset: Asset) => void
  projectID: string
  videos: Asset[]
  viewMode: ResultViewMode
}) {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [viewer, setViewer] = useState<Asset | null>(null)
  const [renaming, setRenaming] = useState<Asset | null>(null)
  const [name, setName] = useState('')
  const canvasQuery = useQuery({ queryKey: ['canvas', episode?.id], queryFn: () => api.canvas(episode!.id), enabled: !!episode })
  const shots = useMemo(() => (canvasQuery.data?.nodes ?? [])
    .filter((node) => node.data.kind === 'video')
    .sort((left, right) => (left.data.shot_number ?? 0) - (right.data.shot_number ?? 0)), [canvasQuery.data?.nodes])
  const shotMap = useMemo(() => new Map(shots.map((shot) => [shot.id, shot])), [shots])
  const episodeVideos = useMemo(() => videos
    .filter((asset) => asset.episode_id === episode?.id)
    .sort((left, right) => {
      const leftShot = left.canvas_node_id ? shotMap.get(left.canvas_node_id)?.data.shot_number ?? Number.MAX_SAFE_INTEGER : Number.MAX_SAFE_INTEGER
      const rightShot = right.canvas_node_id ? shotMap.get(right.canvas_node_id)?.data.shot_number ?? Number.MAX_SAFE_INTEGER : Number.MAX_SAFE_INTEGER
      return leftShot - rightShot || right.created_at.localeCompare(left.created_at)
    }), [episode?.id, shotMap, videos])
  useEffect(() => setPage(1), [episode?.id])
  const pageItems = episodeVideos.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['assets', projectID] }),
      episode && queryClient.invalidateQueries({ queryKey: ['canvas', episode.id] }),
      queryClient.invalidateQueries({ queryKey: ['staged-assets', projectID] }),
      queryClient.invalidateQueries({ queryKey: ['staged-summary', projectID] }),
    ])
  }
  const select = useMutation({
    mutationFn: (asset: Asset) => api.selectCanvasVideo(asset.canvas_node_id!, asset.id),
    onSuccess: async () => { await refresh(); message.success('已设为当前镜头视频') },
    onError,
  })
  const status = useMutation({
    mutationFn: ({ asset, adopted }: { asset: Asset; adopted: boolean }) => adopted ? api.adoptAsset(asset.id) : api.discardAsset(asset.id),
    onSuccess: async (_, input) => { await refresh(); message.success(input.adopted ? '视频资产已采用' : '视频资产已弃用') },
    onError,
  })
  const remove = useMutation({
    mutationFn: api.deleteAsset,
    onSuccess: async () => { await refresh(); message.success('视频资产已永久删除') },
    onError,
  })
  const rename = useMutation({
    mutationFn: () => api.updateAsset(renaming!.id, { name: name.trim() }),
    onSuccess: async () => { await refresh(); setRenaming(null); setName(''); message.success('视频名称已更新') },
    onError,
  })

  if (!episode) return <div className="episode-video-empty"><Empty description="请先从顶部选择一个分集，再查看对应的视频分镜"/></div>
  if (canvasQuery.isLoading) return <Skeleton active paragraph={{ rows: 9 }}/>
  if (!episodeVideos.length) return <div className="episode-video-empty"><Empty description="当前分集还没有生成的视频；请先在画布创建分镜，再到生成页生成视频"/></div>

  return <>
    <div className={`episode-video-gallery ${viewMode}`}>
      {pageItems.map((asset) => {
        const shot = asset.canvas_node_id ? shotMap.get(asset.canvas_node_id) : undefined
        const selected = shot?.data.selected_video_asset_id === asset.id
        const exports = exportsByAsset.get(asset.id) ?? []
        return <article className={`episode-video-card${selected ? ' selected' : ''}`} key={asset.id}>
          <button className="episode-video-preview" onClick={() => setViewer(asset)} type="button">
            <video muted preload="metadata" src={api.assetProxyURL(asset.id)}/>
            <span><PlayCircleOutlined/></span>
          </button>
          <div className="episode-video-info">
            <div><strong>{asset.name}</strong>{selected && <Tag color="orange" icon={<CheckCircleOutlined/>}>当前镜头</Tag>}</div>
            <span>{shot ? `${String(shot.data.shot_number ?? 0).padStart(2, '0')} · ${shot.data.title}` : '原分镜已删除'}</span>
            <small>{new Date(asset.created_at).toLocaleString('zh-CN')} · {formatBytes(asset.file_size_bytes)}</small>
            <div className="asset-storage-badges"><span><HddOutlined/>本地原件</span>{exports.length > 0 && <span className="remote"><CloudOutlined/>S3 × {exports.length}</span>}</div>
          </div>
          <div className="episode-video-actions">
            {!selected && asset.status !== 'discarded' && asset.canvas_node_id && <Button icon={<CheckCircleOutlined/>} loading={select.isPending} onClick={() => select.mutate(asset)} size="small" type="primary">设为当前</Button>}
            <Tooltip title="查看视频"><Button icon={<EyeOutlined/>} onClick={() => setViewer(asset)} size="small" type="text"/></Tooltip>
            <Tooltip title="重命名"><Button icon={<EditOutlined/>} onClick={() => { setRenaming(asset); setName(asset.name) }} size="small" type="text"/></Tooltip>
            <Tooltip title={exports.length ? `管理 ${exports.length} 个 S3 副本` : '发布到 S3'}><Button icon={<CloudUploadOutlined/>} onClick={() => onPublish(asset)} size="small" type="text"/></Tooltip>
            {asset.status === 'adopted'
              ? <Tooltip title="弃用资产"><Button icon={<InboxOutlined/>} onClick={() => status.mutate({ asset, adopted: false })} size="small" type="text"/></Tooltip>
              : <Tooltip title={asset.status === 'discarded' ? '重新采用资产' : '采用资产'}><Button icon={<CheckCircleOutlined/>} onClick={() => status.mutate({ asset, adopted: true })} size="small" type="text"/></Tooltip>}
            <Popconfirm cancelText="取消" description="将永久删除数据库记录和视频文件。" okButtonProps={{ danger: true }} okText="永久删除" onConfirm={() => remove.mutate(asset.id)} title="确认删除这个视频？">
              <Tooltip title="删除"><Button danger icon={<DeleteOutlined/>} size="small" type="text"/></Tooltip>
            </Popconfirm>
          </div>
        </article>
      })}
    </div>
    {episodeVideos.length > PAGE_SIZE && <Pagination current={page} onChange={setPage} pageSize={PAGE_SIZE} showSizeChanger={false} total={episodeVideos.length}/>}
    <MaterialViewerModal exports={viewer ? exportsByAsset.get(viewer.id) ?? [] : []} item={viewer} onClose={() => setViewer(null)} open={!!viewer} url={viewer ? api.assetURL(viewer.id) : ''}/>
    <Modal cancelText="取消" confirmLoading={rename.isPending} okButtonProps={{ disabled: !name.trim() }} okText="保存" onCancel={() => setRenaming(null)} onOk={() => rename.mutate()} open={!!renaming} title="重命名视频">
      <Input autoFocus maxLength={160} onChange={(event) => setName(event.target.value)} onPressEnter={() => name.trim() && rename.mutate()} value={name}/>
    </Modal>
  </>
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
