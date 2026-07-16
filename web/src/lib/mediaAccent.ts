type OKLabSample = {
  a: number
  b: number
  lightness: number
  weight: number
}

type ColorBucket = OKLabSample & {
  key: string
}

type PaletteSample = OKLabSample & {
  chroma: number
  hue: number
  key: string
}

export type MediaPalette = {
  accent: string
  colors: [string, string, string]
}

const SAMPLE_WIDTH = 24
const SAMPLE_HEIGHT = 14
const MINIMUM_PALETTE_DISTANCE = 0.08

export function selectMediaAccent(pixels: Uint8ClampedArray): string | null {
  return selectMediaPalette(pixels)?.accent ?? null
}

export function selectMediaPalette(pixels: Uint8ClampedArray): MediaPalette | null {
  const buckets = new Map<string, ColorBucket>()

  for (let index = 0; index + 3 < pixels.length; index += 4) {
    const alpha = pixels[index + 3]! / 255
    if (alpha < 0.2) continue

    const sample = rgbToOKLab(pixels[index]!, pixels[index + 1]!, pixels[index + 2]!)
    const chroma = Math.hypot(sample.a, sample.b)
    if (sample.lightness < 0.18 || sample.lightness > 0.94 || chroma < 0.04) continue

    const hue = normalizeHue(Math.atan2(sample.b, sample.a) * 180 / Math.PI)
    const key = `${Math.floor(hue / 30)}:${Math.floor(sample.lightness / 0.08)}:${Math.floor(chroma / 0.05)}`
    const bucket = buckets.get(key)
    if (bucket) {
      bucket.lightness += sample.lightness * alpha
      bucket.a += sample.a * alpha
      bucket.b += sample.b * alpha
      bucket.weight += alpha
    } else {
      buckets.set(key, {
        a: sample.a * alpha,
        b: sample.b * alpha,
        key,
        lightness: sample.lightness * alpha,
        weight: alpha,
      })
    }
  }

  const candidates = Array.from(buckets.values())
    .map(normalizeBucket)
    .sort((left, right) => (
      right.weight - left.weight
      || right.chroma - left.chroma
      || left.key.localeCompare(right.key)
    ))
  if (candidates.length === 0) return null

  const selected: PaletteSample[] = []
  for (const candidate of candidates) {
    if (selected.every((current) => paletteDistance(current, candidate) >= MINIMUM_PALETTE_DISTANCE)) {
      selected.push(candidate)
    }
    if (selected.length === 3) break
  }

  const colors = selected.map(formatPaletteSample)
  const base = selected[0]!
  const fallbackOffsets = [105, 215, 55, 160, 270]
  for (const offset of fallbackOffsets) {
    if (colors.length === 3) break
    const hue = normalizeHue(base.hue + offset)
    if (selected.some((sample) => hueDistance(sample.hue, hue) < 42)) continue
    const fallback: PaletteSample = {
      a: Math.cos(hue * Math.PI / 180) * base.chroma,
      b: Math.sin(hue * Math.PI / 180) * base.chroma,
      chroma: base.chroma,
      hue,
      key: `fallback:${offset}`,
      lightness: clamp(base.lightness + (colors.length % 2 === 0 ? 0.04 : -0.03), 0.42, 0.72),
      weight: 0,
    }
    selected.push(fallback)
    colors.push(formatPaletteSample(fallback))
  }

  const paletteColors = colors.slice(0, 3) as [string, string, string]
  return { accent: paletteColors[0], colors: paletteColors }
}

export function sampleMediaAccent(image: HTMLImageElement): string | null {
  return sampleMediaPalette(image)?.accent ?? null
}

export function sampleMediaPalette(image: HTMLImageElement): MediaPalette | null {
  try {
    const canvas = document.createElement('canvas')
    canvas.width = SAMPLE_WIDTH
    canvas.height = SAMPLE_HEIGHT
    const context = canvas.getContext('2d', { willReadFrequently: true })
    if (!context) return null

    context.drawImage(image, 0, 0, SAMPLE_WIDTH, SAMPLE_HEIGHT)
    return selectMediaPalette(context.getImageData(0, 0, SAMPLE_WIDTH, SAMPLE_HEIGHT).data)
  } catch {
    return null
  }
}

function normalizeBucket(bucket: ColorBucket): PaletteSample {
  const lightness = bucket.lightness / bucket.weight
  const a = bucket.a / bucket.weight
  const b = bucket.b / bucket.weight
  return {
    a,
    b,
    chroma: Math.hypot(a, b),
    hue: normalizeHue(Math.atan2(b, a) * 180 / Math.PI),
    key: bucket.key,
    lightness,
    weight: bucket.weight,
  }
}

function formatPaletteSample(sample: PaletteSample) {
  const lightness = clamp(sample.lightness, 0.42, 0.72)
  const chroma = clamp(sample.chroma, 0.08, 0.18)
  return `oklch(${lightness.toFixed(3)} ${chroma.toFixed(3)} ${sample.hue.toFixed(1)})`
}

function paletteDistance(left: PaletteSample, right: PaletteSample) {
  return Math.hypot(
    left.lightness - right.lightness,
    left.a - right.a,
    left.b - right.b,
  )
}

function hueDistance(left: number, right: number) {
  const distance = Math.abs(left - right) % 360
  return Math.min(distance, 360 - distance)
}

function rgbToOKLab(red: number, green: number, blue: number): OKLabSample {
  const r = srgbToLinear(red / 255)
  const g = srgbToLinear(green / 255)
  const b = srgbToLinear(blue / 255)
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)

  return {
    lightness: 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    a: 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    b: 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
    weight: 1,
  }
}

function srgbToLinear(value: number) {
  return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
}

function normalizeHue(hue: number) {
  return (hue % 360 + 360) % 360
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value))
}
