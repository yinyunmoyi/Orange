import assert from 'node:assert/strict'
import test from 'node:test'
import { runPlaybackPipeline } from './playbackPipeline.js'

test('aborted Strict Mode attachment does not start a media probe', async (t) => {
  const originalWindow = globalThis.window
  const originalFetch = globalThis.fetch
  t.after(() => {
    globalThis.window = originalWindow
    globalThis.fetch = originalFetch
  })
  let listenerCount = 0
  globalThis.window = {
    localStorage: {
      getItem: () => null,
      setItem: () => {},
    },
    setTimeout,
    clearTimeout,
  }
  globalThis.fetch = async () => ({ ok: true })

  const video = {
    src: '',
    currentSrc: '',
    paused: true,
    ended: false,
    muted: false,
    volume: 1,
    readyState: 0,
    networkState: 0,
    currentTime: 0,
    duration: Number.NaN,
    error: null,
    buffered: { length: 0 },
    load() {},
    pause() {},
    removeAttribute() {},
    addEventListener() {
      listenerCount += 1
    },
    removeEventListener() {},
  }
  const controller = new AbortController()
  const result = runPlaybackPipeline(
    video,
    { url: 'blob:test', fingerprint: 'strict-mode-test' },
    { signal: controller.signal },
  )
  controller.abort()

  await assert.rejects(result, { name: 'AbortError' })
  assert.equal(listenerCount, 0)
})
