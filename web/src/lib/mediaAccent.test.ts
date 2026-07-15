import { afterEach, describe, expect, it, vi } from 'vitest'

import { sampleMediaAccent, selectMediaAccent } from './mediaAccent'

function pixels(...colors: Array<[number, number, number, number]>) {
  return new Uint8ClampedArray(colors.flat())
}

function parseAccent(accent: string) {
  const match = /^oklch\((\d+\.\d{3}) (\d+\.\d{3}) (\d+\.\d)\)$/.exec(accent)
  if (!match) throw new Error(`Unexpected accent: ${accent}`)
  return {
    lightness: Number(match[1]),
    chroma: Number(match[2]),
    hue: Number(match[3]),
  }
}

describe('selectMediaAccent', () => {
  it('selects a stable dominant colorful pixel group within the atmospheric range', () => {
    const data = pixels(
      [240, 40, 35, 255],
      [240, 40, 35, 255],
      [220, 55, 45, 255],
      [35, 80, 235, 255],
    )

    const first = selectMediaAccent(data)
    const second = selectMediaAccent(data)

    expect(first).not.toBeNull()
    expect(second).toBe(first)
    const accent = parseAccent(first!)
    expect(accent.lightness).toBeGreaterThanOrEqual(0.42)
    expect(accent.lightness).toBeLessThanOrEqual(0.72)
    expect(accent.chroma).toBeGreaterThanOrEqual(0.08)
    expect(accent.chroma).toBeLessThanOrEqual(0.18)
    expect(accent.hue).toBeGreaterThanOrEqual(15)
    expect(accent.hue).toBeLessThanOrEqual(45)
  })

  it('ignores low-chroma grayscale pixels', () => {
    expect(selectMediaAccent(pixels(
      [72, 72, 72, 255],
      [128, 128, 128, 255],
      [190, 190, 190, 255],
    ))).toBeNull()
  })

  it('ignores transparent colorful pixels', () => {
    expect(selectMediaAccent(pixels(
      [235, 30, 30, 0],
      [30, 210, 90, 16],
      [30, 80, 235, 31],
    ))).toBeNull()
  })

  it('ignores near-black and near-white pixels', () => {
    expect(selectMediaAccent(pixels(
      [0, 0, 0, 255],
      [8, 3, 5, 255],
      [248, 252, 255, 255],
      [255, 255, 255, 255],
    ))).toBeNull()
  })
})

describe('sampleMediaAccent', () => {
  afterEach(() => vi.restoreAllMocks())

  it('samples the image through a 24 by 14 canvas', () => {
    const image = document.createElement('img')
    const canvas = document.createElement('canvas')
    const drawImage = vi.fn()
    const getImageData = vi.fn(() => ({
      data: pixels([235, 45, 35, 255]),
    }))
    vi.spyOn(canvas, 'getContext').mockReturnValue({ drawImage, getImageData } as unknown as CanvasRenderingContext2D)
    vi.spyOn(document, 'createElement').mockReturnValue(canvas)

    const accent = sampleMediaAccent(image)

    expect(accent).toMatch(/^oklch\(/)
    expect(canvas.width).toBe(24)
    expect(canvas.height).toBe(14)
    expect(drawImage).toHaveBeenCalledWith(image, 0, 0, 24, 14)
    expect(getImageData).toHaveBeenCalledWith(0, 0, 24, 14)
  })

  it('returns null when canvas sampling is blocked', () => {
    const image = document.createElement('img')
    const canvas = document.createElement('canvas')
    const drawImage = vi.fn(() => {
      throw new DOMException('Canvas is tainted', 'SecurityError')
    })
    vi.spyOn(canvas, 'getContext').mockReturnValue({ drawImage } as unknown as CanvasRenderingContext2D)
    vi.spyOn(document, 'createElement').mockReturnValue(canvas)

    expect(sampleMediaAccent(image)).toBeNull()
  })
})
