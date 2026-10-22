import assert from 'node:assert/strict'
import test from 'node:test'
import { checkBuildNode } from './check-build-node.mjs'

for (const version of ['22.23.4', '22.24.0', '24.22.1', '24.23.0']) {
  test(`accepts patched LTS Node ${version}`, () => {
    assert.doesNotThrow(() => checkBuildNode(version))
  })
}

for (const version of ['20.20.2', '22.23.3', '24.18.1', '24.22.0', '26.12.1', '22.23.4-rc.1', '', undefined]) {
  test(`rejects unsupported build Node ${version}`, () => {
    assert.throws(() => checkBuildNode(version))
  })
}
