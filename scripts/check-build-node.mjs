import assert from 'node:assert/strict'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

export function checkBuildNode(version) {
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(version || '')
  assert.ok(match, `Expected a stable Node.js release, got ${version}`)
  const [major, minor, patch] = match.slice(1).map(Number)
  const floor = { 22: [23, 4], 24: [22, 1] }[major]
  assert.ok(floor && (minor > floor[0] || (minor === floor[0] && patch >= floor[1])),
    `Node.js ${version} is below the supported build security baseline`)
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  checkBuildNode(process.versions.node)
  console.log(JSON.stringify({ node: process.versions.node }))
}
