type OKLabSample = {
  a: number
  b: number
  lightness: number
  weight: number
}

type ColorBucket = OKLabSample & {
  key: string
}

const SAMPLE_WIDTH = 24
const SAMPLE_HEIGHT = 14

export function selectMediaAccent(pixels: Uint8ClampedArray): string | null {
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

  const dominant = Array.from(buckets.values()).sort((left, right) => (
    right.weight - left.weight || left.key.localeCompare(right.key)
  ))[0]
  if (!dominant) return null

  const lightness = clamp(dominant.lightness / dominant.weight, 0.42, 0.72)
  const a = dominant.a / dominant.weight
  const b = dominant.b / dominant.weight
  const chroma = clamp(Math.hypot(a, b), 0.08, 0.18)
  const hue = normalizeHue(Math.atan2(b, a) * 180 / Math.PI)

  return `oklch(${lightness.toFixed(3)} ${chroma.toFixed(3)} ${hue.toFixed(1)})`
}

export function sampleMediaAccent(image: HTMLImageElement): string | null {
  try {
    const canvas = document.createElement('canvas')
    canvas.width = SAMPLE_WIDTH
    canvas.height = SAMPLE_HEIGHT
    const context = canvas.getContext('2d', { willReadFrequently: true })
    if (!context) return null

    context.drawImage(image, 0, 0, SAMPLE_WIDTH, SAMPLE_HEIGHT)
    return selectMediaAccent(context.getImageData(0, 0, SAMPLE_WIDTH, SAMPLE_HEIGHT).data)
  } catch {
    return null
  }
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
