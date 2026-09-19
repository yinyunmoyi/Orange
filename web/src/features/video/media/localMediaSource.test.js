import assert from 'node:assert/strict'
import test from 'node:test'
import {
  fingerprintParts,
  VIDEO_PICKER_OPTIONS,
} from './localMediaSource.js'

const base = {
  name: 'sample.mov',
  size: 1024,
  lastModified: 123456,
  headBytes: new Uint8Array([1, 2, 3]),
  tailBytes: new Uint8Array([4, 5, 6]),
}

test('fingerprintParts returns a stable media id', async () => {
  const first = await fingerprintParts(base)
  const second = await fingerprintParts(base)

  assert.equal(first, second)
  assert.match(first, /^[a-f0-9]{64}$/)
})

test('fingerprintParts changes when sampled content changes', async () => {
  const first = await fingerprintParts(base)
  const second = await fingerprintParts({
    ...base,
    tailBytes: new Uint8Array([4, 5, 7]),
  })

  assert.notEqual(first, second)
})

test('video picker does not filter files before adapter validation', () => {
  assert.deepEqual(VIDEO_PICKER_OPTIONS, { multiple: false })
})
