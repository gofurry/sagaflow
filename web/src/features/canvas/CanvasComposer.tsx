import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent as ReactMouseEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react'
import { ApartmentOutlined, ArrowRightOutlined, BorderOutlined, BranchesOutlined, DashOutlined, DeleteOutlined, EditOutlined, FileTextOutlined, FullscreenOutlined, MinusOutlined, RadiusSettingOutlined, ReloadOutlined, SelectOutlined, VideoCameraOutlined } from '@ant-design/icons'
import { App, Button, Input, InputNumber, Modal, Popconfirm, Select, Spin } from 'antd'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  addEdge,
  Background,
  BackgroundVariant,
  BaseEdge,
  ConnectionMode,
  Controls,
  EdgeLabelRenderer,
  getBezierPath,
  getSmoothStepPath,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  ViewportPortal,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type EdgeProps,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import { api } from '../../api/client'
import type { Asset, AssetGroup, CanvasAnnotationDTO, CanvasAnnotationKind, CanvasDocument, CanvasEdgeData, CanvasEdgeKind, CanvasEdgeRouting, CanvasNodeData, Episode, Project } from '../../api/types'
import { FloatingToolbar } from '../../components/FloatingToolbar'
import { MarkdownEditor } from '../../components/Markdown'
import { MaterialViewerModal } from '../../components/MaterialViewerModal'

type FlowNode = Node<CanvasNodeData>
type FlowEdge = Edge<CanvasEdgeData>
type SaveState = 'saved' | 'dirty' | 'saving' | 'error'
interface CanvasMenuState { left: number; top: number; flowX: number; flowY: number }
type CanvasAnnotationTool = 'select' | CanvasAnnotationKind
type AnnotationGestureMode = 'move' | 'resize-box' | 'resize-start' | 'resize-end'
interface AnnotationGesture { id: string; mode: AnnotationGestureMode; startX: number; startY: number; original: CanvasAnnotationDTO }

function createCanvasID() {
  const webCrypto = globalThis.crypto
  if (typeof webCrypto?.randomUUID === 'function') return webCrypto.randomUUID()

  const bytes = new Uint8Array(16)
  if (typeof webCrypto?.getRandomValues === 'function') {
    webCrypto.getRandomValues(bytes)
  } else {
    for (let index = 0; index < bytes.length; index += 1) bytes[index] = Math.floor(Math.random() * 256)
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0'))
  return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`
}

const nodeTypes = { asset: AssetCanvasNode, video: VideoCanvasNode, note: NoteCanvasNode }
const edgeTypes = { canvasConnection: CanvasRelationshipEdge }
const CanvasContext = createContext<{
  assets: Map<string, Asset>
  openEdge: (id: string) => void
  removeEdge: (id: string) => void
  updateEdge: (id: string, patch: Partial<CanvasEdgeData>) => void
}>({ assets: new Map(), openEdge: () => undefined, removeEdge: () => undefined, updateEdge: () => undefined })

export function CanvasComposer(props: { project: Project; episode: Episode; onError: (error: unknown) => void }) {
  return <ReactFlowProvider><CanvasInner {...props}/></ReactFlowProvider>
}

function CanvasInner({ project, episode, onError }: { project: Project; episode: Episode; onError: (error: unknown) => void }) {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const flow = useReactFlow<FlowNode, FlowEdge>()
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<FlowEdge>([])
  const [annotations, setAnnotations] = useState<CanvasAnnotationDTO[]>([])
  const [annotationTool, setAnnotationTool] = useState<CanvasAnnotationTool>('select')
  const [annotationDraft, setAnnotationDraft] = useState<CanvasAnnotationDTO | null>(null)
  const [selectedAnnotationID, setSelectedAnnotationID] = useState('')
  const [annotationEditorID, setAnnotationEditorID] = useState('')
  const [loadedEpisode, setLoadedEpisode] = useState('')
  const [menu, setMenu] = useState<CanvasMenuState | null>(null)
  const [nodeEditorID, setNodeEditorID] = useState('')
  const [edgeEditorID, setEdgeEditorID] = useState('')
  const [viewer, setViewer] = useState<Asset | null>(null)
  const [saveState, setSaveState] = useState<SaveState>('saved')
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const saving = useRef(false)
  const queuedDocument = useRef<CanvasDocument | null>(null)
  const lastSaved = useRef('')
  const latestDocument = useRef<CanvasDocument>({ nodes: [], edges: [], annotations: [] })
  const loadedEpisodeRef = useRef('')
  const episodeIDRef = useRef(episode.id)
  const annotationGesture = useRef<AnnotationGesture | null>(null)

  const canvasQuery = useQuery({ queryKey: ['canvas', episode.id], queryFn: () => api.canvas(episode.id) })
  const groupsQuery = useQuery({ queryKey: ['asset-groups', project.id], queryFn: () => api.assetGroups(project.id) })
  const assetsQuery = useQuery({ queryKey: ['assets', project.id], queryFn: () => api.assets(project.id) })
  const groups = useMemo(() => groupsQuery.data ?? [], [groupsQuery.data])
  const assets = useMemo(() => assetsQuery.data ?? [], [assetsQuery.data])
  const assetMap = useMemo(() => new Map(assets.map((asset) => [asset.id, asset])), [assets])
  const adoptedAssets = useMemo(() => assets.filter((asset) => asset.status === 'adopted' && asset.group_id && asset.media_type !== 'video'), [assets])

  const toDocument = useCallback((currentNodes = nodes, currentEdges = edges): CanvasDocument => ({
    nodes: currentNodes.map((node) => ({
      id: node.id,
      type: node.data.kind,
      position: node.position,
      width: node.measured?.width ?? node.width,
      height: node.measured?.height ?? node.height,
      z_index: node.zIndex,
      data: node.data,
    })),
    edges: currentEdges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      source_handle: edge.sourceHandle ?? null,
      target_handle: edge.targetHandle ?? null,
      type: edge.data?.relation ?? 'relation',
      data: edge.data,
    })),
    annotations,
  }), [annotations, edges, nodes])

  const persist = useCallback(async (document: CanvasDocument) => {
    queuedDocument.current = document
    if (saving.current) return
    saving.current = true
    while (queuedDocument.current) {
      const next = queuedDocument.current
      queuedDocument.current = null
      setSaveState('saving')
      try {
        const stored = await api.saveCanvas(episode.id, next)
        lastSaved.current = JSON.stringify(next)
        localStorage.removeItem(`sagaflow.canvas.draft.${episode.id}`)
        queryClient.setQueryData(['canvas', episode.id], stored)
        setSaveState(queuedDocument.current ? 'dirty' : 'saved')
      } catch (error) {
        queuedDocument.current = next
        setSaveState('error')
        onError(error)
        break
      }
    }
    saving.current = false
  }, [episode.id, onError, queryClient])
  const persistRef = useRef(persist)
  persistRef.current = persist

  useEffect(() => {
    if (!canvasQuery.data || groupsQuery.isLoading || assetsQuery.isLoading || loadedEpisode === episode.id) return
    const loadedNodes = canvasQuery.data.nodes.map((node) => ({
      id: node.id,
      type: node.data.kind,
      deletable: node.data.kind !== 'asset',
      position: node.position,
      width: node.width,
      height: node.height,
      zIndex: node.z_index,
      data: refreshAssetNodeData(node.data, assetMap, groups),
    })) as FlowNode[]
    const syncedNodes = syncAdoptedAssets(loadedNodes, adoptedAssets, groups)
    const loadedEdges = canvasQuery.data.edges.map((edge) => flowEdge(edge.id, edge.source, edge.target, edge.type, edge.data, edge.source_handle, edge.target_handle))
    setNodes(syncedNodes)
    setEdges(loadedEdges)
    setAnnotations(canvasQuery.data.annotations ?? [])
    setSelectedAnnotationID('')
    setAnnotationEditorID('')
    setAnnotationTool('select')
    setNodeEditorID('')
    setEdgeEditorID('')
    setLoadedEpisode(episode.id)
    lastSaved.current = JSON.stringify(canvasQuery.data)
    setSaveState(syncedNodes.length === loadedNodes.length ? 'saved' : 'dirty')
    setTimeout(() => flow.fitView({ padding: 0.16, maxZoom: 1 }), 80)
  }, [adoptedAssets, assetMap, assetsQuery.isLoading, canvasQuery.data, episode.id, flow, groups, groupsQuery.isLoading, loadedEpisode, setEdges, setNodes])

  useEffect(() => {
    if (loadedEpisode !== episode.id && !canvasQuery.data) {
      setLoadedEpisode('')
      setNodes([])
      setEdges([])
      setAnnotations([])
      setSelectedAnnotationID('')
      setAnnotationEditorID('')
      setAnnotationTool('select')
      setAnnotationDraft(null)
      setNodeEditorID('')
      setEdgeEditorID('')
    }
  }, [canvasQuery.data, episode.id, loadedEpisode, setEdges, setNodes])

  const document = useMemo(() => toDocument(), [toDocument])
  latestDocument.current = document
  loadedEpisodeRef.current = loadedEpisode
  episodeIDRef.current = episode.id
  const serializedDocument = useMemo(() => JSON.stringify(document), [document])
  useEffect(() => {
    if (loadedEpisode !== episode.id || serializedDocument === lastSaved.current) return
    setSaveState('dirty')
    localStorage.setItem(`sagaflow.canvas.draft.${episode.id}`, serializedDocument)
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => void persist(document), 800)
    return () => { if (saveTimer.current) clearTimeout(saveTimer.current) }
  }, [document, episode.id, loadedEpisode, persist, serializedDocument])

  useEffect(() => () => {
    if (saveTimer.current) clearTimeout(saveTimer.current)
    const pending = latestDocument.current
    if (loadedEpisodeRef.current === episodeIDRef.current && JSON.stringify(pending) !== lastSaved.current) void persistRef.current(pending)
  }, [])

  useEffect(() => {
    if (loadedEpisode !== episode.id) return
    setNodes((current) => syncAdoptedAssets(current.map((node) => ({ ...node, data: refreshAssetNodeData(node.data, assetMap, groups) })), adoptedAssets, groups))
  }, [adoptedAssets, assetMap, episode.id, groups, loadedEpisode, setNodes])

  const editedNode = nodes.find((node) => node.id === nodeEditorID) ?? null
  const editedEdge = edges.find((edge) => edge.id === edgeEditorID) ?? null
  const selectedAnnotation = annotations.find((annotation) => annotation.id === selectedAnnotationID) ?? null
  const editedAnnotation = annotations.find((annotation) => annotation.id === annotationEditorID) ?? null
  const updateNode = useCallback((id: string, patch: Partial<CanvasNodeData>) => setNodes((current) => current.map((node) => node.id === id ? { ...node, data: { ...node.data, ...patch } } : node)), [setNodes])
  const updateEdge = useCallback((id: string, patch: Partial<CanvasEdgeData>) => setEdges((current) => current.map((edge) => edge.id === id ? { ...edge, data: { relation: edge.data?.relation ?? 'relation', routing: edge.data?.routing ?? 'curve', ...edge.data, ...patch } } : edge)), [setEdges])
  const updateAnnotation = useCallback((id: string, patch: Partial<CanvasAnnotationDTO>) => setAnnotations((current) => current.map((annotation) => annotation.id === id ? { ...annotation, ...patch } : annotation)), [])
  const removeAnnotation = useCallback((id: string) => {
    setAnnotations((current) => current.filter((annotation) => annotation.id !== id))
    setSelectedAnnotationID((current) => current === id ? '' : current)
    setAnnotationEditorID((current) => current === id ? '' : current)
  }, [])
  const selectAnnotation = useCallback((id: string) => {
    setSelectedAnnotationID(id)
    setNodes((current) => current.map((node) => node.selected ? { ...node, selected: false } : node))
    setEdges((current) => current.map((edge) => edge.selected ? { ...edge, selected: false } : edge))
    setMenu(null)
  }, [setEdges, setNodes])
  const beginAnnotationGesture = useCallback((id: string, mode: AnnotationGestureMode, event: ReactPointerEvent<Element>) => {
    const annotation = annotations.find((item) => item.id === id)
    if (!annotation) return
    event.preventDefault()
    event.stopPropagation()
    ;(event.currentTarget as Element & { setPointerCapture(pointerId: number): void }).setPointerCapture(event.pointerId)
    annotationGesture.current = { id, mode, startX: event.clientX, startY: event.clientY, original: annotation }
    selectAnnotation(id)
  }, [annotations, selectAnnotation])
  const onCanvasPointerDown = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (annotationTool === 'select' || event.button !== 0) return
    const target = event.target as Element
    if (!target.closest('.react-flow__pane')) return
    event.preventDefault()
    event.stopPropagation()
    event.currentTarget.setPointerCapture(event.pointerId)
    const position = flow.screenToFlowPosition({ x: event.clientX, y: event.clientY })
    setAnnotationDraft({
      id: createCanvasID(), type: annotationTool, position, width: 0, height: 0,
      stroke_color: '#c8753f', stroke_width: 2, line_style: 'solid', opacity: 1, label: '', z_index: 0,
    })
    setSelectedAnnotationID('')
    setMenu(null)
  }, [annotationTool, flow])
  const onCanvasPointerMove = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (annotationDraft) {
      const position = flow.screenToFlowPosition({ x: event.clientX, y: event.clientY })
      const delta = constrainedAnnotationDelta(annotationDraft.position, position, annotationDraft.type, event.shiftKey)
      setAnnotationDraft({ ...annotationDraft, width: delta.width, height: delta.height })
      return
    }
    const gesture = annotationGesture.current
    if (!gesture) return
    const zoom = flow.getZoom()
    const deltaX = (event.clientX - gesture.startX) / zoom
    const deltaY = (event.clientY - gesture.startY) / zoom
    setAnnotations((current) => current.map((annotation) => annotation.id === gesture.id ? transformAnnotation(gesture, deltaX, deltaY) : annotation))
  }, [annotationDraft, flow])
  const onCanvasPointerUp = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (annotationDraft) {
      const completed = completeAnnotation(annotationDraft)
      setAnnotationDraft(null)
      setAnnotationTool('select')
      if (completed) {
        setAnnotations((current) => [...current, completed])
        setSelectedAnnotationID(completed.id)
      }
    }
    annotationGesture.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  }, [annotationDraft])
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      if (target?.closest('input, textarea, [contenteditable="true"]')) return
      if (event.key === 'Escape') {
        setAnnotationDraft(null)
        setAnnotationTool('select')
        setSelectedAnnotationID('')
        setAnnotationEditorID('')
        annotationGesture.current = null
        return
      }
      if (selectedAnnotationID && (event.key === 'Delete' || event.key === 'Backspace')) {
        event.preventDefault()
        event.stopPropagation()
        removeAnnotation(selectedAnnotationID)
      }
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [removeAnnotation, selectedAnnotationID])
  const removeEdge = useCallback((id: string) => {
    setEdges((current) => current.filter((edge) => edge.id !== id))
    setEdgeEditorID((current) => current === id ? '' : current)
  }, [setEdges])
  const removeNode = useCallback((id: string) => {
    const target = nodes.find((node) => node.id === id)
    if (!target || target.data.kind === 'asset') return
    setNodes((current) => current.filter((node) => node.id !== id))
    setEdges((current) => current.filter((edge) => edge.source !== id && edge.target !== id))
    setNodeEditorID('')
  }, [nodes, setEdges, setNodes])
  const addNodeAt = useCallback((kind: 'video' | 'note', position?: { x: number; y: number }) => {
    const center = position ?? flow.screenToFlowPosition({ x: window.innerWidth * .52, y: window.innerHeight * .48 })
    const shotNumber = nextShotNumber(nodes)
    const data: CanvasNodeData = kind === 'video'
      ? { kind: 'video', title: `第 ${shotNumber} 镜`, body: '', shot_number: shotNumber, target_duration_seconds: 5 }
      : { kind: 'note', title: '创作备注', body: '', color: 'sand' }
    const node: FlowNode = { id: createCanvasID(), type: kind, position: center, data }
    setNodes((current) => [...current, node])
    setNodeEditorID(node.id)
    setMenu(null)
  }, [flow, nodes, setNodes])
  const syncAssets = () => {
    setNodes((current) => syncAdoptedAssets(current, adoptedAssets, groups))
    message.success('已同步采用资产')
    setMenu(null)
  }
  const arrange = () => {
    setNodes((current) => arrangeCanvasNodes(current, adoptedAssets, groups))
    setTimeout(() => flow.fitView({ padding: .16, maxZoom: 1 }), 80)
    setMenu(null)
  }
  const onConnect = useCallback((connection: Connection) => {
    const source = nodes.find((node) => node.id === connection.source)
    const target = nodes.find((node) => node.id === connection.target)
    if (!source || !target || source.id === target.id) return
    const normalized = normalizeConnection(source, target, connection.sourceHandle, connection.targetHandle)
    const duplicate = edges.some((edge) => edge.data?.relation === normalized.relation && ((edge.source === normalized.source && edge.target === normalized.target) || (edge.source === normalized.target && edge.target === normalized.source)))
    if (duplicate) {
      message.info('这两个节点已经建立了关系')
      return
    }
    setEdges((current) => addEdge(flowEdge(createCanvasID(), normalized.source, normalized.target, normalized.relation, { relation: normalized.relation, routing: 'curve' }, normalized.sourceHandle, normalized.targetHandle), current))
  }, [edges, message, nodes, setEdges])
  const onPaneContextMenu = (event: MouseEvent | ReactMouseEvent<Element>) => {
    event.preventDefault()
    if (annotationTool !== 'select') {
      setAnnotationDraft(null)
      setAnnotationTool('select')
      return
    }
    const bounds = (event.currentTarget as Element).getBoundingClientRect()
    const flowPosition = flow.screenToFlowPosition({ x: event.clientX, y: event.clientY })
    setMenu({
      left: Math.max(8, Math.min(event.clientX - bounds.left, bounds.width - 198)),
      top: Math.max(8, Math.min(event.clientY - bounds.top, bounds.height - 220)),
      flowX: flowPosition.x,
      flowY: flowPosition.y,
    })
  }
  const retrySave = () => void persist(queuedDocument.current ?? document)

  if (canvasQuery.isLoading || groupsQuery.isLoading || assetsQuery.isLoading) return <div className="canvas-loading"><Spin size="large"/><span>载入分集画布…</span></div>
  return <div
    className={`canvas-shell annotation-tool-${annotationTool}`}
    onContextMenu={(event) => event.preventDefault()}
    onPointerDownCapture={onCanvasPointerDown}
    onPointerMove={onCanvasPointerMove}
    onPointerUp={onCanvasPointerUp}
  >
    <FloatingToolbar ariaLabel="画布工具栏" items={[
      { key: 'video', label: '新建视频分镜', icon: <VideoCameraOutlined/>, active: true, onClick: () => addNodeAt('video') },
      { key: 'note', label: '新建备注', icon: <FileTextOutlined/>, onClick: () => addNodeAt('note') },
      { key: 'sync', label: '同步已采用资产', icon: <ReloadOutlined/>, onClick: syncAssets },
      { key: 'arrange', label: '自动整理画布', icon: <FullscreenOutlined/>, onClick: arrange },
    ]}/>
    <CanvasContext.Provider value={{ assets: assetMap, openEdge: setEdgeEditorID, removeEdge, updateEdge }}>
      <ReactFlow
        connectionLineStyle={{ stroke: '#c8753f', strokeWidth: 2.4 }}
        connectionMode={ConnectionMode.Loose}
        defaultEdgeOptions={{ type: 'canvasConnection' }}
        deleteKeyCode={['Backspace', 'Delete']}
        edges={edges}
        edgeTypes={edgeTypes}
        fitView
        maxZoom={2}
        minZoom={0.15}
        nodes={nodes}
        nodesDraggable={annotationTool === 'select'}
        nodeTypes={nodeTypes}
        onConnect={onConnect}
        onEdgeClick={() => { setSelectedAnnotationID(''); setMenu(null) }}
        onEdgeDoubleClick={(event, edge) => { event.preventDefault(); event.stopPropagation(); setEdgeEditorID(edge.id); setMenu(null) }}
        onEdgesChange={onEdgesChange}
        onNodeClick={() => { setSelectedAnnotationID(''); setMenu(null) }}
        onNodeDoubleClick={(event, node) => {
          event.preventDefault()
          event.stopPropagation()
          setMenu(null)
          if (node.data.kind === 'asset') {
            const asset = node.data.asset_id ? assetMap.get(node.data.asset_id) : undefined
            if (asset) setViewer(asset)
            return
          }
          setNodeEditorID(node.id)
        }}
        onNodesChange={onNodesChange}
        onPaneClick={() => { if (annotationTool === 'select') setSelectedAnnotationID(''); setMenu(null) }}
        onPaneContextMenu={onPaneContextMenu}
        panOnDrag={annotationTool === 'select'}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#d8cdbf" gap={25} size={1.1} variant={BackgroundVariant.Dots}/>
        <ViewportPortal>
          {annotations.map((annotation) => <CanvasAnnotation
            annotation={annotation}
            key={annotation.id}
            onDoubleClick={() => setAnnotationEditorID(annotation.id)}
            onGestureStart={beginAnnotationGesture}
            onSelect={selectAnnotation}
            selected={annotation.id === selectedAnnotationID}
          />)}
          {annotationDraft && <CanvasAnnotation annotation={annotationDraft} draft/>}
        </ViewportPortal>
        <Controls position="bottom-left"/>
        <MiniMap pannable zoomable nodeColor={(node) => nodeColor((node.data as CanvasNodeData).kind)}/>
      </ReactFlow>
    </CanvasContext.Provider>

    <CanvasAnnotationToolbar activeTool={annotationTool} onChange={(tool) => { setAnnotationTool(tool); setAnnotationDraft(null); setSelectedAnnotationID(''); setMenu(null) }}/>
    {selectedAnnotation && <CanvasAnnotationStylebar annotation={selectedAnnotation} onChange={updateAnnotation} onDelete={removeAnnotation} onEdit={setAnnotationEditorID}/>}

    <button className={`canvas-save-state ${saveState}`} onClick={saveState === 'error' ? retrySave : undefined} type="button">
      <span/>{saveState === 'saving' ? '保存中' : saveState === 'dirty' ? '等待保存' : saveState === 'error' ? '保存失败 · 点击重试' : '已自动保存'}
    </button>
    {menu && <div className="canvas-context-menu" style={{ left: menu.left, top: menu.top }}>
      <button onClick={() => addNodeAt('video', { x: menu.flowX, y: menu.flowY })} type="button"><VideoCameraOutlined/><span>新建视频分镜</span></button>
      <button onClick={() => addNodeAt('note', { x: menu.flowX, y: menu.flowY })} type="button"><FileTextOutlined/><span>新建备注</span></button>
      <i/>
      <button onClick={syncAssets} type="button"><ReloadOutlined/><span>同步已采用资产</span></button>
      <button onClick={arrange} type="button"><FullscreenOutlined/><span>自动整理画布</span></button>
    </div>}
    <CanvasNodeModal
      node={editedNode}
      onChange={updateNode}
      onClose={() => setNodeEditorID('')}
      onDelete={removeNode}
    />
    <CanvasEdgeModal edge={editedEdge} onChange={updateEdge} onClose={() => setEdgeEditorID('')} onDelete={removeEdge}/>
    <CanvasAnnotationModal annotation={editedAnnotation} onChange={updateAnnotation} onClose={() => setAnnotationEditorID('')} onDelete={removeAnnotation}/>
    <MaterialViewerModal item={viewer} onClose={() => setViewer(null)} open={!!viewer} url={viewer ? api.assetURL(viewer.id) : ''}/>
  </div>
}

const annotationTools: Array<{ key: CanvasAnnotationTool; label: string; icon: ReactNode }> = [
  { key: 'select', label: '选择', icon: <SelectOutlined/> },
  { key: 'arrow', label: '箭头', icon: <ArrowRightOutlined/> },
  { key: 'line', label: '线', icon: <MinusOutlined/> },
  { key: 'rectangle', label: '矩形框', icon: <BorderOutlined/> },
  { key: 'ellipse', label: '椭圆框', icon: <RadiusSettingOutlined/> },
]

const annotationColors = [
  { label: '橘色', value: '#c8753f' },
  { label: '棕灰', value: '#75685f' },
  { label: '青绿', value: '#748b78' },
  { label: '砖红', value: '#b45e52' },
]

function CanvasAnnotationToolbar({ activeTool, onChange }: { activeTool: CanvasAnnotationTool; onChange: (tool: CanvasAnnotationTool) => void }) {
  return <div aria-label="画布标注工具" className="canvas-annotation-toolbar" role="toolbar">
    {annotationTools.map((tool, index) => <span className={index === 1 ? 'with-divider' : ''} key={tool.key}>
      <button aria-label={tool.label} aria-pressed={activeTool === tool.key} className={activeTool === tool.key ? 'active' : ''} onClick={() => onChange(tool.key)} title={tool.label} type="button">{tool.icon}<small>{tool.label}</small></button>
    </span>)}
  </div>
}

function CanvasAnnotationStylebar({ annotation, onChange, onDelete, onEdit }: {
  annotation: CanvasAnnotationDTO
  onChange: (id: string, patch: Partial<CanvasAnnotationDTO>) => void
  onDelete: (id: string) => void
  onEdit: (id: string) => void
}) {
  return <div aria-label="标注样式" className="canvas-annotation-stylebar" role="toolbar">
    <div className="canvas-annotation-colors">{annotationColors.map((color) => <button aria-label={color.label} className={annotation.stroke_color === color.value ? 'active' : ''} key={color.value} onClick={() => onChange(annotation.id, { stroke_color: color.value })} style={{ '--annotation-color': color.value } as CSSProperties} title={color.label} type="button"/>)}</div>
    <i/>
    {[1.5, 2.5, 4].map((width) => <button aria-label={`${width} 像素线宽`} className={`stroke-width${annotation.stroke_width === width ? ' active' : ''}`} key={width} onClick={() => onChange(annotation.id, { stroke_width: width })} title={`${width} 像素`} type="button"><span style={{ height: width }}/></button>)}
    <button aria-label="切换虚线" className={annotation.line_style === 'dashed' ? 'active' : ''} onClick={() => onChange(annotation.id, { line_style: annotation.line_style === 'dashed' ? 'solid' : 'dashed' })} title="实线 / 虚线" type="button"><DashOutlined/></button>
    <i/>
    <button aria-label="编辑说明" onClick={() => onEdit(annotation.id)} title="编辑说明" type="button"><EditOutlined/></button>
    <button aria-label="删除标注" className="danger" onClick={() => onDelete(annotation.id)} title="删除标注" type="button"><DeleteOutlined/></button>
  </div>
}

function CanvasAnnotationModal({ annotation, onChange, onClose, onDelete }: {
  annotation: CanvasAnnotationDTO | null
  onChange: (id: string, patch: Partial<CanvasAnnotationDTO>) => void
  onClose: () => void
  onDelete: (id: string) => void
}) {
  if (!annotation) return null
  return <Modal
    className="canvas-edit-modal"
    footer={<div className="canvas-modal-footer"><Popconfirm cancelText="取消" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => onDelete(annotation.id)} title="删除这个画布标注？"><Button danger icon={<DeleteOutlined/>}>删除</Button></Popconfirm><Button onClick={onClose} type="primary">完成</Button></div>}
    onCancel={onClose}
    open
    title={<div className="canvas-modal-title"><span>画布标注</span><strong>{annotationTypeLabel(annotation.type)}</strong></div>}
    width={520}
  >
    <div className="canvas-modal-form">
      <label><span>说明文字</span><Input.TextArea autoFocus maxLength={300} onChange={(event) => onChange(annotation.id, { label: event.target.value })} placeholder="可选，说明这个箭头、线条或框选区域的含义。" rows={5} showCount value={annotation.label}/></label>
      <small>标注只用于画布表达，不会进入模型 Prompt、资产关系或视频参考。</small>
    </div>
  </Modal>
}

function CanvasAnnotation({ annotation, selected = false, draft = false, onSelect, onDoubleClick, onGestureStart }: {
  annotation: CanvasAnnotationDTO
  selected?: boolean
  draft?: boolean
  onSelect?: (id: string) => void
  onDoubleClick?: () => void
  onGestureStart?: (id: string, mode: AnnotationGestureMode, event: ReactPointerEvent<Element>) => void
}) {
  const commonClass = `canvas-annotation ${annotation.type}${selected ? ' selected' : ''}${draft ? ' draft' : ''}`
  const dash = annotation.line_style === 'dashed' ? '9 7' : undefined
  const open = (event: ReactMouseEvent<Element>) => { event.preventDefault(); event.stopPropagation(); onDoubleClick?.() }
  const select = (event: ReactMouseEvent<Element>) => { event.preventDefault(); event.stopPropagation(); onSelect?.(annotation.id) }
  if (annotation.type === 'line' || annotation.type === 'arrow') {
    const padding = 18
    const endX = annotation.position.x + annotation.width
    const endY = annotation.position.y + annotation.height
    const left = Math.min(annotation.position.x, endX) - padding
    const top = Math.min(annotation.position.y, endY) - padding
    const width = Math.max(Math.abs(annotation.width), 1) + padding * 2
    const height = Math.max(Math.abs(annotation.height), 1) + padding * 2
    const x1 = annotation.position.x - left
    const y1 = annotation.position.y - top
    const x2 = endX - left
    const y2 = endY - top
    const markerID = `canvas-annotation-arrow-${annotation.id.replaceAll('-', '')}`
    return <div className={commonClass} style={{ left, top, width, height }}>
      <svg aria-label={annotationTypeLabel(annotation.type)} height="100%" onClick={select} onDoubleClick={open} role="button" width="100%">
        {annotation.type === 'arrow' && <defs><marker id={markerID} markerHeight="7" markerUnits="strokeWidth" markerWidth="7" orient="auto" refX="6.4" refY="3.5"><path d="M 0 0 L 7 3.5 L 0 7 z" fill={annotation.stroke_color}/></marker></defs>}
        <line className="canvas-annotation-hit" onPointerDown={(event) => onGestureStart?.(annotation.id, 'move', event)} x1={x1} x2={x2} y1={y1} y2={y2}/>
        <line markerEnd={annotation.type === 'arrow' ? `url(#${markerID})` : undefined} opacity={annotation.opacity} pointerEvents="none" stroke={annotation.stroke_color} strokeDasharray={dash} strokeLinecap="round" strokeWidth={annotation.stroke_width} vectorEffect="non-scaling-stroke" x1={x1} x2={x2} y1={y1} y2={y2}/>
      </svg>
      {annotation.label && <button className="canvas-annotation-label" onClick={select} onDoubleClick={open} onPointerDown={(event) => onGestureStart?.(annotation.id, 'move', event)} style={{ left: (x1 + x2) / 2, top: (y1 + y2) / 2 }} title="双击编辑说明" type="button">{annotation.label}</button>}
      {selected && <>
        <button aria-label="调整起点" className="canvas-annotation-point start" onPointerDown={(event) => onGestureStart?.(annotation.id, 'resize-start', event)} style={{ left: x1, top: y1 }} type="button"/>
        <button aria-label="调整终点" className="canvas-annotation-point end" onPointerDown={(event) => onGestureStart?.(annotation.id, 'resize-end', event)} style={{ left: x2, top: y2 }} type="button"/>
      </>}
    </div>
  }
  const shape = annotation.type === 'rectangle'
    ? <rect className="canvas-annotation-stroke" height={annotation.height} onPointerDown={(event) => onGestureStart?.(annotation.id, 'move', event)} rx="10" width={annotation.width} x="9" y="9"/>
    : <ellipse className="canvas-annotation-stroke" cx={annotation.width / 2 + 9} cy={annotation.height / 2 + 9} onPointerDown={(event) => onGestureStart?.(annotation.id, 'move', event)} rx={annotation.width / 2} ry={annotation.height / 2}/>
  return <div className={commonClass} style={{ left: annotation.position.x - 9, top: annotation.position.y - 9, width: annotation.width + 18, height: annotation.height + 18 }}>
    <svg aria-label={annotationTypeLabel(annotation.type)} height="100%" onClick={select} onDoubleClick={open} role="button" style={{ color: annotation.stroke_color, opacity: annotation.opacity }} width="100%">
      {shape}
      {annotation.type === 'rectangle'
        ? <rect fill="none" height={annotation.height} pointerEvents="none" rx="10" stroke="currentColor" strokeDasharray={dash} strokeWidth={annotation.stroke_width} vectorEffect="non-scaling-stroke" width={annotation.width} x="9" y="9"/>
        : <ellipse cx={annotation.width / 2 + 9} cy={annotation.height / 2 + 9} fill="none" pointerEvents="none" rx={annotation.width / 2} ry={annotation.height / 2} stroke="currentColor" strokeDasharray={dash} strokeWidth={annotation.stroke_width} vectorEffect="non-scaling-stroke"/>}
    </svg>
    {annotation.label && <button className="canvas-annotation-label shape-label" onClick={select} onDoubleClick={open} onPointerDown={(event) => onGestureStart?.(annotation.id, 'move', event)} style={{ left: annotation.width / 2 + 9, top: annotation.height / 2 + 9 }} title="双击编辑说明" type="button">{annotation.label}</button>}
    {selected && <button aria-label="调整大小" className="canvas-annotation-point resize" onPointerDown={(event) => onGestureStart?.(annotation.id, 'resize-box', event)} style={{ left: annotation.width + 9, top: annotation.height + 9 }} type="button"/>}
  </div>
}

function constrainedAnnotationDelta(start: { x: number; y: number }, end: { x: number; y: number }, type: CanvasAnnotationKind, constrain: boolean) {
  let width = end.x - start.x
  let height = end.y - start.y
  if (!constrain) return { width, height }
  if (type === 'line' || type === 'arrow') {
    if (Math.abs(width) >= Math.abs(height)) height = 0
    else width = 0
    return { width, height }
  }
  const size = Math.max(Math.abs(width), Math.abs(height))
  width = Math.sign(width || 1) * size
  height = Math.sign(height || 1) * size
  return { width, height }
}

function completeAnnotation(annotation: CanvasAnnotationDTO) {
  if (annotation.type === 'line' || annotation.type === 'arrow') return Math.hypot(annotation.width, annotation.height) >= 16 ? annotation : null
  const width = Math.abs(annotation.width)
  const height = Math.abs(annotation.height)
  if (width < 24 || height < 24) return null
  return {
    ...annotation,
    position: {
      x: annotation.width < 0 ? annotation.position.x + annotation.width : annotation.position.x,
      y: annotation.height < 0 ? annotation.position.y + annotation.height : annotation.position.y,
    },
    width,
    height,
  }
}

function transformAnnotation(gesture: AnnotationGesture, deltaX: number, deltaY: number) {
  const original = gesture.original
  if (gesture.mode === 'move') return { ...original, position: { x: original.position.x + deltaX, y: original.position.y + deltaY } }
  if (gesture.mode === 'resize-box') return { ...original, width: Math.max(24, original.width + deltaX), height: Math.max(24, original.height + deltaY) }
  if (gesture.mode === 'resize-start') {
    const position = { x: original.position.x + deltaX, y: original.position.y + deltaY }
    const candidate = { ...original, position, width: original.position.x + original.width - position.x, height: original.position.y + original.height - position.y }
    return Math.hypot(candidate.width, candidate.height) >= 8 ? candidate : original
  }
  const candidate = { ...original, width: original.width + deltaX, height: original.height + deltaY }
  return Math.hypot(candidate.width, candidate.height) >= 8 ? candidate : original
}

function annotationTypeLabel(type: CanvasAnnotationKind) {
  return type === 'arrow' ? '箭头' : type === 'line' ? '线' : type === 'rectangle' ? '矩形框' : '椭圆框'
}

function NodeHandles() {
  return <>
    <Handle className="canvas-node-handle canvas-node-handle-left" id="left-target" position={Position.Left} type="target"/>
    <Handle className="canvas-node-handle canvas-node-handle-right" id="right-source" position={Position.Right} type="source"/>
  </>
}

function AssetCanvasNode({ data, selected }: NodeProps<FlowNode>) {
  const { assets } = useContext(CanvasContext)
  const asset = data.asset_id ? assets.get(data.asset_id) : undefined
  const textQuery = useQuery({
    queryKey: ['canvas-asset-text', asset?.id],
    queryFn: async () => { const response = await fetch(api.assetURL(asset!.id), { credentials: 'include' }); if (!response.ok) throw new Error('无法读取文本资产'); return response.text() },
    enabled: asset?.media_type === 'text',
    staleTime: Infinity,
  })
  return <article className={`canvas-content-node canvas-asset-node${selected ? ' selected' : ''}${asset?.status === 'discarded' ? ' unavailable' : ''}`}>
    <NodeHandles/>
    <div className={`canvas-node-media ${asset?.media_type ?? data.media_type ?? 'file'}`}>
      {asset?.media_type === 'image' && <img alt="" loading="lazy" src={api.assetURL(asset.id)}/>}
      {asset?.media_type === 'video' && <video muted preload="metadata" src={api.assetProxyURL(asset.id)}/>}
      {asset?.media_type === 'audio' && <audio controls preload="metadata" src={api.assetProxyURL(asset.id)}/>}
      {asset?.media_type === 'text' && <p>{plainText(textQuery.data ?? '读取 Markdown…').slice(0, 180)}</p>}
      {asset && !['image', 'video', 'audio', 'text'].includes(asset.media_type) && <FileTextOutlined/>}
    </div>
    <div className="canvas-node-caption"><strong>{asset?.name ?? data.title}</strong><span>{data.group_path || mediaLabel(asset?.media_type ?? data.media_type)}</span></div>
  </article>
}

function VideoCanvasNode({ data, selected }: NodeProps<FlowNode>) {
  const { assets } = useContext(CanvasContext)
  const selectedAsset = data.selected_video_asset_id ? assets.get(data.selected_video_asset_id) : undefined
  return <article className={`canvas-content-node canvas-video-node${selected ? ' selected' : ''}`}>
    <NodeHandles/>
    <div className="canvas-shot-number">{String(data.shot_number ?? 0).padStart(2, '0')}</div>
    <div className="canvas-node-media video">
      {selectedAsset ? <video muted preload="metadata" src={api.assetProxyURL(selectedAsset.id)}/> : <VideoCameraOutlined/>}
    </div>
    <div className="canvas-node-caption"><strong>{data.title}</strong><span>{data.target_duration_seconds ? `${data.target_duration_seconds} 秒` : '未设时长'} · {selectedAsset ? '已选择成片' : '待生成'}</span></div>
  </article>
}

function NoteCanvasNode({ data, selected }: NodeProps<FlowNode>) {
  return <article className={`canvas-content-node canvas-note-node color-${data.color ?? 'sand'}${selected ? ' selected' : ''}`}>
    <NodeHandles/>
    <strong>{data.title}</strong>
    <p>{plainText(data.body ?? '').slice(0, 220) || '双击卡片编辑备注'}</p>
  </article>
}

function CanvasRelationshipEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data, selected, markerEnd, markerStart }: EdgeProps<FlowEdge>) {
  const { openEdge, removeEdge, updateEdge } = useContext(CanvasContext)
  const routing = data?.routing ?? 'curve'
  const pathResult = routing === 'step'
    ? getSmoothStepPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, borderRadius: 16, offset: 28 })
    : getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, curvature: .38 })
  const [path, labelX, labelY] = pathResult
  const relation = data?.relation ?? 'relation'
  const note = data?.note?.trim()
  const color = relation === 'reference' ? '#c8753f' : relation === 'annotation' ? '#9b887b' : '#b37f5d'
  return <>
    <BaseEdge
      id={id}
      interactionWidth={30}
      markerEnd={markerEnd}
      markerStart={markerStart}
      path={path}
      style={{ stroke: color, strokeDasharray: relation === 'annotation' ? '7 6' : undefined, strokeLinecap: 'round', strokeWidth: selected ? 3.2 : 2.2 }}
    />
    <EdgeLabelRenderer>
      {note && <button
        className="canvas-edge-note nodrag nopan"
        onClick={(event) => event.stopPropagation()}
        onDoubleClick={(event) => { event.preventDefault(); event.stopPropagation(); openEdge(id) }}
        style={{ transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)` }}
        title="双击编辑连线备注"
        type="button"
      >{note}</button>}
      {selected && <div className="canvas-edge-toolbar nodrag nopan" style={{ transform: `translate(-50%, -50%) translate(${labelX}px,${labelY + (note ? 38 : 0)}px)` }}>
        <button aria-label="设为曲线" className={routing === 'curve' ? 'active' : ''} onClick={(event) => { event.stopPropagation(); updateEdge(id, { routing: 'curve' }) }} title="曲线" type="button"><BranchesOutlined/></button>
        <button aria-label="设为折线" className={routing === 'step' ? 'active' : ''} onClick={(event) => { event.stopPropagation(); updateEdge(id, { routing: 'step' }) }} title="折线" type="button"><ApartmentOutlined/></button>
        <button aria-label="编辑连线备注" onClick={(event) => { event.stopPropagation(); openEdge(id) }} title="备注" type="button"><EditOutlined/></button>
        <button aria-label="删除连线" className="danger" onClick={(event) => { event.stopPropagation(); removeEdge(id) }} title="删除" type="button"><DeleteOutlined/></button>
      </div>}
    </EdgeLabelRenderer>
  </>
}

function CanvasNodeModal({ node, onChange, onClose, onDelete }: {
  node: FlowNode | null
  onChange: (id: string, patch: Partial<CanvasNodeData>) => void
  onClose: () => void
  onDelete: (id: string) => void
}) {
  if (!node || node.data.kind === 'asset') return null
  const change = (patch: Partial<CanvasNodeData>) => onChange(node.id, patch)
  const kindLabel = node.data.kind === 'video' ? '视频分镜' : '备注'
  const footer = <div className="canvas-modal-footer"><Popconfirm cancelText="取消" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => onDelete(node.id)} title={`从画布删除这个${kindLabel}？`}><Button danger icon={<DeleteOutlined/>}>删除</Button></Popconfirm><Button onClick={onClose} type="primary">完成</Button></div>
  return <Modal className="canvas-edit-modal" footer={footer} onCancel={onClose} open title={<div className="canvas-modal-title"><span>{kindLabel}</span><strong>{node.data.title}</strong></div>} width={node.data.kind === 'note' ? 860 : 720}>
    {node.data.kind === 'video' && <div className="canvas-modal-form">
      <label><span>分镜标题</span><Input onChange={(event) => change({ title: event.target.value })} value={node.data.title}/></label>
      <div className="canvas-modal-grid">
        <label><span>镜号</span><InputNumber min={1} onChange={(value) => value && change({ shot_number: value })} value={node.data.shot_number}/></label>
        <label><span>目标时长（秒）</span><InputNumber min={1} onChange={(value) => change({ target_duration_seconds: value ?? undefined })} value={node.data.target_duration_seconds}/></label>
      </div>
      <label><span>画面描述</span><Input.TextArea autoSize={{ minRows: 8 }} onChange={(event) => change({ body: event.target.value })} placeholder="描述构图、动作、镜头运动和节奏；视频生成页会带入这段内容。" value={node.data.body}/></label>
    </div>}
    {node.data.kind === 'note' && <div className="canvas-modal-form">
      <label><span>标题</span><Input onChange={(event) => change({ title: event.target.value })} value={node.data.title}/></label>
      <label><span>颜色</span><div className="canvas-note-colors">{['sand', 'orange', 'sage', 'rose'].map((color) => <button aria-label={color} className={`${color}${node.data.color === color ? ' active' : ''}`} key={color} onClick={() => change({ color })} type="button"/>)}</div></label>
      <label><span>备注内容</span><MarkdownEditor height={360} onChange={(body) => change({ body })} value={node.data.body}/></label>
      <small>备注连线只表达说明关系，不会自动加入生成 Prompt。</small>
    </div>}
  </Modal>
}

function CanvasEdgeModal({ edge, onChange, onClose, onDelete }: {
  edge: FlowEdge | null
  onChange: (id: string, patch: Partial<CanvasEdgeData>) => void
  onClose: () => void
  onDelete: (id: string) => void
}) {
  if (!edge) return null
  const routing = edge.data?.routing ?? 'curve'
  return <Modal
    className="canvas-edit-modal canvas-edge-modal"
    footer={<div className="canvas-modal-footer"><Popconfirm cancelText="取消" okButtonProps={{ danger: true }} okText="删除" onConfirm={() => onDelete(edge.id)} title="删除这条连线？"><Button danger icon={<DeleteOutlined/>}>删除连线</Button></Popconfirm><Button onClick={onClose} type="primary">完成</Button></div>}
    onCancel={onClose}
    open
    title={<div className="canvas-modal-title"><span>连线</span><strong>关系与备注</strong></div>}
    width={560}
  >
    <div className="canvas-modal-form">
      <label><span>线条样式</span><Select onChange={(value: CanvasEdgeRouting) => onChange(edge.id, { routing: value })} options={[{ label: '曲线', value: 'curve' }, { label: '折线', value: 'step' }]} value={routing}/></label>
      <label><span>连线备注</span><Input.TextArea maxLength={300} onChange={(event) => onChange(edge.id, { note: event.target.value })} placeholder="说明这两个节点之间的关系；备注会直接显示在连线上。" rows={5} showCount value={edge.data?.note ?? ''}/></label>
      <small>双击连线可以随时重新打开这里。关系备注只用于画布表达，不会自动加入生成 Prompt。</small>
    </div>
  </Modal>
}

function normalizeConnection(sourceNode: FlowNode, targetNode: FlowNode, sourceHandle?: string | null, targetHandle?: string | null) {
  let source = sourceNode
  let target = targetNode
  let normalizedSourceHandle = sourceHandle
  let normalizedTargetHandle = targetHandle
  let relation: CanvasEdgeKind = 'relation'
  if (sourceNode.data.kind === 'note' || targetNode.data.kind === 'note') {
    relation = 'annotation'
    if (targetNode.data.kind === 'note' && sourceNode.data.kind !== 'note') {
      source = targetNode
      target = sourceNode
      normalizedSourceHandle = targetHandle
      normalizedTargetHandle = sourceHandle
    }
  } else if ((sourceNode.data.kind === 'asset' && targetNode.data.kind === 'video') || (sourceNode.data.kind === 'video' && targetNode.data.kind === 'asset')) {
    relation = 'reference'
    if (sourceNode.data.kind === 'video') {
      source = targetNode
      target = sourceNode
      normalizedSourceHandle = targetHandle
      normalizedTargetHandle = sourceHandle
    }
  }
  return { source: source.id, target: target.id, sourceHandle: normalizedSourceHandle, targetHandle: normalizedTargetHandle, relation }
}

function flowEdge(id: string, source: string, target: string, relation: CanvasEdgeKind, data?: CanvasEdgeData, sourceHandle?: string | null, targetHandle?: string | null): FlowEdge {
  return {
    id,
    source,
    target,
    sourceHandle: normalizeHandleID(sourceHandle),
    targetHandle: normalizeHandleID(targetHandle),
    type: 'canvasConnection',
    data: { ...data, relation, routing: data?.routing === 'step' ? 'step' : 'curve' },
    deletable: true,
    selectable: true,
  }
}

function normalizeHandleID(handle?: string | null) {
  if (handle === 'left') return 'left-target'
  if (handle === 'right') return 'right-source'
  return handle ?? undefined
}

function refreshAssetNodeData(data: CanvasNodeData, assets: Map<string, Asset>, groups: AssetGroup[]): CanvasNodeData {
  if (data.kind !== 'asset' || !data.asset_id) return data
  const asset = assets.get(data.asset_id)
  if (!asset) return data
  return { ...data, title: asset.name, media_type: asset.media_type, group_id: asset.group_id, group_path: asset.group_id ? groupPath(asset.group_id, groups) : '分集视频', asset_status: asset.status }
}

function syncAdoptedAssets(nodes: FlowNode[], adoptedAssets: Asset[], groups: AssetGroup[]) {
  const eligibleNodes = nodes.filter((node) => node.data.kind !== 'asset' || node.data.media_type !== 'video')
  const existing = new Set(eligibleNodes.filter((node) => node.data.kind === 'asset').map((node) => node.data.asset_id))
  const positions = assetTreePositions(adoptedAssets, groups)
  const additions = adoptedAssets.filter((asset) => !existing.has(asset.id)).map((asset) => assetNode(asset, positions.get(asset.id) ?? { x: 90, y: 90 }, groups))
  return additions.length || eligibleNodes.length !== nodes.length ? [...eligibleNodes, ...additions] : nodes
}

function arrangeCanvasNodes(nodes: FlowNode[], adoptedAssets: Asset[], groups: AssetGroup[]) {
  const positions = assetTreePositions(adoptedAssets, groups)
  const assetKinds = activeAssetKinds(adoptedAssets, groups)
  let noteY = 90
  let videoY = 90
  const notesX = 90 + assetKinds.length * 340
  const videosX = notesX + 340
  const videos = [...nodes].filter((node) => node.data.kind === 'video').sort((left, right) => (left.data.shot_number ?? 0) - (right.data.shot_number ?? 0))
  const videoPositions = new Map(videos.map((node) => {
    const position = { x: videosX, y: videoY }
    videoY += Math.max(252, estimatedNodeHeight(node) + 34)
    return [node.id, position]
  }))
  const notePositions = new Map(nodes.filter((node) => node.data.kind === 'note').map((node) => {
    const position = { x: notesX, y: noteY }
    noteY += Math.max(190, estimatedNodeHeight(node) + 62)
    return [node.id, position]
  }))
  return nodes.map((node) => {
    if (node.data.kind === 'asset' && node.data.asset_id && positions.has(node.data.asset_id)) return { ...node, position: positions.get(node.data.asset_id)! }
    if (node.data.kind === 'video' && videoPositions.has(node.id)) return { ...node, position: videoPositions.get(node.id)! }
    if (node.data.kind === 'note' && notePositions.has(node.id)) return { ...node, position: notePositions.get(node.id)! }
    return node
  })
}

function assetTreePositions(assets: Asset[], groups: AssetGroup[]) {
  const kinds = activeAssetKinds(assets, groups)
  const groupOrder = groupTreeOrder(groups)
  const byKindRow = new Map<string, number>()
  const positions = new Map<string, { x: number; y: number }>()
  const sorted = [...assets].sort((left, right) => (groupOrder.get(left.group_id ?? '') ?? 9999) - (groupOrder.get(right.group_id ?? '') ?? 9999) || left.name.localeCompare(right.name, 'zh-CN'))
  for (const asset of sorted) {
    const group = groups.find((item) => item.id === asset.group_id)
    const kind = group?.kind ?? 'material'
    const kindIndex = Math.max(0, kinds.indexOf(kind))
    const row = byKindRow.get(kind) ?? 0
    byKindRow.set(kind, row + 1)
    positions.set(asset.id, { x: 90 + kindIndex * 340 + Math.min(groupDepth(group, groups), 4) * 18, y: 90 + row * 220 })
  }
  return positions
}

function activeAssetKinds(assets: Asset[], groups: AssetGroup[]) {
  const order = ['character', 'scene', 'prop', 'material']
  const used = new Set<string>(assets.map((asset) => groups.find((group) => group.id === asset.group_id)?.kind ?? 'material'))
  const active = order.filter((kind) => used.has(kind))
  return active.length ? active : ['material']
}

function estimatedNodeHeight(node: FlowNode) {
  return node.measured?.height ?? node.height ?? (node.data.kind === 'video' ? 218 : node.data.kind === 'note' ? 128 : node.data.media_type === 'audio' ? 142 : 198)
}

function groupTreeOrder(groups: AssetGroup[]) {
  const result = new Map<string, number>()
  let index = 0
  const visit = (parentID: string | null, kind: string) => {
    groups.filter((group) => group.kind === kind && group.parent_id === parentID).sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name, 'zh-CN')).forEach((group) => { result.set(group.id, index++); visit(group.id, kind) })
  }
  ;['character', 'scene', 'prop', 'material'].forEach((kind) => visit(null, kind))
  return result
}

function groupDepth(group: AssetGroup | undefined, groups: AssetGroup[]) {
  let depth = 0
  let current = group
  while (current?.parent_id && depth < 12) { depth += 1; current = groups.find((item) => item.id === current?.parent_id) }
  return depth
}

function groupPath(groupID: string, groups: AssetGroup[]) {
  const names: string[] = []
  let current = groups.find((group) => group.id === groupID)
  while (current && names.length < 16) { names.unshift(current.name); current = current.parent_id ? groups.find((group) => group.id === current?.parent_id) : undefined }
  return names.join(' / ')
}

function assetNode(asset: Asset, position: { x: number; y: number }, groups: AssetGroup[]): FlowNode {
  return { id: createCanvasID(), type: 'asset', deletable: false, position, data: { kind: 'asset', title: asset.name, asset_id: asset.id, media_type: asset.media_type, group_id: asset.group_id, group_path: asset.group_id ? groupPath(asset.group_id, groups) : '分集视频', asset_status: asset.status } }
}

function nextShotNumber(nodes: FlowNode[]) { return Math.max(0, ...nodes.filter((node) => node.data.kind === 'video').map((node) => node.data.shot_number ?? 0)) + 1 }
function plainText(value: string) { return value.replace(/```[\s\S]*?```/g, ' ').replace(/[#>*_`|()\u005B\u005D-]/g, ' ').replace(/\s+/g, ' ').trim() }
function mediaLabel(value?: string) { return ({ image: '图像', video: '视频', audio: '音频', text: 'Markdown', file: '文件' } as Record<string, string>)[value ?? ''] ?? '资产' }
function nodeColor(kind?: string) { return ({ asset: '#d39a62', video: '#c8753f', note: '#9a806f' } as Record<string, string>)[kind ?? ''] ?? '#9a806f' }
