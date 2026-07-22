import { useMemo, useState, type MouseEvent } from 'react'
import { App, Button, DatePicker, Empty, Input, Modal, Pagination, Popconfirm, Select, Tooltip } from 'antd'
import { CheckCircleOutlined, DeleteOutlined, EditOutlined, FileOutlined, FileTextOutlined, PictureOutlined, PlayCircleOutlined, SearchOutlined, SoundOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { GetProps } from 'antd'
import { api } from '../../api/client'
import type { AssetGroup, Capability, StagedAsset } from '../../api/types'
import { MaterialViewerModal } from '../../components/MaterialViewerModal'

export type ResultViewMode = 'grid' | 'list'
type RangeValue = GetProps<typeof DatePicker.RangePicker>['value']

export function StagedAssetGallery({
  groups,
  onError,
  projectID,
  viewMode,
  source,
  capability,
  processed,
  showProcessedFilter = false,
}: {
  groups: AssetGroup[]
  onError: (error: unknown) => void
  projectID: string
  viewMode: ResultViewMode
  source?: 'generated' | 'upload'
  capability?: Capability
  processed?: 'unprocessed' | 'imported'
  showProcessedFilter?: boolean
}) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [nameFilter, setNameFilter] = useState('')
  const [range, setRange] = useState<RangeValue>(null)
  const [sort, setSort] = useState('created_desc')
  const [processedFilter, setProcessedFilter] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<StagedAsset | null>(null)
  const [viewer, setViewer] = useState<StagedAsset | null>(null)
  const [renameItem, setRenameItem] = useState<StagedAsset | null>(null)
  const [groupID, setGroupID] = useState<string>()
  const [name, setName] = useState('')
  const effectiveProcessed = processed ?? processedFilter
  const filters = {
    source,
    capability,
    processed: effectiveProcessed,
    name: nameFilter.trim(),
    created_from: range?.[0]?.startOf('day').toISOString(),
    created_to: range?.[1]?.endOf('day').toISOString(),
    sort,
    page,
    page_size: 24,
  }
  const itemsQuery = useQuery({
    queryKey: ['staged-assets', projectID, filters],
    queryFn: () => api.stagedAssets(projectID, filters),
  })
  const pageData = itemsQuery.data
  const items = pageData?.items ?? []
  const groupOptions = useMemo(() => groups
    .map((group) => ({ value: group.id, label: assetGroupPath(group, groups) }))
    .sort((left, right) => left.label.localeCompare(right.label, 'zh-CN')), [groups])
  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['staged-assets', projectID] }),
    queryClient.invalidateQueries({ queryKey: ['staged-summary', projectID] }),
    queryClient.invalidateQueries({ queryKey: ['assets', projectID] }),
  ])
  const importItem = useMutation({
    mutationFn: () => api.importStagedAsset(selected!.id, { group_id: groupID!, name: name.trim() || undefined }),
    onSuccess: async () => {
      await refresh()
      setSelected(null)
      setGroupID(undefined)
      setName('')
      message.success('素材已入库')
    },
    onError,
  })
  const rename = useMutation({
    mutationFn: () => api.updateStagedAsset(renameItem!.id, { name: name.trim() }),
    onSuccess: async () => {
      await refresh()
      setRenameItem(null)
      setName('')
      message.success('素材名称已更新')
    },
    onError,
  })
  const remove = useMutation({
    mutationFn: api.deleteStagedAsset,
    onSuccess: async () => {
      await refresh()
      message.success('暂存素材已删除')
    },
    onError,
  })
  const openImport = (item: StagedAsset) => {
    setSelected(item)
    setGroupID(item.target_asset_group_id ?? undefined)
    setName(item.asset_name || item.name)
  }
  const openRename = (item: StagedAsset) => {
    setRenameItem(item)
    setName(item.asset_name || item.name)
  }
  const stop = (event: MouseEvent) => event.stopPropagation()

  return <>
    <div className="staged-filter-bar">
      <Input allowClear onChange={(event) => { setNameFilter(event.target.value); setPage(1) }} placeholder="按名称搜索" prefix={<SearchOutlined/>} value={nameFilter}/>
      <DatePicker.RangePicker onChange={(value) => { setRange(value); setPage(1) }} value={range}/>
      {showProcessedFilter && <Select onChange={(value) => { setProcessedFilter(value); setPage(1) }} options={[{ value: '', label: '全部状态' }, { value: 'unprocessed', label: '未处理' }, { value: 'imported', label: '已入库' }]} value={processedFilter}/>}
      <Select onChange={(value) => { setSort(value); setPage(1) }} options={[{ value: 'created_desc', label: '最新优先' }, { value: 'created_asc', label: '最早优先' }, { value: 'name_asc', label: '名称 A-Z' }, { value: 'name_desc', label: '名称 Z-A' }]} value={sort}/>
      <span>{pageData?.total ?? 0} 项</span>
    </div>
    {itemsQuery.isLoading
      ? <div className="staged-loading">正在读取暂存素材…</div>
      : items.length === 0
        ? <Empty className="generation-results-empty" image={Empty.PRESENTED_IMAGE_SIMPLE} description={processed === 'unprocessed' ? '暂存区还没有未处理素材' : processed === 'imported' ? '还没有从暂存区入库的素材' : '没有符合条件的生成结果'}/>
        : <>
          <div className={`generation-result-gallery ${viewMode}`}>
            {items.map((item) => <article
              className={`generation-result-card ${item.asset_id ? 'processed' : 'unprocessed'}`}
              key={item.id}
              onClick={() => setViewer(item)}
              role="button"
              tabIndex={0}
              onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') setViewer(item) }}
            >
              <StagedPreview item={item}/>
              <div className="generation-result-info">
                <div className="generation-result-title">
                  <strong title={item.asset_name || item.name}>{item.asset_name || item.name}</strong>
                </div>
                <span>{mediaLabel(item.media_type)} · {item.source === 'generated' ? `${item.provider_code} / ${item.model_identifier}` : '手动上传'}</span>
                <small>{new Date(item.created_at).toLocaleString()}</small>
                {item.asset_group_id && <em>已加入：{groupName(item.asset_group_id, groups)}</em>}
              </div>
              <div className="generation-result-actions" onClick={stop}>
                <Tooltip title="重命名"><Button icon={<EditOutlined/>} onClick={() => openRename(item)} shape="circle" type="text"/></Tooltip>
                {!item.asset_id && <>
                  <Tooltip title={groups.length === 0 ? '请先创建资产分组' : '加入资产库'}><Button className="generation-result-import" disabled={groups.length === 0} icon={<CheckCircleOutlined/>} onClick={() => openImport(item)} shape="circle" type="text"/></Tooltip>
                  <Popconfirm cancelText="取消" description="暂存记录和对象存储中的文件都会永久删除。" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => remove.mutate(item.id)} title="删除这个暂存素材？">
                    <Button danger icon={<DeleteOutlined/>} shape="circle" type="text"/>
                  </Popconfirm>
                </>}
                {item.asset_id && <Tooltip title="这个素材已关联正式资产"><span className="generation-result-bound"><CheckCircleOutlined/></span></Tooltip>}
              </div>
            </article>)}
          </div>
          <Pagination current={pageData?.page ?? 1} onChange={setPage} pageSize={pageData?.page_size ?? 24} showSizeChanger={false} showTotal={(total) => `共 ${total} 项`} total={pageData?.total ?? 0}/>
        </>}

    <MaterialViewerModal item={viewer} onClose={() => setViewer(null)} open={!!viewer} url={viewer ? api.stagedAssetURL(viewer.id) : ''}/>

    <Modal
      cancelText="取消"
      confirmLoading={importItem.isPending}
      okButtonProps={{ disabled: !groupID }}
      okText="加入资产库"
      onCancel={() => setSelected(null)}
      onOk={() => importItem.mutate()}
      open={!!selected}
      title="选择资产分组"
      width={640}
    >
      <div className="generation-import-form">
        <label><span>资产名称</span><Input onChange={(event) => setName(event.target.value)} value={name}/></label>
        <label><span>目标分组</span><Select autoFocus onChange={setGroupID} options={groupOptions} placeholder={groupOptions.length ? '选择一个资产分组' : '请先创建资产分组'} showSearch optionFilterProp="label" value={groupID}/></label>
        <p>入库后仍可在“已入库”中查看；删除正式资产后，这项素材会重新回到“未处理”。</p>
      </div>
    </Modal>

    <Modal cancelText="取消" confirmLoading={rename.isPending} okButtonProps={{ disabled: !name.trim() }} okText="保存" onCancel={() => setRenameItem(null)} onOk={() => rename.mutate()} open={!!renameItem} title="重命名素材">
      <Input autoFocus maxLength={160} onChange={(event) => setName(event.target.value)} onPressEnter={() => name.trim() && rename.mutate()} value={name}/>
    </Modal>
  </>
}

function StagedPreview({ item }: { item: StagedAsset }) {
  if (item.media_type === 'image') return <div className="generation-result-preview image"><img alt={item.name} src={api.stagedAssetURL(item.id)}/><span>点击查看</span></div>
  const icon = item.media_type === 'audio' ? <SoundOutlined/> : item.media_type === 'text' ? <FileTextOutlined/> : item.media_type === 'video' ? <PlayCircleOutlined/> : item.media_type === 'file' ? <FileOutlined/> : <PictureOutlined/>
  return <div className={`generation-result-preview summary ${item.media_type}`}>{icon}<strong>{mediaLabel(item.media_type)}</strong><span>{item.media_type === 'text' && item.content_text ? `${item.content_text.length} 字 · 点击查看` : '点击查看内容'}</span></div>
}

function assetGroupPath(group: AssetGroup, groups: AssetGroup[]) {
  const names = [group.name]
  let parentID = group.parent_id
  while (parentID) {
    const parent = groups.find((item) => item.id === parentID)
    if (!parent) break
    names.unshift(parent.name)
    parentID = parent.parent_id
  }
  return `${kindLabel(group.kind)} / ${names.join(' / ')}`
}

function groupName(id: string, groups: AssetGroup[]) {
  const group = groups.find((item) => item.id === id)
  return group ? assetGroupPath(group, groups) : '已删除的分组'
}

function kindLabel(kind: AssetGroup['kind']) {
  return ({ character: '人物', scene: '场景', prop: '道具', material: '素材' })[kind]
}

function mediaLabel(media: StagedAsset['media_type']) {
  return ({ text: '文本', image: '图像', audio: '音频', video: '视频', file: '文件' })[media]
}
