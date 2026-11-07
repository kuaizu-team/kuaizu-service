import assert from 'node:assert/strict'
import test from 'node:test'
import { check, decode } from './check-govulncheck.mjs'
const module = 'github.com/labstack/echo/v4'
const fixture = (version = 'v4.16.3', id = 'GO-2026-6872', alias = 'GHSA-246p-cpwv-v3jq') => [
  { config: { scanner_name: 'govulncheck', scanner_version: 'v1.9.0', scan_level: 'symbol', scan_mode: 'source' } },
  { SBOM: { modules: [{ path: module, version }] } },
  { osv: { id, aliases: [alias], references: [{url: 'https://github.com/labstack/echo/security/advisories/' + alias}] } },
  { finding: { osv: id, trace: [{ module, version, function: 'New' }] } },
]
test('only exact upstream-fixed Echo findings are reviewed', () => {
  for (const [id, alias] of [['GO-2026-6872','GHSA-246p-cpwv-v3jq'], ['GO-2026-6875','GHSA-99jh-6h7p-pp36']]) {
    assert.deepEqual(check(fixture('v4.16.3',id,alias)).reviewedUpstreamFixes,[id])
  }
  for (const version of ['v4.15.0','v4.16.2','v4.16.4']) assert.throws(() => check(fixture(version)))
  assert.throws(() => check(fixture('v4.16.3','GO-NEW')))
  assert.throws(() => check(fixture('v4.16.3','GO-2026-6872','wrong-alias')))
  const otherModule = fixture(); otherModule[3].finding.trace[0].module = 'other'
  assert.throws(() => check(otherModule))
  const replaced = fixture(); replaced[1].SBOM.modules[0].replace = { path: 'fork' }
  assert.throws(() => check(replaced))
})
test('missing results, truncated output, and lowered scan levels fail closed', () => {
  assert.throws(() => check([]))
  assert.throws(() => check(fixture().slice(0,1)))
  assert.throws(() => decode('{"config":'))
  const low = fixture(); low[0].config.scan_level = 'module'
  assert.throws(() => check(low))
  const raw = fixture().map(m => JSON.stringify(m,null,2)).join('\n')
  assert.deepEqual(decode(raw),fixture())
  assert.deepEqual(decode(JSON.stringify({progress:{message:'a } { " quote'}})),[{progress:{message:'a } { " quote'}}])
})
test('uncalled findings keep existing govulncheck symbol-level semantics', () => {
  const messages = fixture('v4.15.0'); delete messages[3].finding.trace[0].function
  assert.equal(check(messages).unresolved,0)
})
