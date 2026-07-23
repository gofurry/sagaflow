import type { AssetRemoteExport } from '../../api/types'

export function usableAssetExports(exports: AssetRemoteExport[], assetID: string) {
  return exports
    .filter((item) => item.asset_id === assetID && item.state === 'ready' && item.connection_enabled)
    .sort((left, right) => Number(right.connection_is_default) - Number(left.connection_is_default) || right.created_at.localeCompare(left.created_at))
}

export function preferredAssetExport(exports: AssetRemoteExport[], assetID: string) {
  return usableAssetExports(exports, assetID)[0]
}

export function groupAssetExports(exports: AssetRemoteExport[]) {
  const grouped = new Map<string, AssetRemoteExport[]>()
  for (const item of exports) {
    const current = grouped.get(item.asset_id) ?? []
    current.push(item)
    grouped.set(item.asset_id, current)
  }
  return grouped
}

export function assetExportLabel(item: AssetRemoteExport) {
  return item.connection_name || 'S3 连接'
}
