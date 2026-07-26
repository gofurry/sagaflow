import { describe, expect, it } from 'vitest'
import { binarizeMaskAlpha } from './imageMask'

describe('binarizeMaskAlpha', () => {
  it('turns antialiased alpha edges into a strict black-and-white opaque mask', () => {
    const source = new Uint8ClampedArray([
      255, 255, 255, 0,
      255, 255, 255, 127,
      255, 255, 255, 128,
      255, 255, 255, 255,
    ])

    expect([...binarizeMaskAlpha(source)]).toEqual([
      0, 0, 0, 255,
      0, 0, 0, 255,
      255, 255, 255, 255,
      255, 255, 255, 255,
    ])
  })
})
