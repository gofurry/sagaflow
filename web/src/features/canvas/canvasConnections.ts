import type { CanvasEdgeDTO, CanvasEdgeKind, CanvasNodeData } from '../../api/types'

type ConnectableNode = { id: string; data: Pick<CanvasNodeData, 'kind'> }

export function normalizeCanvasConnection(
  sourceNode: ConnectableNode,
  targetNode: ConnectableNode,
  sourceHandle?: string | null,
  targetHandle?: string | null,
) {
  let source = sourceNode
  let target = targetNode
  let relation: CanvasEdgeKind = 'relation'

  if (sourceNode.data.kind === 'note' || targetNode.data.kind === 'note') {
    relation = 'annotation'
  } else if ((sourceNode.data.kind === 'asset' && targetNode.data.kind === 'video') || (sourceNode.data.kind === 'video' && targetNode.data.kind === 'asset')) {
    relation = 'reference'
    if (sourceNode.data.kind === 'video') {
      source = targetNode
      target = sourceNode
      sourceHandle = undefined
      targetHandle = undefined
    }
  }

  return {
    source: source.id,
    target: target.id,
    sourceHandle: sourceEndpointHandle(sourceHandle),
    targetHandle: targetEndpointHandle(targetHandle),
    relation,
  }
}

export function repairCanvasEdge(edge: CanvasEdgeDTO): CanvasEdgeDTO {
  const sourceHandle = normalizeHandleID(edge.source_handle)
  const targetHandle = normalizeHandleID(edge.target_handle)
  if (edge.type === 'annotation' && sourceHandle === 'left-target' && targetHandle === 'right-source') {
    return {
      ...edge,
      source: edge.target,
      target: edge.source,
      source_handle: 'right-source',
      target_handle: 'left-target',
    }
  }
  return {
    ...edge,
    source_handle: sourceEndpointHandle(sourceHandle),
    target_handle: targetEndpointHandle(targetHandle),
  }
}

export function normalizeHandleID(handle?: string | null) {
  if (handle === 'left') return 'left-target'
  if (handle === 'right') return 'right-source'
  return handle ?? undefined
}

function sourceEndpointHandle(handle?: string | null) {
  const normalized = normalizeHandleID(handle)
  return normalized === 'right-source' ? normalized : undefined
}

function targetEndpointHandle(handle?: string | null) {
  const normalized = normalizeHandleID(handle)
  return normalized === 'left-target' ? normalized : undefined
}
