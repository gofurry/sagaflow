import { useEffect, useMemo, useState, type CSSProperties, type MouseEvent } from 'react'
import { AppstoreOutlined, AudioOutlined, BarsOutlined, CaretDownOutlined, CaretRightOutlined, CloudOutlined, CloudUploadOutlined, CompressOutlined, DeleteOutlined, EditOutlined, ExpandOutlined, EyeOutlined, FileImageOutlined, FileTextOutlined, HddOutlined, InboxOutlined, PlusOutlined, ReloadOutlined, UploadOutlined, VideoCameraOutlined } from '@ant-design/icons'
import { App, Button, Form, Input, Modal, Pagination, Popconfirm, Select, Skeleton, Tag, Tooltip, Upload } from 'antd'
import type { UploadFile } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Asset, AssetGroup, AssetKind, AssetRemoteExport, AssetStatus, Episode, MediaType, Project } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { MaterialViewerModal } from '../../components/MaterialViewerModal'
import { StagedAssetGallery, type ResultViewMode } from '../generation/StagedAssetGallery'
import { EpisodeVideoLibrary } from './EpisodeVideoLibrary'
import { assetExportLabel, groupAssetExports } from './storage'

const kindMeta: Record<AssetKind, { label: string; color: string }> = {
  character: { label: '人物', color: '#c8753f' },
  scene: { label: '场景', color: '#d39a62' },
  prop: { label: '道具', color: '#ad6841' },
  material: { label: '素材', color: '#9a8066' },
}
const kinds = Object.keys(kindMeta) as AssetKind[]
const statusMeta: Record<AssetStatus, { label: string; color: string }> = {
  candidate: { label: '候选', color: 'orange' },
  adopted: { label: '已采用', color: 'green' },
  discarded: { label: '已弃用', color: 'default' },
}
const ASSET_PAGE_SIZE = 48

interface Props {
  project: Project
  episode: Episode | null
  onError: (error: unknown) => void
}

interface GroupFormValues {
  name: string
  description?: string
}

interface PendingFile {
  uid: string
  file: File
}

interface UploadResult {
  uploaded: unknown[]
  failed: Array<PendingFile & { reason: unknown }>
}

export function AssetLibraryPage({ episode, project, onError }: Props) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [groupForm] = Form.useForm<GroupFormValues>()
  const [uploadForm] = Form.useForm()
  const [stagingUploadForm] = Form.useForm()
  const [selectedKind, setSelectedKind] = useState<AssetKind>('character')
  const [stagingView, setStagingView] = useState<'unprocessed' | 'imported' | null>(null)
  const [videoView, setVideoView] = useState(false)
  const [resultView, setResultView] = useState<ResultViewMode>('grid')
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [assetPanels, setAssetPanels] = useState<Set<string>>(new Set())
  const [groupOpen, setGroupOpen] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [stagingUploadOpen, setStagingUploadOpen] = useState(false)
  const [editingGroup, setEditingGroup] = useState<AssetGroup | null>(null)
  const [createParent, setCreateParent] = useState<AssetGroup | null>(null)
  const [pendingFiles, setPendingFiles] = useState<PendingFile[]>([])
  const [stagingPendingFiles, setStagingPendingFiles] = useState<PendingFile[]>([])
  const [viewerAsset, setViewerAsset] = useState<Asset | null>(null)
  const [renameAsset, setRenameAsset] = useState<Asset | null>(null)
  const [renameName, setRenameName] = useState('')
	const [publishingAsset, setPublishingAsset] = useState<Asset | null>(null)
	const [publishingGroup, setPublishingGroup] = useState<AssetGroup | null>(null)
	const [publishConnectionID, setPublishConnectionID] = useState<string>()
  const [assetPage, setAssetPage] = useState(1)

  const groupsQuery = useQuery({ queryKey: ['asset-groups', project.id], queryFn: () => api.assetGroups(project.id) })
  const assetFilters = videoView
    ? { episode_id: episode?.id, media_type: 'video', ungrouped: 1, page: assetPage, page_size: ASSET_PAGE_SIZE }
    : { group_kind: selectedKind, page: assetPage, page_size: ASSET_PAGE_SIZE }
  const assetsQuery = useQuery({
    queryKey: ['assets', project.id, assetFilters],
    queryFn: () => api.assetPage(project.id, assetFilters),
    enabled: !stagingView,
    placeholderData: (previous) => previous,
  })
  const assetSummaryQuery = useQuery({ queryKey: ['asset-summary', project.id], queryFn: () => api.assetSummary(project.id) })
  const stagedSummaryQuery = useQuery({ queryKey: ['staged-summary', project.id], queryFn: () => api.stagedAssetSummary(project.id) })
	const s3Query = useQuery({ queryKey: ['s3-connections'], queryFn: api.s3Connections })
	const exportsQuery = useQuery({ queryKey: ['asset-exports', publishingAsset?.id], queryFn: () => api.assetExports(publishingAsset!.id), enabled: !!publishingAsset })
  const projectExportsQuery = useQuery({ queryKey: ['asset-exports', 'project', project.id], queryFn: () => api.projectAssetExports(project.id) })
  const groups = useMemo(() => groupsQuery.data ?? [], [groupsQuery.data])
  const assets = useMemo(() => assetsQuery.data?.items ?? [], [assetsQuery.data?.items])
  const exportsByAsset = useMemo(() => groupAssetExports(projectExportsQuery.data ?? []), [projectExportsQuery.data])
  const kindGroups = useMemo(() => groups.filter((group) => group.kind === selectedKind), [groups, selectedKind])
  const kindGroupIDs = useMemo(() => new Set(kindGroups.map((group) => group.id)), [kindGroups])
  const kindAssets = useMemo(() => assets.filter((asset) => !!asset.group_id && kindGroupIDs.has(asset.group_id)), [assets, kindGroupIDs])
  const episodeVideoAssets = useMemo(() => videoView ? assets : [], [assets, videoView])
  const rootGroups = useMemo(() => sortedGroups(kindGroups.filter((group) => !group.parent_id)), [kindGroups])
  const selectedGroup = kindGroups.find((group) => group.id === selectedID) ?? null
  const refreshing = groupsQuery.isFetching || assetsQuery.isFetching || assetSummaryQuery.isFetching || stagedSummaryQuery.isFetching
  const allExpanded = kindGroups.length > 0 && kindGroups.every((group) => expandedGroups.has(group.id))
  const fileList: UploadFile[] = pendingFiles.map(({ uid, file }) => ({ uid, name: file.name, size: file.size, type: file.type, status: 'done' }))
  const stagingFileList: UploadFile[] = stagingPendingFiles.map(({ uid, file }) => ({ uid, name: file.name, size: file.size, type: file.type, status: 'done' }))
  useEffect(() => setAssetPage(1), [episode?.id, project.id])

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['asset-groups', project.id] }),
      queryClient.invalidateQueries({ queryKey: ['assets', project.id] }),
      queryClient.invalidateQueries({ queryKey: ['asset-summary', project.id] }),
      queryClient.invalidateQueries({ queryKey: ['staged-assets', project.id] }),
      queryClient.invalidateQueries({ queryKey: ['staged-summary', project.id] }),
      queryClient.invalidateQueries({ queryKey: ['asset-exports', 'project', project.id] }),
    ])
  }
  const closeGroupModal = () => {
    setGroupOpen(false)
    setEditingGroup(null)
    setCreateParent(null)
    groupForm.resetFields()
  }
  const closeUpload = () => {
    setUploadOpen(false)
    setPendingFiles([])
    uploadForm.resetFields()
  }
  const closeStagingUpload = () => {
    setStagingUploadOpen(false)
    setStagingPendingFiles([])
    stagingUploadForm.resetFields()
  }
  const saveGroup = useMutation({
    mutationFn: (values: GroupFormValues) => editingGroup
      ? api.updateAssetGroup(editingGroup.id, { ...values, kind: editingGroup.kind, parent_id: editingGroup.parent_id, sort_order: editingGroup.sort_order })
      : api.createAssetGroup(project.id, { ...values, kind: createParent?.kind ?? selectedKind, parent_id: createParent?.id ?? null, sort_order: 0 }),
    onSuccess: async (group) => {
      await refresh()
      setExpandedGroups((current) => {
        const next = new Set(current)
        let parentID = group.parent_id
        while (parentID) {
          next.add(parentID)
          parentID = groups.find((item) => item.id === parentID)?.parent_id ?? null
        }
        return next
      })
      setSelectedKind(group.kind)
      setSelectedID(group.id)
      closeGroupModal()
      message.success(editingGroup ? '分组信息已保存' : '子分组已创建')
    },
    onError,
  })
  const deleteGroup = useMutation({
    mutationFn: api.deleteAssetGroup,
    onSuccess: async (result) => {
      setSelectedID(null)
      await refresh()
      message.success(`分组分支已删除，共清理 ${result.asset_count} 个资产`)
    },
    onError,
  })
  const upload = useMutation({
    mutationFn: async (values: { name?: string; media_type?: MediaType; status?: string }): Promise<UploadResult> => {
      if (!selectedGroup) return { uploaded: [], failed: [] }
      const results = await Promise.allSettled(pendingFiles.map(({ file }) => api.uploadAsset(selectedGroup.id, file, {
        ...values,
        name: pendingFiles.length === 1 ? values.name : undefined,
      })))
      const uploaded: unknown[] = []
      const failed: UploadResult['failed'] = []
      results.forEach((result, index) => {
        if (result.status === 'fulfilled') uploaded.push(result.value)
        else failed.push({ ...pendingFiles[index], reason: result.reason })
      })
      return { uploaded, failed }
    },
    onSuccess: async ({ uploaded, failed }) => {
      await refresh()
      if (selectedGroup && uploaded.length > 0) setAssetPanels((current) => new Set(current).add(selectedGroup.id))
      if (failed.length === 0) {
        closeUpload()
        message.success(`已上传 ${uploaded.length} 个资产`)
        return
      }
      setPendingFiles(failed.map(({ uid, file }) => ({ uid, file })))
      const detail = uploadErrorMessage(failed[0].reason)
      if (uploaded.length > 0) message.warning(`已上传 ${uploaded.length} 个资产，${failed.length} 个失败并已保留，可直接重试${detail ? `：${detail}` : ''}`)
      else message.error(`上传失败，文件已保留，可直接重试${detail ? `：${detail}` : ''}`)
    },
    onError,
  })
  const uploadToStaging = useMutation({
    mutationFn: async (values: { name?: string; media_type?: MediaType }): Promise<UploadResult> => {
      const results = await Promise.allSettled(stagingPendingFiles.map(({ file }) => api.uploadStagedAsset(project.id, file, {
        ...values,
        name: stagingPendingFiles.length === 1 ? values.name : undefined,
      })))
      const uploaded: unknown[] = []
      const failed: UploadResult['failed'] = []
      results.forEach((result, index) => {
        if (result.status === 'fulfilled') uploaded.push(result.value)
        else failed.push({ ...stagingPendingFiles[index], reason: result.reason })
      })
      return { uploaded, failed }
    },
    onSuccess: async ({ uploaded, failed }) => {
      await refresh()
      if (failed.length === 0) {
        closeStagingUpload()
        message.success(`已上传 ${uploaded.length} 项素材到暂存区`)
        return
      }
      setStagingPendingFiles(failed.map(({ uid, file }) => ({ uid, file })))
      const detail = uploadErrorMessage(failed[0].reason)
      if (uploaded.length > 0) message.warning(`已上传 ${uploaded.length} 项，${failed.length} 项失败并已保留${detail ? `：${detail}` : ''}`)
      else message.error(`上传失败，文件已保留，可直接重试${detail ? `：${detail}` : ''}`)
    },
    onError,
  })
  const setStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: 'adopted' | 'discarded' }) => status === 'adopted' ? api.adoptAsset(id) : api.discardAsset(id),
    onSuccess: async (_, input) => {
      await refresh()
      message.success(input.status === 'adopted' ? '资产已采用' : '资产已弃用')
    },
    onError,
  })
  const deleteAsset = useMutation({
    mutationFn: api.deleteAsset,
    onSuccess: async () => {
      await refresh()
      message.success('资产已永久删除')
    },
    onError,
  })
  const rename = useMutation({
    mutationFn: () => api.updateAsset(renameAsset!.id, { name: renameName.trim() }),
    onSuccess: async () => {
      await refresh()
      setRenameAsset(null)
      setRenameName('')
      message.success('资产名称已更新')
    },
    onError,
  })
	const publish = useMutation({
		mutationFn: () => api.publishAsset(publishingAsset!.id, publishConnectionID!),
		onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ['asset-exports', publishingAsset?.id] }), queryClient.invalidateQueries({ queryKey: ['asset-exports', 'project', project.id] })]); message.success('已发布公网副本，可作为云模型参考'); setPublishConnectionID(undefined) },
		onError,
	})
	const publishGroup = useMutation({
		mutationFn: () => api.publishAssetGroup(publishingGroup!.id, publishConnectionID!),
		onSuccess: async (result) => {
			await queryClient.invalidateQueries({ queryKey: ['asset-exports'] })
			setPublishingGroup(null)
			setPublishConnectionID(undefined)
			if (result.failed.length) message.warning(`已发布 ${result.published} 个，跳过 ${result.skipped} 个，${result.failed.length} 个失败`)
			else message.success(`分组发布完成：新增 ${result.published} 个，跳过 ${result.skipped} 个`)
		},
		onError,
	})
	const deleteExport = useMutation({
		mutationFn: api.deleteAssetExport,
		onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ['asset-exports', publishingAsset?.id] }), queryClient.invalidateQueries({ queryKey: ['asset-exports', 'project', project.id] })]); message.success('远端副本已删除，本地素材不受影响') },
		onError,
	})
	const copyExportURL = async (id: string) => {
		try {
			const result = await api.assetExportURL(id)
			await navigator.clipboard.writeText(result.url)
			message.success('临时访问链接已复制')
		} catch (error) { onError(error) }
	}
  const openAssetPublisher = (asset: Asset) => {
    const existing = new Set((exportsByAsset.get(asset.id) ?? []).map((item) => item.connection_id))
    const available = (s3Query.data ?? []).filter((item) => item.enabled && !existing.has(item.id))
    setPublishingAsset(asset)
    setPublishConnectionID((available.find((item) => item.is_default) ?? available[0])?.id)
  }
  const openGroupPublisher = (group: AssetGroup) => {
    const available = (s3Query.data ?? []).filter((item) => item.enabled)
    setPublishingGroup(group)
    setPublishConnectionID((available.find((item) => item.is_default) ?? available[0])?.id)
  }

  const openCreateGroup = (parent: AssetGroup | null = null) => {
    setEditingGroup(null)
    setCreateParent(parent)
    groupForm.setFieldsValue({ name: '', description: '' })
    setGroupOpen(true)
  }
  const openEditGroup = (group: AssetGroup) => {
    setEditingGroup(group)
    setCreateParent(null)
    groupForm.setFieldsValue({ name: group.name, description: group.description })
    setGroupOpen(true)
  }
  const openUpload = (group: AssetGroup) => {
    setSelectedID(group.id)
    uploadForm.resetFields()
    uploadForm.setFieldsValue({ status: 'candidate' })
    setPendingFiles([])
    setUploadOpen(true)
  }
  const selectGroup = (group: AssetGroup) => {
    const opening = !assetPanels.has(group.id) && !expandedGroups.has(group.id)
    setSelectedID(group.id)
    setAssetPanels((current) => changedSet(current, group.id, opening))
    setExpandedGroups((current) => changedSet(current, group.id, opening))
  }
  const toggleChildren = (group: AssetGroup) => setExpandedGroups((current) => toggledSet(current, group.id))
  const toggleAssets = (group: AssetGroup) => {
    setSelectedID(group.id)
    setAssetPanels((current) => toggledSet(current, group.id))
  }
  const toggleAll = () => setExpandedGroups((current) => {
    const next = new Set(current)
    if (allExpanded) kindGroups.forEach((group) => next.delete(group.id))
    else kindGroups.forEach((group) => next.add(group.id))
    return next
  })

  const toolbarItems = videoView
    ? [
        { key: 'refresh', label: '刷新视频', icon: <ReloadOutlined/>, loading: refreshing, onClick: () => void refresh() },
        { key: 'grid', label: '网格视图', icon: <AppstoreOutlined/>, active: resultView === 'grid', onClick: () => setResultView('grid' as const) },
        { key: 'list', label: '列表视图', icon: <BarsOutlined/>, active: resultView === 'list', onClick: () => setResultView('list' as const) },
      ]
    : stagingView
    ? [
        { key: 'refresh', label: '刷新暂存区', icon: <ReloadOutlined/>, loading: refreshing, onClick: () => void refresh() },
        ...(stagingView === 'unprocessed' ? [{ key: 'upload-staging', label: '上传素材到暂存区', icon: <UploadOutlined/>, onClick: () => { setStagingPendingFiles([]); stagingUploadForm.resetFields(); setStagingUploadOpen(true) } }] : []),
        { key: 'grid', label: '网格视图', icon: <AppstoreOutlined/>, active: resultView === 'grid', onClick: () => setResultView('grid' as const) },
        { key: 'list', label: '列表视图', icon: <BarsOutlined/>, active: resultView === 'list', onClick: () => setResultView('list' as const) },
      ]
    : [
        { key: 'refresh', label: '刷新', icon: <ReloadOutlined/>, loading: refreshing, onClick: () => void refresh() },
        { key: 'create-root', label: `新建${kindMeta[selectedKind].label}分组`, icon: <PlusOutlined/>, onClick: () => openCreateGroup() },
        { key: 'create-child', label: selectedGroup ? `在“${selectedGroup.name}”下新建子分组` : '先选择一个父分组', icon: <CaretRightOutlined/>, disabled: !selectedGroup, onClick: () => selectedGroup && openCreateGroup(selectedGroup) },
        { key: 'upload', label: selectedGroup ? `给“${selectedGroup.name}”上传资产` : '先选择一个分组', icon: <UploadOutlined/>, disabled: !selectedGroup, onClick: () => selectedGroup && openUpload(selectedGroup) },
        { key: 'expand', label: allExpanded ? '收起当前分类' : '展开当前分类', icon: allExpanded ? <CompressOutlined/> : <ExpandOutlined/>, disabled: kindGroups.length === 0, onClick: toggleAll },
      ]

  return <div className="page page-assets">
    <FloatingToolbar ariaLabel="资产工具栏" items={toolbarItems}/>

    <section className="asset-page-content">
      <div aria-label="资产分类" className="asset-kind-tabs" role="tablist">
        {kinds.map((kind) => {
          const assetCount = assetSummaryQuery.data?.by_group_kind[kind] ?? 0
          return <button
            aria-selected={!stagingView && !videoView && selectedKind === kind}
            className={!stagingView && !videoView && selectedKind === kind ? 'active' : ''}
            key={kind}
            onClick={() => {
              setStagingView(null)
              setVideoView(false)
              setSelectedKind(kind)
              setSelectedID(null)
              setAssetPage(1)
            }}
            role="tab"
            type="button"
          >
            <span className="kind-dot" style={{ background: kindMeta[kind].color }}/>
            <strong>{kindMeta[kind].label}</strong>
            <em>{assetCount}</em>
          </button>
        })}
        <span aria-hidden className="asset-tab-divider"/>
        <button
          aria-selected={stagingView === 'imported'}
          className={`generation-results-tab${stagingView === 'imported' ? ' active' : ''}`}
          onClick={() => {
            setStagingView('imported')
            setVideoView(false)
            setSelectedID(null)
            setAssetPage(1)
          }}
          role="tab"
          type="button"
        >
          <strong>已入库</strong>
          <em>{stagedSummaryQuery.data?.imported ?? 0}</em>
        </button>
        <button
          aria-selected={stagingView === 'unprocessed'}
          className={`generation-results-tab${stagingView === 'unprocessed' ? ' active' : ''}`}
          onClick={() => {
            setStagingView('unprocessed')
            setVideoView(false)
            setSelectedID(null)
            setAssetPage(1)
          }}
          role="tab"
          type="button"
        >
          <strong>未处理</strong>
          <em>{stagedSummaryQuery.data?.unprocessed ?? 0}</em>
        </button>
        <span aria-hidden className="asset-tab-divider"/>
        <button
          aria-selected={videoView}
          className={`generation-results-tab${videoView ? ' active' : ''}`}
          onClick={() => {
            setStagingView(null)
            setVideoView(true)
            setSelectedID(null)
            setAssetPage(1)
          }}
          role="tab"
          type="button"
        >
          <strong>视频</strong>
          <em>{videoView ? assetsQuery.data?.total ?? 0 : assetSummaryQuery.data?.ungrouped_video_total ?? 0}</em>
        </button>
      </div>

      {videoView
        ? <EpisodeVideoLibrary episode={episode} exportsByAsset={exportsByAsset} onError={onError} onPublish={openAssetPublisher} projectID={project.id} videos={episodeVideoAssets} viewMode={resultView}/>
        : stagingView
        ? <StagedAssetGallery groups={groups} onError={onError} processed={stagingView} projectID={project.id} viewMode={resultView}/>
        : groupsQuery.isLoading || assetsQuery.isLoading
        ? <Skeleton active paragraph={{ rows: 10 }}/>
        : rootGroups.length
          ? <div className="asset-tree-list">{rootGroups.map((group) => <AssetGroupBranch
              assetPanels={assetPanels}
              assets={kindAssets}
              exportsByAsset={exportsByAsset}
              depth={0}
              expandedGroups={expandedGroups}
              group={group}
              groups={kindGroups}
              key={group.id}
              onCreateChild={openCreateGroup}
              onDelete={(id) => deleteGroup.mutate(id)}
              onEdit={openEditGroup}
              onGroupPublish={openGroupPublisher}
              onSelect={selectGroup}
              onStatus={(id, status) => setStatus.mutate({ id, status })}
              onToggleAssets={toggleAssets}
              onToggleChildren={toggleChildren}
              onUpload={openUpload}
              onAssetDelete={(id) => deleteAsset.mutate(id)}
              onAssetRename={(asset) => { setRenameAsset(asset); setRenameName(asset.name) }}
			  onAssetPublish={openAssetPublisher}
              onAssetView={setViewerAsset}
              selectedID={selectedID}
            />)}</div>
          : <div className="asset-tab-empty"><span>“{kindMeta[selectedKind].label}”分类还没有分组</span><Button icon={<PlusOutlined/>} onClick={() => openCreateGroup()} type="primary">新建第一个分组</Button></div>}
      {!stagingView && (assetsQuery.data?.total ?? 0) > ASSET_PAGE_SIZE && <Pagination
        className="asset-library-pagination"
        current={assetPage}
        onChange={setAssetPage}
        pageSize={ASSET_PAGE_SIZE}
        showSizeChanger={false}
        total={assetsQuery.data?.total ?? 0}
      />}
    </section>

    <Modal confirmLoading={saveGroup.isPending} okText="保存" onCancel={closeGroupModal} onOk={() => groupForm.submit()} open={groupOpen} title={editingGroup ? '编辑分组' : createParent ? `新建“${createParent.name}”的子分组` : `新建${kindMeta[selectedKind].label}分组`}>
      <div className="asset-parent-context">
        <span>{createParent ? '父级路径' : '所属分类'}</span>
        <strong>{createParent ? groupPath(createParent, groups) : kindMeta[editingGroup?.kind ?? selectedKind].label}</strong>
      </div>
      <Form form={groupForm} layout="vertical" onFinish={(values) => saveGroup.mutate(values)} requiredMark={false}>
        <Form.Item label="分组名称" name="name" rules={[{ required: true, whitespace: true }]}><Input autoFocus placeholder="例如：小王 / 普通表情 / 开心"/></Form.Item>
        <Form.Item label="分组说明" name="description"><Input.TextArea placeholder="描述这个分组收纳的资产内容" rows={3}/></Form.Item>
      </Form>
    </Modal>

    <Modal cancelText="取消" confirmLoading={upload.isPending} okButtonProps={{ disabled: pendingFiles.length === 0 }} okText={`上传${pendingFiles.length ? ` (${pendingFiles.length})` : ''}`} onCancel={closeUpload} onOk={() => uploadForm.submit()} open={uploadOpen} title={`上传资产到 ${selectedGroup ? groupPath(selectedGroup, groups) : ''}`} width={720}>
      <Form form={uploadForm} initialValues={{ status: 'candidate' }} layout="vertical" onFinish={(values) => upload.mutate(values)} requiredMark={false}>
        <Upload.Dragger
          beforeUpload={(file) => {
            setPendingFiles((current) => current.some((item) => item.uid === file.uid) ? current : [...current, { uid: file.uid, file }])
            return false
          }}
          fileList={fileList}
          multiple
          onRemove={(file) => {
            setPendingFiles((current) => current.filter((item) => item.uid !== file.uid))
            return true
          }}
        >
          <p className="ant-upload-drag-icon"><InboxOutlined/></p>
          <p>拖入或点击选择一个或多个图片、音频、视频及其他文件</p>
        </Upload.Dragger>
        <Form.Item label="资产名称" name="name" style={{ marginTop: 16 }}><Input disabled={pendingFiles.length !== 1} placeholder={pendingFiles.length > 1 ? '批量上传时使用各自的原文件名' : pendingFiles[0]?.file.name ?? '默认使用文件名'}/></Form.Item>
        <div className="asset-upload-fields">
          <Form.Item label="媒体类型" name="media_type"><Select allowClear placeholder="自动识别" options={['image', 'audio', 'video', 'text', 'file'].map((value) => ({ value, label: value }))}/></Form.Item>
          <Form.Item label="初始状态" name="status"><Select options={[{ value: 'candidate', label: '候选' }, { value: 'adopted', label: '直接采用' }]}/></Form.Item>
        </div>
      </Form>
    </Modal>

    <Modal cancelText="取消" confirmLoading={uploadToStaging.isPending} okButtonProps={{ disabled: stagingPendingFiles.length === 0 }} okText={`上传到暂存区${stagingPendingFiles.length ? ` (${stagingPendingFiles.length})` : ''}`} onCancel={closeStagingUpload} onOk={() => stagingUploadForm.submit()} open={stagingUploadOpen} title="上传素材到暂存区" width={720}>
      <Form form={stagingUploadForm} layout="vertical" onFinish={(values) => uploadToStaging.mutate(values)} requiredMark={false}>
        <Upload.Dragger
          beforeUpload={(file) => {
            setStagingPendingFiles((current) => current.some((item) => item.uid === file.uid) ? current : [...current, { uid: file.uid, file }])
            return false
          }}
          fileList={stagingFileList}
          multiple
          onRemove={(file) => {
            setStagingPendingFiles((current) => current.filter((item) => item.uid !== file.uid))
            return true
          }}
        >
          <p className="ant-upload-drag-icon"><InboxOutlined/></p>
          <p>素材暂不需要选择分组，稍后可以从“未处理”中入库</p>
        </Upload.Dragger>
        <Form.Item label="素材名称" name="name" style={{ marginTop: 16 }}><Input disabled={stagingPendingFiles.length !== 1} placeholder={stagingPendingFiles.length > 1 ? '批量上传时使用各自的原文件名' : stagingPendingFiles[0]?.file.name ?? '默认使用文件名'}/></Form.Item>
        <Form.Item label="媒体类型" name="media_type"><Select allowClear placeholder="自动识别" options={['image', 'audio', 'video', 'text', 'file'].map((value) => ({ value, label: value }))}/></Form.Item>
      </Form>
    </Modal>

    <MaterialViewerModal exports={viewerAsset ? exportsByAsset.get(viewerAsset.id) ?? [] : []} item={viewerAsset} onClose={() => setViewerAsset(null)} open={!!viewerAsset} url={viewerAsset ? api.assetURL(viewerAsset.id) : ''}/>

    <Modal cancelText="取消" confirmLoading={rename.isPending} okButtonProps={{ disabled: !renameName.trim() }} okText="保存" onCancel={() => setRenameAsset(null)} onOk={() => rename.mutate()} open={!!renameAsset} title="重命名资产">
      <Input autoFocus maxLength={160} onChange={(event) => setRenameName(event.target.value)} onPressEnter={() => renameName.trim() && rename.mutate()} value={renameName}/>
    </Modal>

	<Modal cancelText="关闭" confirmLoading={publish.isPending} okButtonProps={{ disabled: !publishConnectionID }} okText="上传新副本" onCancel={() => { setPublishingAsset(null); setPublishConnectionID(undefined) }} onOk={() => publish.mutate()} open={!!publishingAsset} title={`S3 副本 · ${publishingAsset?.name ?? ''}`}>
	  <p>本地文件始终是主副本。远端副本只在你选择它作为云模型参考时使用。</p>
	  <Select onChange={setPublishConnectionID} options={s3Query.data?.filter((item) => item.enabled && !exportsQuery.data?.some((exported) => exported.connection_id === item.id)).map((item) => ({ value: item.id, label: `${item.name} · ${item.bucket}` }))} placeholder="选择尚未发布的 S3 连接" style={{ width: '100%' }} value={publishConnectionID}/>
	  {!s3Query.data?.some((item) => item.enabled) && <p>请先在“设置 → S3 发布”中添加并启用连接。</p>}
	  <div className="asset-export-list">
	    {exportsQuery.data?.map((exported) => <div key={exported.id}><div><strong>{assetExportLabel(exported)}{exported.connection_is_default ? ' · 默认' : ''}</strong><span>{exported.object_key}</span></div><Button onClick={() => void copyExportURL(exported.id)} size="small" type="text">复制链接</Button><Popconfirm cancelText="取消" description="只删除远端副本，本地素材不会删除。" okButtonProps={{ danger: true }} okText="删除副本" onConfirm={() => deleteExport.mutate(exported.id)} title="删除这个远端副本？"><Button danger loading={deleteExport.isPending && deleteExport.variables === exported.id} size="small" type="text">删除</Button></Popconfirm></div>)}
	    {!exportsQuery.isLoading && !exportsQuery.data?.length && <span>还没有远端副本</span>}
	  </div>
	</Modal>

	<Modal cancelText="取消" confirmLoading={publishGroup.isPending} okButtonProps={{ disabled: !publishConnectionID }} okText="上传分组素材" onCancel={() => { setPublishingGroup(null); setPublishConnectionID(undefined) }} onOk={() => publishGroup.mutate()} open={!!publishingGroup} title={`批量发布 · ${publishingGroup?.name ?? ''}`}>
	  <p>将把这个分组及全部子分组内尚未发布的素材上传到所选 S3；已存在的副本和已弃用素材会自动跳过。</p>
	  <Select onChange={setPublishConnectionID} options={s3Query.data?.filter((item) => item.enabled).map((item) => ({ value: item.id, label: `${item.name} · ${item.bucket}${item.is_default ? ' · 默认' : ''}` }))} placeholder="选择 S3 连接" style={{ width: '100%' }} value={publishConnectionID}/>
	  {!s3Query.data?.some((item) => item.enabled) && <p>请先在“设置 → S3 发布”中添加并启用连接。</p>}
	</Modal>
  </div>
}

interface BranchProps {
  group: AssetGroup
  groups: AssetGroup[]
  assets: Asset[]
  exportsByAsset: Map<string, AssetRemoteExport[]>
  depth: number
  selectedID: string | null
  expandedGroups: Set<string>
  assetPanels: Set<string>
  onSelect: (group: AssetGroup) => void
  onToggleChildren: (group: AssetGroup) => void
  onToggleAssets: (group: AssetGroup) => void
  onCreateChild: (parent: AssetGroup) => void
  onUpload: (group: AssetGroup) => void
  onEdit: (group: AssetGroup) => void
  onGroupPublish: (group: AssetGroup) => void
  onDelete: (id: string) => void
  onStatus: (id: string, status: 'adopted' | 'discarded') => void
  onAssetDelete: (id: string) => void
  onAssetView: (asset: Asset) => void
  onAssetRename: (asset: Asset) => void
	onAssetPublish: (asset: Asset) => void
}

function AssetGroupBranch({ group, groups, assets, exportsByAsset, depth, selectedID, expandedGroups, assetPanels, onSelect, onToggleChildren, onToggleAssets, onCreateChild, onUpload, onEdit, onGroupPublish, onDelete, onStatus, onAssetDelete, onAssetView, onAssetRename, onAssetPublish }: BranchProps) {
  const children = sortedGroups(groups.filter((item) => item.parent_id === group.id))
  const ownAssets = assets.filter((asset) => asset.group_id === group.id)
  const childrenOpen = expandedGroups.has(group.id)
  const assetsOpen = assetPanels.has(group.id)
  const subtree = descendantIDs(group.id, groups)
  const subtreeAssets = assets.filter((asset) => !!asset.group_id && subtree.has(asset.group_id)).length
  const style = { '--asset-depth': depth } as CSSProperties
  return <div className="asset-node-branch" style={style}>
    <div className={`asset-node-row${selectedID === group.id ? ' selected' : ''}`}>
      <button aria-label={children.length ? (childrenOpen ? '收起子分组' : '展开子分组') : '没有子分组'} className="asset-child-toggle" disabled={children.length === 0} onClick={() => onToggleChildren(group)} type="button">
        {childrenOpen ? <CaretDownOutlined/> : <CaretRightOutlined/>}
      </button>
      <button aria-expanded={assetsOpen || childrenOpen} className="asset-node-main" onClick={() => onSelect(group)} type="button">
        <span className="asset-node-marker"/>
        <span className="asset-node-text"><strong>{group.name}</strong>{group.description && <small>{group.description}</small>}</span>
      </button>
      <div className="asset-node-side">
        <span className="asset-node-count">{children.length} 子分组</span>
        <button className={`asset-variant-toggle${assetsOpen ? ' active' : ''}`} onClick={() => onToggleAssets(group)} type="button"><FileImageOutlined/><span>{ownAssets.length} 资产</span></button>
        <div className="asset-node-actions">
          <button className="asset-add-child" onClick={(event) => { event.stopPropagation(); onCreateChild(group) }} type="button"><PlusOutlined/><span>子分组</span></button>
          <AssetAction icon={<UploadOutlined/>} label="上传资产" onClick={() => onUpload(group)}/>
          <AssetAction icon={<CloudUploadOutlined/>} label="发布分组到 S3" onClick={() => onGroupPublish(group)}/>
          <AssetAction icon={<EditOutlined/>} label="编辑分组" onClick={() => onEdit(group)}/>
          <Popconfirm cancelText="取消" description={`将永久删除 ${subtree.size} 个分组和 ${subtreeAssets} 个资产。`} okButtonProps={{ danger: true }} okText="删除整个分支" onConfirm={() => onDelete(group.id)} title="确认删除这个分组分支？">
            <button aria-label="删除分组分支" className="asset-row-action danger" onClick={(event) => event.stopPropagation()} type="button"><DeleteOutlined/></button>
          </Popconfirm>
        </div>
      </div>
    </div>

    <div className={`asset-variant-region${assetsOpen ? ' open' : ''}`} inert={!assetsOpen}>
      <div className="asset-variant-inner">
        <div className="asset-node-expanded">
          <div className="asset-node-summary"><span>同一分组可以同时采用多个资产</span><em>{ownAssets.length} 个资产</em></div>
          {ownAssets.length
            ? <div className="asset-variant-list">{ownAssets.map((asset) => <AssetRow asset={asset} exports={exportsByAsset.get(asset.id) ?? []} key={asset.id} onDelete={() => onAssetDelete(asset.id)} onPublish={() => onAssetPublish(asset)} onRename={() => onAssetRename(asset)} onStatus={(status) => onStatus(asset.id, status)} onView={() => onAssetView(asset)}/>)}</div>
            : <div className="asset-node-no-variants"><span>这个分组还没有资产</span><Button icon={<UploadOutlined/>} onClick={() => onUpload(group)} size="small" type="text">上传资产</Button></div>}
        </div>
      </div>
    </div>

    <div className={`asset-node-region${childrenOpen ? ' open' : ''}`} inert={!childrenOpen}>
      <div className="asset-node-inner">
        {children.map((child) => <AssetGroupBranch
		  {...{ assetPanels, assets, expandedGroups, exportsByAsset, groups, onAssetDelete, onAssetPublish, onAssetRename, onAssetView, onCreateChild, onDelete, onEdit, onGroupPublish, onSelect, onStatus, onToggleAssets, onToggleChildren, onUpload, selectedID }}
          depth={depth + 1}
          group={child}
          key={child.id}
        />)}
      </div>
    </div>
  </div>
}

function AssetRow({ asset, exports, onStatus, onDelete, onView, onRename, onPublish }: { asset: Asset; exports: AssetRemoteExport[]; onStatus: (status: 'adopted' | 'discarded') => void; onDelete: () => void; onView: () => void; onRename: () => void; onPublish: () => void }) {
  const status = statusMeta[asset.status]
  return <div className={`asset-variant-row status-${asset.status}`}>
    <AssetPreview asset={asset} onView={onView}/>
    <div className="asset-variant-info">
      <div><button className="asset-name-button" onClick={onView} type="button"><strong>{asset.name}</strong></button><Tag color={status.color}>{status.label}</Tag></div>
      <span>{asset.media_type} · {asset.source === 'generated' ? '模型生成' : '手动上传'} · {formatBytes(asset.file_size_bytes)}</span>
      {asset.provider_code && <small>{asset.provider_code} / {asset.model_identifier}</small>}
      <div className="asset-storage-badges"><span><HddOutlined/>本地原件</span>{exports.length > 0 && <span className="remote"><CloudOutlined/>S3 × {exports.length}</span>}</div>
    </div>
    <div className="asset-variant-actions">
      <AssetAction icon={<EyeOutlined/>} label="查看内容" onClick={onView}/>
      <AssetAction icon={<EditOutlined/>} label="重命名" onClick={onRename}/>
	  <AssetAction icon={<CloudUploadOutlined/>} label={exports.length ? `管理 ${exports.length} 个 S3 副本` : '发布到 S3'} onClick={onPublish}/>
      {asset.status !== 'adopted' && <AssetAction icon={<FileImageOutlined/>} label={asset.status === 'discarded' ? '重新采用' : '采用'} onClick={() => onStatus('adopted')}/>}
      {asset.status !== 'discarded' && <AssetAction icon={<InboxOutlined/>} label="弃用" onClick={() => onStatus('discarded')}/>}
      <Popconfirm cancelText="取消" description="删除本地资产记录；如有 S3 副本，需要先在发布窗口中单独删除。" okButtonProps={{ danger: true }} okText="永久删除" onConfirm={onDelete} title="确认删除这个资产？">
        <button aria-label="删除资产" className="asset-row-action danger" type="button"><DeleteOutlined/></button>
      </Popconfirm>
    </div>
  </div>
}

function AssetAction({ icon, label, onClick }: { icon: React.ReactNode; label: string; onClick: () => void }) {
  const click = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation()
    onClick()
  }
  return <Tooltip title={label}><button aria-label={label} className="asset-row-action" onClick={click} type="button">{icon}</button></Tooltip>
}

function AssetPreview({ asset, onView }: { asset: Asset; onView: () => void }) {
  const icon = asset.media_type === 'audio' ? <AudioOutlined/> : asset.media_type === 'text' ? <FileTextOutlined/> : asset.media_type === 'video' ? <VideoCameraOutlined/> : <FileImageOutlined/>
  return <button aria-label={`查看 ${asset.name}`} className={`asset-variant-preview ${asset.media_type}`} onClick={onView} type="button">
    {asset.media_type === 'image' ? <img alt="" src={api.assetURL(asset.id)}/> : icon}
    <span>{asset.media_type === 'text' ? 'Markdown' : asset.media_type === 'audio' ? '音频' : asset.media_type === 'video' ? '视频' : asset.mime_type}</span>
  </button>
}

function sortedGroups(groups: AssetGroup[]) {
  return [...groups].sort((left, right) => left.sort_order - right.sort_order || left.name.localeCompare(right.name, 'zh-CN'))
}

function toggledSet(current: Set<string>, id: string) {
  const next = new Set(current)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  return next
}

function changedSet(current: Set<string>, id: string, included: boolean) {
  const next = new Set(current)
  if (included) next.add(id)
  else next.delete(id)
  return next
}

function descendantIDs(rootID: string, groups: AssetGroup[]) {
  const ids = new Set<string>([rootID])
  let changed = true
  while (changed) {
    changed = false
    for (const group of groups) {
      if (group.parent_id && ids.has(group.parent_id) && !ids.has(group.id)) {
        ids.add(group.id)
        changed = true
      }
    }
  }
  return ids
}

function groupPath(group: AssetGroup, groups: AssetGroup[]) {
  const parts = [group.name]
  let parentID = group.parent_id
  while (parentID) {
    const parent = groups.find((item) => item.id === parentID)
    if (!parent) break
    parts.unshift(parent.name)
    parentID = parent.parent_id
  }
  return `${kindMeta[group.kind].label} / ${parts.join(' / ')}`
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

function uploadErrorMessage(reason: unknown) {
  if (reason instanceof Error) return reason.message
  return typeof reason === 'string' ? reason : ''
}
