import { createServer } from 'node:http'

export const syntheticTMDBPort = 18082

export function playwrightEnvironment(source) {
  const environment = { ...source }
  delete environment.NO_COLOR
  return environment
}

export async function startSyntheticTMDB({ port = syntheticTMDBPort, token } = {}) {
  if (!token) throw new Error('Synthetic TMDB token is required')
  const counts = new Map()
  const failingIDs = new Set()
  const origin = `http://127.0.0.1:${port}`
  const images = syntheticImages()
  const server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', origin)
    if (url.pathname === '/__counts') {
      return writeJSON(response, 200, Object.fromEntries(counts))
    }
    if (url.pathname === '/__control' && request.method === 'POST') {
      const body = await readJSON(request)
      failingIDs.clear()
      for (const id of body.failingIds ?? []) failingIDs.add(Number(id))
      if (body.resetCounts) counts.clear()
      return writeJSON(response, 200, { failingIds: [...failingIDs] })
    }
    if (url.pathname.startsWith('/images/')) {
      const image = images.get(url.pathname)
      if (!image) return writeJSON(response, 404, { status_message: 'not found' })
      response.writeHead(200, {
        'Cache-Control': 'public, max-age=3600',
        'Content-Length': image.length,
        'Content-Type': 'image/bmp',
      })
      response.end(image)
      return
    }
    if (request.headers.authorization !== `Bearer ${token}`) {
      return writeJSON(response, 401, { status_message: 'unauthorized' })
    }
    counts.set(url.pathname, (counts.get(url.pathname) ?? 0) + 1)
    const mediaID = Number(url.pathname.match(/^\/3\/(?:movie|tv)\/(\d+)/)?.[1] ?? 0)
    if (failingIDs.has(mediaID)) return writeJSON(response, 502, { status_message: 'synthetic failure' })
    const payload = tmdbPayload(url.pathname, origin)
    if (!payload) return writeJSON(response, 404, { status_message: 'not found' })
    return writeJSON(response, 200, payload)
  })
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(port, '127.0.0.1', resolve)
  })
  return {
    baseURL: `${origin}/3`,
    origin,
    close: () => new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve())),
  }
}

function tmdbPayload(pathname, origin) {
  const image = (name) => `${origin}/images/${name}.bmp`
  if (pathname === '/3/search/multi') return { page: 1, results: [], total_pages: 1, total_results: 0 }
  if (pathname === '/3/tv/1001') {
    return {
      id: 1001,
      name: '潮汐档案',
      original_name: 'Tidal Archive',
      first_air_date: '2025-01-01',
      poster_path: image('tide-poster'),
      backdrop_path: image('tide-backdrop'),
      overview: '海岸观测站的三名记录员，在潮汐数据中发现一段跨越多年的失踪航线。',
      number_of_seasons: 2,
      number_of_episodes: 5,
      episode_run_time: [47],
      genres: [{ id: 18, name: '剧情' }, { id: 9648, name: '悬疑' }],
      seasons: [
        { id: 101, name: '第 1 季', overview: '', poster_path: image('tide-poster'), air_date: '2025-01-01', season_number: 1, episode_count: 3 },
        { id: 102, name: '第 2 季', overview: '', poster_path: image('tide-season-two'), air_date: '2026-07-01', season_number: 2, episode_count: 2 },
      ],
    }
  }
  if (pathname === '/3/tv/1001/credits') {
    return {
      cast: [
        { id: 1, name: '林见川', character: '顾潮', profile_path: image('cast-one'), order: 0 },
        { id: 2, name: '周聆', character: '许栖', profile_path: image('cast-two'), order: 1 },
        { id: 3, name: '陈望', character: '罗远', profile_path: image('cast-three'), order: 2 },
        { id: 4, name: '季宁', character: '站长', profile_path: '', order: 3 },
      ],
    }
  }
  if (pathname === '/3/tv/1001/season/1') {
    return {
      id: 101, name: '第 1 季', overview: '', poster_path: image('tide-poster'), air_date: '2025-01-01', season_number: 1,
      episodes: [
        { id: 1101, name: '低潮线', overview: '', air_date: '2025-01-01', season_number: 1, episode_number: 1, runtime: 45, still_path: image('still-one') },
        { id: 1102, name: '回声浮标', overview: '', air_date: '2025-01-08', season_number: 1, episode_number: 2, runtime: 47, still_path: image('still-two') },
        { id: 1103, name: '无人航道', overview: '', air_date: '2025-01-15', season_number: 1, episode_number: 3, runtime: 49, still_path: image('still-three') },
      ],
    }
  }
  if (pathname === '/3/tv/1001/season/2') {
    return {
      id: 102, name: '第 2 季', overview: '', poster_path: image('tide-season-two'), air_date: '2026-07-01', season_number: 2,
      episodes: [
        { id: 1201, name: '重返北堤', overview: '', air_date: '2026-07-01', season_number: 2, episode_number: 1, runtime: 48, still_path: image('still-four') },
        { id: 1202, name: '潮汐尽头', overview: '', air_date: '2026-07-08', season_number: 2, episode_number: 2, runtime: 52, still_path: image('still-five') },
      ],
    }
  }
  if (pathname === '/3/movie/2002') {
    return {
      id: 2002,
      title: '静默轨道',
      original_title: 'Silent Track',
      release_date: '2024-09-20',
      poster_path: image('silent-poster'),
      backdrop_path: image('silent-backdrop'),
      overview: '一名声音工程师沿着废弃铁路寻找最后一段未被数字化的现场录音。',
      runtime: 112,
      genres: [{ id: 18, name: '剧情' }],
    }
  }
  if (pathname === '/3/movie/2002/credits') {
    return { cast: [{ id: 5, name: '程默', character: '沈言', profile_path: image('cast-four'), order: 0 }] }
  }
  if (pathname === '/3/movie/3003') {
    return {
      id: 3003, title: '缓存样片', original_title: 'Cache Sample', release_date: '2026-01-01',
      poster_path: '', backdrop_path: '', overview: '', runtime: 90, genres: [],
    }
  }
  return null
}

function syntheticImages() {
  return new Map([
    ['/images/tide-backdrop.bmp', createBitmap(720, 405, [[13, 31, 45], [92, 54, 50], [224, 151, 91]], 0.2)],
    ['/images/tide-poster.bmp', createBitmap(300, 450, [[15, 42, 54], [139, 72, 54], [235, 178, 105]], 0.7)],
    ['/images/tide-season-two.bmp', createBitmap(300, 450, [[32, 38, 48], [54, 94, 89], [189, 174, 113]], 1.1)],
    ['/images/silent-backdrop.bmp', createBitmap(720, 405, [[29, 30, 33], [68, 72, 76], [198, 161, 105]], 1.7)],
    ['/images/silent-poster.bmp', createBitmap(300, 450, [[24, 25, 28], [84, 75, 65], [208, 174, 119]], 2.1)],
    ['/images/cast-one.bmp', createBitmap(180, 270, [[31, 48, 56], [126, 82, 66], [224, 174, 124]], 0.4)],
    ['/images/cast-two.bmp', createBitmap(180, 270, [[44, 38, 47], [126, 75, 83], [231, 177, 144]], 0.9)],
    ['/images/cast-three.bmp', createBitmap(180, 270, [[32, 45, 43], [78, 104, 87], [207, 177, 118]], 1.4)],
    ['/images/cast-four.bmp', createBitmap(180, 270, [[37, 37, 40], [98, 85, 69], [218, 180, 126]], 1.9)],
    ['/images/still-one.bmp', createBitmap(480, 270, [[17, 41, 54], [57, 78, 78], [202, 157, 99]], 0.1)],
    ['/images/still-two.bmp', createBitmap(480, 270, [[18, 33, 48], [91, 61, 59], [218, 142, 86]], 0.6)],
    ['/images/still-three.bmp', createBitmap(480, 270, [[24, 36, 44], [61, 82, 76], [185, 160, 105]], 1.1)],
    ['/images/still-four.bmp', createBitmap(480, 270, [[31, 38, 49], [46, 87, 84], [184, 170, 111]], 1.6)],
    ['/images/still-five.bmp', createBitmap(480, 270, [[21, 32, 43], [84, 72, 67], [210, 157, 99]], 2.2)],
  ])
}

function createBitmap(width, height, [top, bottom, accent], seed) {
  const rowSize = Math.ceil(width * 3 / 4) * 4
  const imageSize = rowSize * height
  const buffer = Buffer.alloc(54 + imageSize)
  buffer.write('BM', 0)
  buffer.writeUInt32LE(buffer.length, 2)
  buffer.writeUInt32LE(54, 10)
  buffer.writeUInt32LE(40, 14)
  buffer.writeInt32LE(width, 18)
  buffer.writeInt32LE(height, 22)
  buffer.writeUInt16LE(1, 26)
  buffer.writeUInt16LE(24, 28)
  buffer.writeUInt32LE(imageSize, 34)
  for (let row = 0; row < height; row += 1) {
    const y = 1 - row / Math.max(height - 1, 1)
    for (let x = 0; x < width; x += 1) {
      const nx = x / Math.max(width - 1, 1)
      const glow = Math.max(0, 1 - Math.hypot(nx - 0.7, y - 0.36) * 1.55)
      const texture = (Math.sin((nx * 5.2 + y * 2.3 + seed) * Math.PI) + 1) * 0.025
      const offset = 54 + row * rowSize + x * 3
      const channels = top.map((value, index) => value * (1 - y) + bottom[index] * y + accent[index] * glow * 0.34 + 255 * texture)
      buffer[offset] = clamp(channels[2])
      buffer[offset + 1] = clamp(channels[1])
      buffer[offset + 2] = clamp(channels[0])
    }
  }
  return buffer
}

function clamp(value) {
  return Math.max(0, Math.min(255, Math.round(value)))
}

async function readJSON(request) {
  const chunks = []
  let bytes = 0
  for await (const chunk of request) {
    bytes += chunk.length
    if (bytes > 64 * 1024) throw new Error('Synthetic TMDB control body is too large')
    chunks.push(chunk)
  }
  if (chunks.length === 0) return {}
  return JSON.parse(Buffer.concat(chunks).toString('utf8'))
}

function writeJSON(response, status, payload) {
  const body = Buffer.from(JSON.stringify(payload))
  response.writeHead(status, { 'Content-Length': body.length, 'Content-Type': 'application/json' })
  response.end(body)
}
