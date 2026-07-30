import { useEffect, useRef, useState } from 'react'
import { CloudOutlined, DownloadOutlined, FileOutlined, HddOutlined } from '@ant-design/icons'
import { Button, Modal, Segmented, Skeleton, Tag } from 'antd'
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

export function MaterialViewerModal({ item, open, url, exports = [], onClose }: {
  item: MaterialViewItem | null
  open: boolean
  url: string
  exports?: AssetRemoteExport[]
  onClose: () => void
}) {
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(false)
  const [textMode, setTextMode] = useState<'rendered' | 'source'>('rendered')
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
  const previewURL = item && ['audio', 'video'].includes(item.media_type) ? withQuery(url, 'proxy=1') : url
  const downloadURL = url ? `${url}${url.includes('?') ? '&' : '?'}download=1` : ''
  return <Modal
    destroyOnHidden
    footer={<Button download href={downloadURL} icon={<DownloadOutlined/>}>下载原文件</Button>}
    onCancel={() => { stopPlayback(); onClose() }}
    open={open}
    title={item?.name}
    width={1040}
  >
    {item && <>
      <div className={`material-viewer material-${item.media_type}`}>
        {item.media_type === 'image' && <img alt={item.name} src={url}/>}
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
        <div className="material-storage-row local"><HddOutlined/><div><strong>本地原件</strong><span>{item.mime_type} · {formatBytes(item.file_size_bytes)}</span></div><Tag color="green">主副本</Tag></div>
        {exports.map((item) => <div className={`material-storage-row remote state-${item.state}`} key={item.id}>
          <CloudOutlined/>
          <div><strong>{item.connection_name || 'S3 连接'}{item.connection_is_default && <em>默认</em>}</strong><span title={item.object_key}>{item.object_key}</span></div>
          <Tag color={item.state === 'ready' && item.connection_enabled ? 'orange' : 'default'}>{item.state === 'ready' ? (item.connection_enabled ? '可用' : '连接停用') : item.state}</Tag>
          {item.public_url && <Button onClick={() => void openRemoteExport(item.id)} size="small" type="link">打开副本</Button>}
        </div>)}
        {!exports.length && <div className="material-storage-empty"><CloudOutlined/><span>尚未发布 S3 公网副本</span></div>}
      </div>
    </>}
  </Modal>
}

async function openRemoteExport(id: string) {
  const signed = await api.assetExportURL(id)
  window.open(signed.url, '_blank', 'noopener,noreferrer')
}

function withQuery(url: string, value: string) {
  return url ? `${url}${url.includes('?') ? '&' : '?'}${value}` : ''
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
