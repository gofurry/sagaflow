import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeftOutlined, ArrowRightOutlined, AudioOutlined, CloseOutlined, CloudOutlined, FileTextOutlined, FolderOpenOutlined, HddOutlined, InboxOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons'
import { App, Button, Checkbox, Empty, Input, Modal, Select, Spin, Tooltip, Tree } from 'antd'
import type { DataNode } from 'antd/es/tree'
import { api } from '../../api/client'
import type { Asset, AssetGroup, AssetRemoteExport, GenerationInputReference, GenerationReferenceUpload, MediaType } from '../../api/types'
import { assetExportLabel, preferredAssetExport, usableAssetExports } from '../assets/storage'

export interface GenerationReferenceDraft extends GenerationInputReference {
  name: string
  mediaType: MediaType
}

export function GenerationReferencePicker({
  projectID,
  assets,
  exports,
  groups,
  value,
  allowedMedia,
  requiresPublishedAssets = false,
  onChange,
  onError,
}: {
  projectID: string
  assets: Asset[]
  exports: AssetRemoteExport[]
  groups: AssetGroup[]
  value: GenerationReferenceDraft[]
  allowedMedia: MediaType[]
  requiresPublishedAssets?: boolean
  onChange: (value: GenerationReferenceDraft[]) => void
  onError: (error: unknown) => void
}) {
  const { message } = App.useApp()
  const inputRef = useRef<HTMLInputElement>(null)
  const [libraryOpen, setLibraryOpen] = useState(false)
  const [uploading, setUploading] = useState(false)
  const allowed = useMemo(() => new Set(allowedMedia), [allowedMedia])
  const availableAssets = useMemo(() => assets.filter((asset) => asset.status !== 'discarded' && allowed.has(asset.media_type)), [allowed, assets])

  const remove = async (reference: GenerationReferenceDraft) => {
    onChange(value.filter((item) => !(item.source === reference.source && item.id === reference.id)))
    if (reference.source === 'upload') {
      try {
        await api.deleteGenerationReference(reference.id)
      } catch {
        // A submitted reference stays attached to its historical job; only remove it from this draft.
      }
    }
  }
  const move = (index: number, offset: number) => {
    const target = index + offset
    if (target < 0 || target >= value.length) return
    const next = [...value]
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }
  const upload = async (files: FileList | null) => {
    if (!files?.length) return
    setUploading(true)
    const created: GenerationReferenceUpload[] = []
    try {
      for (const file of Array.from(files)) {
        const item = await api.uploadGenerationReference(projectID, file)
        if (!allowed.has(item.media_type)) {
          await api.deleteGenerationReference(item.id)
          throw new Error(`文件“${file.name}”不是当前模型支持的参考类型`)
        }
        created.push(item)
      }
      onChange([...value, ...created.map((item) => ({ source: 'upload' as const, id: item.id, name: item.name, mediaType: item.media_type }))])
      message.success(`已添加 ${created.length} 个临时参考`)
    } catch (error) {
      await Promise.all(created.map((item) => api.deleteGenerationReference(item.id).catch(() => undefined)))
      onError(error)
    } finally {
      setUploading(false)
      if (inputRef.current) inputRef.current.value = ''
    }
  }

  return <div className="generation-reference-picker">
    <div className="reference-picker-heading">
      <div>
        <strong>参考预览</strong>
        <span>{value.length
          ? `${value.length} 项 · 生成时按当前顺序提交`
          : requiresPublishedAssets ? '当前模型只接受已在资产页发布到 S3 的资产' : '可从资产库选择，或上传仅用于本次生成的临时参考'}</span>
      </div>
      <div className="reference-add-actions">
        <Tooltip title="从资产库添加">
          <Button aria-label="从资产库添加参考" icon={<FolderOpenOutlined/>} onClick={() => setLibraryOpen(true)} shape="circle"/>
        </Tooltip>
        {!requiresPublishedAssets && <Tooltip title="上传本次参考；提交后作为任务快照保留，不进入资产库">
          <Button aria-label="上传临时参考" icon={uploading ? <Spin size="small"/> : <UploadOutlined/>} onClick={() => inputRef.current?.click()} shape="circle"/>
        </Tooltip>}
        <input accept={acceptFor(allowedMedia)} hidden multiple onChange={(event) => void upload(event.target.files)} ref={inputRef} type="file"/>
      </div>
    </div>
    {value.length
      ? <div className="reference-preview-grid">{value.map((reference, index) => <div className="reference-preview-item" key={`${reference.source}-${reference.id}`}>
          <div className="reference-preview-media">
            <ReferencePreview reference={reference}/>
            <em>{referenceLabel(reference.mediaType, index)}</em>
            <span>{reference.source === 'asset' ? '资产' : '临时'}</span>
          </div>
          <div className="reference-preview-footer">
            <strong title={reference.name}>{reference.name}</strong>
            <div>
              <Button disabled={index === 0} icon={<ArrowLeftOutlined/>} onClick={() => move(index, -1)} size="small" type="text"/>
              <Button disabled={index === value.length - 1} icon={<ArrowRightOutlined/>} onClick={() => move(index, 1)} size="small" type="text"/>
              <Button danger icon={<CloseOutlined/>} onClick={() => void remove(reference)} size="small" type="text"/>
            </div>
          </div>
          <ReferenceTransport
            exports={exports}
            onChange={(remote_export_id) => onChange(value.map((item) => item.source === reference.source && item.id === reference.id ? { ...item, remote_export_id } : item))}
            reference={reference}
            requiresPublishedAssets={requiresPublishedAssets}
          />
        </div>)}</div>
      : <button className="reference-empty" onClick={() => setLibraryOpen(true)} type="button"><InboxOutlined/><span>添加参考素材</span><small>{referenceHint(allowedMedia, requiresPublishedAssets)}</small></button>}
    <AssetReferenceModal
      assets={availableAssets}
      exports={exports}
      groups={groups}
      onCancel={() => setLibraryOpen(false)}
      onConfirm={(selected) => {
        const byID = new Map(availableAssets.map((asset) => [asset.id, asset]))
        const retained = value.filter((item) => item.source === 'upload' || selected.includes(item.id))
        const retainedAssetIDs = new Set(retained.filter((item) => item.source === 'asset').map((item) => item.id))
        onChange([...retained, ...selected.filter((id) => !retainedAssetIDs.has(id)).flatMap((id) => {
          const asset = byID.get(id)
          const preferred = requiresPublishedAssets ? preferredAssetExport(exports, asset?.id ?? '') : undefined
          return asset ? [{ source: 'asset' as const, id: asset.id, name: asset.name, mediaType: asset.media_type, remote_export_id: preferred?.id }] : []
        })])
        setLibraryOpen(false)
      }}
      open={libraryOpen}
      selected={value.filter((item) => item.source === 'asset').map((item) => item.id)}
    />
  </div>
}

function AssetReferenceModal({ assets, exports, groups, selected, open, onCancel, onConfirm }: { assets: Asset[]; exports: AssetRemoteExport[]; groups: AssetGroup[]; selected: string[]; open: boolean; onCancel: () => void; onConfirm: (selected: string[]) => void }) {
  const [groupSearch, setGroupSearch] = useState('')
  const [assetSearch, setAssetSearch] = useState('')
  const [selectedGroup, setSelectedGroup] = useState<string>()
  const [checked, setChecked] = useState<string[]>(selected)
  const selectedKey = selected.join('|')
  const groupsWithAssets = useMemo(() => new Set(assets.flatMap((asset) => asset.group_id ? [asset.group_id] : [])), [assets])
  const tree = useMemo(() => buildTree(groups, groupSearch, groupsWithAssets), [groupSearch, groups, groupsWithAssets])
  useEffect(() => {
    if (!open) return
    setChecked(selectedKey ? selectedKey.split('|') : [])
    const firstGroup = groups.find((group) => groupsWithAssets.has(group.id))
    setSelectedGroup((current) => current && groups.some((group) => group.id === current) ? current : firstGroup?.id)
  }, [groups, groupsWithAssets, open, selectedKey])
  const visibleAssets = assets.filter((asset) => asset.group_id === selectedGroup && asset.name.toLowerCase().includes(assetSearch.trim().toLowerCase()))
  const toggle = (id: string, next: boolean) => setChecked((current) => next ? [...new Set([...current, id])] : current.filter((item) => item !== id))

  return <Modal cancelText="取消" okText={`添加 ${checked.length} 项参考`} onCancel={onCancel} onOk={() => onConfirm(checked)} open={open} title="从资产库添加参考" width={1040}>
    <div className="reference-library-modal">
      <aside>
        <Input allowClear onChange={(event) => setGroupSearch(event.target.value)} placeholder="搜索分组" prefix={<SearchOutlined/>} value={groupSearch}/>
        {tree.length
          ? <Tree blockNode defaultExpandAll onSelect={(keys) => setSelectedGroup(keys[0] as string | undefined)} selectedKeys={selectedGroup ? [selectedGroup] : []} treeData={tree}/>
          : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的分组"/>}
      </aside>
      <section>
        <div className="reference-library-search">
          <Input allowClear onChange={(event) => setAssetSearch(event.target.value)} placeholder="搜索当前分组内的资产" prefix={<SearchOutlined/>} value={assetSearch}/>
          <span>已选 {checked.length}</span>
        </div>
        {visibleAssets.length
          ? <div className="reference-library-grid">{visibleAssets.map((asset) => <button className={checked.includes(asset.id) ? 'selected' : ''} key={asset.id} onClick={() => toggle(asset.id, !checked.includes(asset.id))} type="button">
              <div><img alt={asset.name} src={api.assetURL(asset.id)}/><Checkbox checked={checked.includes(asset.id)} onChange={(event) => toggle(asset.id, event.target.checked)} onClick={(event) => event.stopPropagation()}/></div>
              <strong title={asset.name}>{asset.name}</strong>
              <small>{asset.status === 'adopted' ? '已采用' : '候选资产'} · 本地{usableAssetExports(exports, asset.id).length ? ` · S3 × ${usableAssetExports(exports, asset.id).length}` : ''}</small>
            </button>)}</div>
          : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={selectedGroup ? '当前分组没有可用资产' : '选择左侧分组查看资产'}/>}
      </section>
    </div>
  </Modal>
}

function buildTree(groups: AssetGroup[], search: string, groupsWithAssets: Set<string>): DataNode[] {
  const normalized = search.trim().toLowerCase()
  const byParent = new Map<string | null, AssetGroup[]>()
  groups.forEach((group) => byParent.set(group.parent_id, [...(byParent.get(group.parent_id) ?? []), group]))
  const walk = (parentID: string | null): DataNode[] => (byParent.get(parentID) ?? []).flatMap((group) => {
    const children = walk(group.id)
    const matches = !normalized || group.name.toLowerCase().includes(normalized) || children.length > 0
    if (!matches) return []
    return [{ key: group.id, title: <span className="reference-tree-title"><span>{group.name}</span>{groupsWithAssets.has(group.id) && <em>有资产</em>}</span>, children }]
  })
  return walk(null)
}

function referenceLabel(mediaType: MediaType, index: number) {
  const prefix = ({ image: '图', video: '视频', audio: '音频', text: '文本', file: '文件' } as Record<MediaType, string>)[mediaType]
  return `${prefix} ${index + 1}`
}

function ReferenceTransport({ exports, reference, requiresPublishedAssets, onChange }: {
  exports: AssetRemoteExport[]
  reference: GenerationReferenceDraft
  requiresPublishedAssets: boolean
  onChange: (remoteExportID?: string) => void
}) {
  if (reference.source === 'upload') return <div className="reference-transport local"><HddOutlined/><span>临时文件直传</span></div>
  if (!requiresPublishedAssets) return <div className="reference-transport local"><HddOutlined/><span>本地文件直传</span></div>
  const available = usableAssetExports(exports, reference.id)
  if (!available.length) return <div className="reference-transport missing"><CloudOutlined/><span>缺少可用的 S3 副本</span></div>
  const value = available.some((item) => item.id === reference.remote_export_id) ? reference.remote_export_id : available[0].id
  return <div className="reference-transport remote"><CloudOutlined/><Select
    onChange={onChange}
    options={available.map((item) => ({ value: item.id, label: `${assetExportLabel(item)}${item.connection_is_default ? ' · 默认' : ''}` }))}
    popupMatchSelectWidth={220}
    size="small"
    value={value}
  /></div>
}

function ReferencePreview({ reference }: { reference: GenerationReferenceDraft }) {
  const url = reference.source === 'asset' ? api.assetURL(reference.id) : api.generationReferenceURL(reference.id)
  if (reference.mediaType === 'image') return <img alt={reference.name} src={url}/>
  if (reference.mediaType === 'video') return <video controls preload="metadata" src={url}/>
  if (reference.mediaType === 'audio') return <div className="reference-preview-audio"><AudioOutlined/><audio controls preload="metadata" src={url}/></div>
  if (reference.mediaType === 'text') return <FileTextOutlined/>
  return <InboxOutlined/>
}

function acceptFor(types: MediaType[]) {
  return types.map((type) => type === 'file' ? '*/*' : `${type}/*`).join(',')
}

function referenceHint(types: MediaType[], requiresPublishedAssets: boolean) {
  const labels = types.map((type) => ({ image: '图像', video: '视频', audio: '音频', text: '文本', file: '文件' })[type])
  const media = labels.length > 1 ? labels.join('、') : labels[0] ?? '素材'
  return requiresPublishedAssets ? `选择已发布到 S3 的${media}资产` : `${media}会按当前顺序传给模型`
}
