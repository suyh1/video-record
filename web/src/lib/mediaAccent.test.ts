import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  sampleMediaAccent,
  sampleMediaPalette,
  selectMediaAccent,
  selectMediaPalette,
} from './mediaAccent'

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

describe('selectMediaPalette', () => {
  it('selects three stable and visually distinct colorful groups', () => {
    const data = pixels(
      [238, 48, 38, 255],
      [238, 48, 38, 255],
      [228, 56, 42, 255],
      [35, 82, 235, 255],
      [35, 82, 235, 255],
      [28, 185, 94, 255],
    )

    const first = selectMediaPalette(data)
    const second = selectMediaPalette(data)

    expect(first).not.toBeNull()
    expect(second).toEqual(first)
    expect(first?.colors).toHaveLength(3)
    expect(new Set(first?.colors).size).toBe(3)
    expect(first?.accent).toBe(first?.colors[0])
    const hues = first!.colors.map((color) => parseAccent(color).hue)
    expect(hues.some((hue) => hue >= 15 && hue <= 45)).toBe(true)
    expect(hues.some((hue) => hue >= 245 && hue <= 285)).toBe(true)
    expect(hues.some((hue) => hue >= 130 && hue <= 165)).toBe(true)
  })

  it('fills missing palette slots deterministically from one colorful group', () => {
    const palette = selectMediaPalette(pixels(
      [238, 48, 38, 255],
      [228, 56, 42, 255],
    ))

    expect(palette?.colors).toHaveLength(3)
    expect(new Set(palette?.colors).size).toBe(3)
    expect(selectMediaPalette(pixels(
      [238, 48, 38, 255],
      [228, 56, 42, 255],
    ))).toEqual(palette)
  })

  it('keeps a restrained three-color palette for muted cinematic artwork', () => {
    const palette = selectMediaPalette(pixels(
      [78, 76, 73, 255],
      [104, 100, 95, 255],
      [128, 122, 115, 255],
      [150, 143, 134, 255],
    ))

    expect(palette?.colors).toHaveLength(3)
    expect(new Set(palette?.colors).size).toBe(3)
  })

  it('returns null when no chromatic pixels are usable', () => {
    expect(selectMediaPalette(pixels(
      [72, 72, 72, 255],
      [248, 252, 255, 255],
      [235, 30, 30, 16],
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

  it('samples a three-color palette from the same bounded canvas', () => {
    const image = document.createElement('img')
    const canvas = document.createElement('canvas')
    const drawImage = vi.fn()
    const getImageData = vi.fn(() => ({
      data: pixels(
        [238, 48, 38, 255],
        [35, 82, 235, 255],
        [28, 185, 94, 255],
      ),
    }))
    vi.spyOn(canvas, 'getContext').mockReturnValue({ drawImage, getImageData } as unknown as CanvasRenderingContext2D)
    vi.spyOn(document, 'createElement').mockReturnValue(canvas)

    const palette = sampleMediaPalette(image)

    expect(palette?.colors).toHaveLength(3)
    expect(drawImage).toHaveBeenCalledWith(image, 0, 0, 24, 14)
  })
})
