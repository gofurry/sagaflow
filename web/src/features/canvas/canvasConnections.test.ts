import { describe, expect, it } from 'vitest'
import type { CanvasEdgeDTO } from '../../api/types'
import { normalizeCanvasConnection, repairCanvasEdge } from './canvasConnections'

describe('canvas connections', () => {
  it('keeps the direction and valid handles when a video connects to a note', () => {
    expect(normalizeCanvasConnection(
      { id: 'video', data: { kind: 'video' } },
      { id: 'note', data: { kind: 'note' } },
      'right-source',
      'left-target',
    )).toEqual({ source: 'video', target: 'note', sourceHandle: 'right-source', targetHandle: 'left-target', relation: 'annotation' })
  })

  it('repairs annotation edges saved with reversed handle roles', () => {
    const edge: CanvasEdgeDTO = {
      id: 'edge', source: 'note', target: 'video', source_handle: 'left-target', target_handle: 'right-source',
      type: 'annotation', data: { relation: 'annotation' },
    }
    expect(repairCanvasEdge(edge)).toMatchObject({ source: 'video', target: 'note', source_handle: 'right-source', target_handle: 'left-target' })
  })

  it('treats a storyboard video connected to another shot as a reference', () => {
    expect(normalizeCanvasConnection(
      { id: 'shot-1', data: { kind: 'video' } },
      { id: 'shot-2', data: { kind: 'video' } },
      'right-source',
      'left-target',
    )).toEqual({ source: 'shot-1', target: 'shot-2', sourceHandle: 'right-source', targetHandle: 'left-target', relation: 'reference' })
  })

  it('upgrades video-to-video relations saved by earlier builds', () => {
    const edge: CanvasEdgeDTO = { id: 'edge', source: 'shot-1', target: 'shot-2', type: 'relation', data: { relation: 'relation' } }
    expect(repairCanvasEdge(edge, [
      { id: 'shot-1', data: { kind: 'video' } },
      { id: 'shot-2', data: { kind: 'video' } },
    ])).toMatchObject({ type: 'reference', data: { relation: 'reference' } })
  })
})
