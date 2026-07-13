import { spawnSync } from 'node:child_process'
import { readFileSync, rmSync, mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const directory = mkdtempSync(join(tmpdir(), 'video-record-openapi-'))
const generated = join(directory, 'generated.ts')

try {
  const result = spawnSync(
    'npx',
    [
      '--yes',
      '--package',
      'openapi-typescript@7.13.0',
      'openapi-typescript',
      '../api/openapi.yaml',
      '-o',
      generated,
    ],
    { cwd: new URL('..', import.meta.url), stdio: 'inherit' },
  )
  if (result.status !== 0) process.exit(result.status ?? 1)
  const expected = readFileSync(new URL('../src/api/generated.ts', import.meta.url))
  const actual = readFileSync(generated)
  if (!expected.equals(actual)) {
    console.error('web/src/api/generated.ts is stale; run npm --prefix web run api:generate')
    process.exit(1)
  }
} finally {
  rmSync(directory, { recursive: true, force: true })
}
