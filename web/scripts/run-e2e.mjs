import { spawn } from 'node:child_process'
import { rm } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { createConnection } from 'node:net'
import { resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'

import { playwrightEnvironment, startSyntheticTMDB, syntheticTMDBPort } from './e2e-environment.mjs'

const require = createRequire(import.meta.url)
const projectRoot = resolve(import.meta.dirname, '../..')
const dataDir = resolve(projectRoot, '.tmp/e2e-data')
const playwrightCLI = require.resolve('@playwright/test/cli')

await rm(dataDir, { force: true, recursive: true })
const syntheticToken = ['e2e', 'tmdb', 'token'].join('-')
const syntheticTMDB = await startSyntheticTMDB({ token: syntheticToken })
const environment = playwrightEnvironment({
  ...process.env,
  E2E_TMDB_ORIGIN: syntheticTMDB.origin,
  TMDB_API_BASE_URL: syntheticTMDB.baseURL,
  TMDB_IMAGE_BASE_URL: syntheticTMDB.imageBaseURL,
  TMDB_READ_ACCESS_TOKEN: syntheticToken,
})

const child = spawn(process.execPath, [playwrightCLI, 'test', ...process.argv.slice(2)], {
  cwd: resolve(projectRoot, 'web'),
  env: environment,
  stdio: 'inherit',
})

const exitCode = await new Promise((resolveExit) => {
  child.on('exit', (code, signal) => resolveExit(signal ? 1 : (code ?? 1)))
})

await syntheticTMDB.close()
await Promise.all([waitForPortRelease(18081), waitForPortRelease(15173), waitForPortRelease(syntheticTMDBPort)])
await rm(dataDir, { force: true, recursive: true })
process.exitCode = exitCode

async function waitForPortRelease(port) {
  const deadline = Date.now() + 10_000
  while (Date.now() < deadline) {
    if (!await portAcceptsConnections(port)) return
    await delay(100)
  }
  throw new Error(`E2E web server on port ${port} did not stop`)
}

function portAcceptsConnections(port) {
  return new Promise((resolveConnection) => {
    const socket = createConnection({ host: '127.0.0.1', port })
    socket.setTimeout(250)
    socket.once('connect', () => {
      socket.destroy()
      resolveConnection(true)
    })
    socket.once('error', () => resolveConnection(false))
    socket.once('timeout', () => {
      socket.destroy()
      resolveConnection(false)
    })
  })
}
