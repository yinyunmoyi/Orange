import assert from 'node:assert/strict'
import test from 'node:test'
import { createPlaybackSource } from './mediaPlayback.js'

test('createPlaybackSource preserves pipeline inputs', () => {
  const file = { name: 'episode.mkv' }
  assert.deepEqual(
    createPlaybackSource({
      file,
      url: 'blob:episode',
      fingerprint: 'abcd1234abcd1234',
      duration: 120,
    }),
    {
      file,
      url: 'blob:episode',
      fingerprint: 'abcd1234abcd1234',
      duration: 120,
    },
  )
})

test('createPlaybackSource requires a playback URL', () => {
  assert.throws(() => createPlaybackSource(), /缺少视频播放地址/)
})

test('createPlaybackSource omits invalid durations', () => {
  assert.deepEqual(createPlaybackSource({ url: 'blob:episode', duration: 0 }), {
    file: null,
    url: 'blob:episode',
    fingerprint: '',
  })
})
