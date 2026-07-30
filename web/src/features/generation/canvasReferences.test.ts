import { describe, expect, it } from 'vitest'
import type { Asset, CanvasEdgeDTO, CanvasNodeDTO } from '../../api/types'
import { canvasReferenceAssets } from './canvasReferences'

const asset = { id: 'video-asset', name: '已采用成片', media_type: 'video' } as Asset
const source = { id: 'shot-1', data: { kind: 'video', title: '镜头一', selected_video_asset_id: asset.id } } as CanvasNodeDTO
const target = { id: 'shot-2', data: { kind: 'video', title: '镜头二' } } as CanvasNodeDTO

describe('canvas video references', () => {
  it('resolves the selected asset of an upstream storyboard shot', () => {
    const edge = { id: 'edge', source: source.id, target: target.id, type: 'reference' } as CanvasEdgeDTO
    expect(canvasReferenceAssets(target, [source, target], [edge], [asset])).toEqual([asset])
  })

  it('keeps earlier video relations usable as references', () => {
    const edge = { id: 'edge', source: source.id, target: target.id, type: 'relation' } as CanvasEdgeDTO
    expect(canvasReferenceAssets(target, [source, target], [edge], [asset])).toEqual([asset])
  })
})
