import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

// Upstream explicitly fixes these in v4.16.3; Go's records omit the v4 fixed event.
// Exact-version corrections only. Reassess rather than extending to future versions.
const fixes = {
  'GO-2026-6872': 'GHSA-246p-cpwv-v3jq',
  'GO-2026-6875': 'GHSA-99jh-6h7p-pp36',
}
const echoModule = 'github.com/labstack/echo/v4'

export function decode(raw) {
  const messages = []
  let start = 0, depth = 0, quoted = false, escaped = false
  for (let i = 0; i < raw.length; i++) {
    const c = raw[i]
    if (quoted) {
      if (escaped) escaped = false
      else if (c === '\\') escaped = true
      else if (c === '"') quoted = false
    } else if (c === '"') quoted = true
    else if (c === '{') depth++
    else if (c === '}') {
      if (--depth === 0) { messages.push(JSON.parse(raw.slice(start, i + 1))); start = i + 1 }
    }
  }
  assert.ok(depth === 0 && !quoted && !raw.slice(start).trim(), 'Incomplete scanner output')
  return messages
}

export function check(messages) {
  const configs = messages.filter(m => m.config).map(m => m.config)
  assert.equal(configs.length, 1, 'Missing scanner configuration')
  const config = configs[0]
  assert.ok(config.scanner_name === 'govulncheck' && config.scanner_version === 'v1.9.0' && config.scan_level === 'symbol' && ['source', 'binary'].includes(config.scan_mode), 'Unexpected scanner configuration')
  const sboms = messages.filter(m => m.SBOM).map(m => m.SBOM)
  assert.equal(sboms.length, 1, 'Missing scanner SBOM')
  assert.ok(sboms[0].modules?.length, 'Empty scanner SBOM')
  const echo = sboms[0].modules.find(m => m.path === echoModule)
  const advisories = new Map(messages.filter(m => m.osv).map(m => [m.osv.id, m.osv]))
  const reviewed = new Set(), blocked = new Set()
  for (const message of messages) {
    assert.ok(!message.error, 'Scanner error')
    if (!message.finding) continue
    const f = message.finding
    assert.ok(f.osv && f.trace?.length, 'Invalid finding')
    const frame = f.trace[0]
    if (!frame.function) continue // Match govulncheck's symbol-level gate.
    const alias = fixes[f.osv]
    const osv = advisories.get(f.osv)
    if (alias && echo?.version === 'v4.16.3' && !echo.replace && frame.module === echoModule && frame.version === 'v4.16.3' && osv?.aliases?.includes(alias) && osv.references?.some(r => r.url === 'https://github.com/labstack/echo/security/advisories/' + alias)) {
      reviewed.add(f.osv)
    } else blocked.add(f.osv)
  }
  assert.equal(blocked.size, 0, 'Unresolved reachable vulnerabilities: ' + [...blocked].join(', '))
  return { unresolved: 0, reviewedUpstreamFixes: [...reviewed].sort() }
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  console.log(JSON.stringify(check(decode(readFileSync(process.argv[2], 'utf8')))))
}
