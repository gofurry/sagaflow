import { useEffect, useState } from 'react'
import { DownloadOutlined, FileOutlined } from '@ant-design/icons'
import { Button, Modal, Segmented, Skeleton } from 'antd'
import type { MediaType } from '../api/types'
import { MarkdownPreview } from './Markdown'

export interface MaterialViewItem {
  name: string
  media_type: MediaType
  mime_type: string
  file_size_bytes: number
  content_text?: string
}

export function MaterialViewerModal({ item, open, url, onClose }: {
  item: MaterialViewItem | null
  open: boolean
  url: string
  onClose: () => void
}) {
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(false)
  const [textMode, setTextMode] = useState<'rendered' | 'source'>('rendered')
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
      setText('')
      setTextMode('rendered')
    }
  }, [open])
  const previewURL = item && ['audio', 'video'].includes(item.media_type) ? withQuery(url, 'proxy=1') : url
  const downloadURL = url ? `${url}${url.includes('?') ? '&' : '?'}download=1` : ''
  return <Modal
    footer={<Button download href={downloadURL} icon={<DownloadOutlined/>}>下载原文件</Button>}
    onCancel={onClose}
    open={open}
    title={item?.name}
    width={1040}
  >
    {item && <div className={`material-viewer material-${item.media_type}`}>
      {item.media_type === 'image' && <img alt={item.name} src={url}/>}
      {item.media_type === 'video' && <video controls preload="metadata" src={previewURL}/>}
      {item.media_type === 'audio' && <div className="material-audio-viewer"><div className="material-audio-disc"/><audio controls preload="metadata" src={previewURL}/><span>{formatBytes(item.file_size_bytes)} · {item.mime_type}</span></div>}
      {item.media_type === 'text' && <div className="material-text-viewer">
        <div className="material-viewer-switch"><Segmented value={textMode} onChange={(value) => setTextMode(value as 'rendered' | 'source')} options={[{ label: '渲染', value: 'rendered' }, { label: '原文', value: 'source' }]}/><span>{text.length} 字</span></div>
        {loading
          ? <Skeleton active paragraph={{ rows: 12 }}/>
          : textMode === 'rendered'
            ? <MarkdownPreview emptyText="文本内容为空" value={text}/>
            : <pre>{text}</pre>}
      </div>}
      {item.media_type === 'file' && <div className="material-file-viewer"><FileOutlined/><strong>{item.name}</strong><span>{item.mime_type} · {formatBytes(item.file_size_bytes)}</span></div>}
    </div>}
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
