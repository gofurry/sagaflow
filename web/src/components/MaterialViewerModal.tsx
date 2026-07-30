import { useEffect, useRef, useState } from 'react'
import { CloudOutlined, CopyOutlined, DownloadOutlined, FileOutlined, FolderOpenOutlined, HddOutlined } from '@ant-design/icons'
import { App, Button, Modal, Segmented, Skeleton, Tag } from 'antd'
import { api } from '../api/client'
import type { AssetRemoteExport, MediaType } from '../api/types'
import { MarkdownPreview } from './Markdown'

export interface MaterialViewItem {
  name: string
  media_type: MediaType
  mime_type: string
  file_size_bytes: number
  content_text?: string
}

export function MaterialViewerModal({ assetID, item, open, url, exports = [], onClose }: {
  assetID?: string
  item: MaterialViewItem | null
  open: boolean
  url: string
  exports?: AssetRemoteExport[]
  onClose: () => void
}) {
  const { message } = App.useApp()
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(false)
  const [textMode, setTextMode] = useState<'rendered' | 'source'>('rendered')
  const [remotePreview, setRemotePreview] = useState<{ id: string; url: string } | null>(null)
  const [remoteLoadingID, setRemoteLoadingID] = useState('')
  const videoRef = useRef<HTMLVideoElement>(null)
  const audioRef = useRef<HTMLAudioElement>(null)
  const stopPlayback = () => {
    for (const media of [videoRef.current, audioRef.current]) {
      if (!media) continue
      media.pause()
      if (Number.isFinite(media.duration)) media.currentTime = 0
    }
  }
  useEffect(() => {
    if (!open || !item || item.media_type !== 'text') return
    if (item.content_text) {
      setText(item.content_text)
      return
    }
    let alive = true
    setLoading(true)
    fetch(url, { credentials: 'include' })
      .then((response) => {
        if (!response.ok) throw new Error(`读取文本失败 (${response.status})`)
        return response.text()
      })
      .then((value) => { if (alive) setText(value) })
      .catch(() => { if (alive) setText('无法读取文本内容。') })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [item, open, url])
  useEffect(() => {
    if (!open) {
      stopPlayback()
      setText('')
      setTextMode('rendered')
    }
  }, [open])
  useEffect(() => { setRemotePreview(null) }, [item, url])
  const activeURL = remotePreview?.url ?? url
  const previewURL = item && ['audio', 'video'].includes(item.media_type) && !remotePreview ? withQuery(url, 'proxy=1') : activeURL
  const downloadURL = url ? `${url}${url.includes('?') ? '&' : '?'}download=1` : ''
  useEffect(() => {
    if (!open || !item || !['audio', 'video'].includes(item.media_type)) return
    const media = item.media_type === 'video' ? videoRef.current : audioRef.current
    media?.load()
  }, [item, open, previewURL])
  const previewRemote = async (id: string) => {
    setRemoteLoadingID(id)
    try {
      if (item && ['image', 'audio', 'video'].includes(item.media_type)) {
        stopPlayback()
        setRemotePreview({ id, url: api.assetExportProxyURL(id) })
        message.success('已切换到 S3 临时预览')
      } else {
        const signed = await api.assetExportURL(id)
        await navigator.clipboard.writeText(signed.url)
        message.success('S3 临时访问链接已复制')
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '无法读取 S3 副本')
    } finally {
      setRemoteLoadingID('')
    }
  }
  const revealLocal = async () => {
    if (!assetID) return
    try {
      await api.revealAsset(assetID)
    } catch (error) {
      message.error(error instanceof Error ? error.message : '无法打开本地文件位置')
    }
  }
  return <Modal
    destroyOnHidden
    footer={<><Button disabled={!assetID} icon={<FolderOpenOutlined/>} onClick={() => void revealLocal()}>显示本地文件</Button><Button download href={downloadURL} icon={<DownloadOutlined/>}>下载原文件</Button></>}
    onCancel={() => { stopPlayback(); onClose() }}
    open={open}
    title={item?.name}
    width={1040}
  >
    {item && <>
      <div className={`material-viewer material-${item.media_type}`}>
        {remotePreview && <div className="material-viewer-source"><CloudOutlined/>正在预览 S3 临时副本</div>}
        {item.media_type === 'image' && <img alt={item.name} src={activeURL}/>}
        {item.media_type === 'video' && <video controls preload="metadata" ref={videoRef} src={previewURL}/>}
        {item.media_type === 'audio' && <div className="material-audio-viewer"><div className="material-audio-disc"/><audio controls preload="metadata" ref={audioRef} src={previewURL}/><span>{formatBytes(item.file_size_bytes)} · {item.mime_type}</span></div>}
        {item.media_type === 'text' && <div className="material-text-viewer">
          <div className="material-viewer-switch"><Segmented value={textMode} onChange={(value) => setTextMode(value as 'rendered' | 'source')} options={[{ label: '渲染', value: 'rendered' }, { label: '原文', value: 'source' }]}/><span>{text.length} 字</span></div>
          {loading
            ? <Skeleton active paragraph={{ rows: 12 }}/>
            : textMode === 'rendered'
              ? <MarkdownPreview emptyText="文本内容为空" value={text}/>
              : <pre>{text}</pre>}
        </div>}
        {item.media_type === 'file' && <div className="material-file-viewer"><FileOutlined/><strong>{item.name}</strong><span>{item.mime_type} · {formatBytes(item.file_size_bytes)}</span></div>}
      </div>
      <div className="material-storage-list">
        <div className="material-storage-row local"><HddOutlined/><div><strong>本地原件</strong><span>{item.mime_type} · {formatBytes(item.file_size_bytes)}</span></div><Tag color="green">主副本</Tag>{remotePreview && <Button onClick={() => { stopPlayback(); setRemotePreview(null) }} size="small" type="link">切回本地</Button>}</div>
        {exports.map((exported) => <div className={`material-storage-row remote state-${exported.state}`} key={exported.id}>
          <CloudOutlined/>
          <div><strong>{exported.connection_name || 'S3 连接'}{exported.connection_is_default && <em>默认</em>}</strong><span title={exported.object_key}>{exported.object_key}</span></div>
          <Tag color={exported.state === 'ready' && exported.connection_enabled ? 'orange' : 'default'}>{exported.state === 'ready' ? (exported.connection_enabled ? '可用' : '连接停用') : exported.state}</Tag>
          <Button icon={['image', 'audio', 'video'].includes(item.media_type) ? <CloudOutlined/> : <CopyOutlined/>} loading={remoteLoadingID === exported.id} onClick={() => void previewRemote(exported.id)} size="small" type="link">{['image', 'audio', 'video'].includes(item.media_type) ? (remotePreview?.id === exported.id ? '预览中' : '预览副本') : '复制临时链接'}</Button>
        </div>)}
        {!exports.length && <div className="material-storage-empty"><CloudOutlined/><span>尚未发布 S3 公网副本</span></div>}
      </div>
    </>}
  </Modal>
}

function withQuery(url: string, value: string) {
  return url ? `${url}${url.includes('?') ? '&' : '?'}${value}` : ''
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
