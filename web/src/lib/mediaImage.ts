const proxyImagePath = /^\/api\/v1\/public\/tmdb\/images\/(?:w300|w342|w780|w1280)\/[A-Za-z0-9_-]+\.(?:jpg|jpeg|png|webp)$/
const signedHeroProxyImage = /^\/api\/v1\/public\/tmdb\/images\/w1280\/[A-Za-z0-9_-]+\.(?:jpg|jpeg|png|webp)\?(?:expires=\d+&signature=[a-f0-9]{64}|signature=[a-f0-9]{64}&expires=\d+)$/i
const signaturePattern = /^[a-f0-9]{64}$/i

export function mediaImageURL(source: string | null | undefined): string | null {
  if (!source || source !== source.trim()) return null

  const applicationOrigin = window.location.origin
  let parsed: URL
  try {
    parsed = new URL(source, applicationOrigin)
  } catch {
    return null
  }

  if (source.startsWith('/') && !source.startsWith('//')) {
    const expires = parsed.searchParams.get('expires') ?? ''
    const signature = parsed.searchParams.get('signature') ?? ''
    return parsed.origin === applicationOrigin
      && proxyImagePath.test(parsed.pathname)
      && /^\d+$/.test(expires)
      && signaturePattern.test(signature)
      ? source
      : null
  }

  if (!/^https?:\/\//i.test(source) || parsed.origin === applicationOrigin) return null
  const hostname = normalizeHostname(parsed.hostname)
  if (hostname === 'image.tmdb.org' || hostname.endsWith('.image.tmdb.org')) return null
  return source
}

export function signedTMDBProxyImageURL(source: string | null | undefined): string | null {
  return source && source === source.trim() && signedHeroProxyImage.test(source) ? source : null
}

function normalizeHostname(hostname: string) {
  return hostname
    .replace(/[\u3002\uff0e\uff61]/g, '.')
    .toLocaleLowerCase('en-US')
    .replace(/\.+$/, '')
}
