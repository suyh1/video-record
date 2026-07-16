import { type CSSProperties, useCallback, useState } from 'react'

import type { MediaPalette } from '../../lib/mediaAccent'

type MediaAtmosphereProperties = CSSProperties & {
  '--media-accent': string
  '--media-atmosphere-1': string
  '--media-atmosphere-2': string
  '--media-atmosphere-3': string
}

export function useMediaAtmosphere(identity: string) {
  const [sample, setSample] = useState<{ identity: string; palette: MediaPalette | null } | null>(null)
  const palette = sample?.identity === identity ? sample.palette : null
  const onPaletteChange = useCallback((nextPalette: MediaPalette | null) => {
    setSample({ identity, palette: nextPalette })
  }, [identity])

  return {
    onPaletteChange,
    style: atmosphereProperties(palette),
  }
}

function atmosphereProperties(palette: MediaPalette | null): MediaAtmosphereProperties {
  return {
    '--media-accent': palette?.accent ?? 'var(--brand)',
    '--media-atmosphere-1': palette?.colors[0] ?? 'var(--brand)',
    '--media-atmosphere-2': palette?.colors[1] ?? 'var(--brand)',
    '--media-atmosphere-3': palette?.colors[2] ?? 'var(--brand)',
  }
}
