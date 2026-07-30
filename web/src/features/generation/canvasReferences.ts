import type { Asset, CanvasEdgeDTO, CanvasNodeDTO } from '../../api/types'

export function canvasReferenceAssets(shot: CanvasNodeDTO, nodes: CanvasNodeDTO[], edges: CanvasEdgeDTO[], assets: Asset[]) {
  const nodeMap = new Map(nodes.map((node) => [node.id, node]))
  const assetMap = new Map(assets.map((asset) => [asset.id, asset]))
  const seen = new Set<string>()
  return edges
    .filter((edge) => edge.target === shot.id && (edge.type === 'reference' || (edge.type === 'relation' && nodeMap.get(edge.source)?.data.kind === 'video')))
    .map((edge) => {
      const source = nodeMap.get(edge.source)
      return source?.data.asset_id ?? (source?.data.kind === 'video' ? source.data.selected_video_asset_id : undefined)
    })
    .filter((id): id is string => !!id && !seen.has(id) && !!seen.add(id))
    .map((id) => assetMap.get(id))
    .filter((asset): asset is Asset => !!asset)
}
