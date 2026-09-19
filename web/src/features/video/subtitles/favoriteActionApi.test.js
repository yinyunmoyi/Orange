import assert from 'node:assert/strict'
import test from 'node:test'
import { recordFavoriteAction } from '../../../api/word.js'

test('recordFavoriteAction sends the stable event contract', async (t) => {
  const originalFetch = globalThis.fetch
  t.after(() => {
    globalThis.fetch = originalFetch
  })
  globalThis.fetch = async (url, init) => {
    assert.equal(url, '/api/v1/favorites/word/42/actions')
    assert.equal(init.method, 'POST')
    assert.deepEqual(JSON.parse(init.body), {
      eventId: 'event-12345678',
      action: 'lookup_opened',
      source: 'web_video',
      metadata: {},
    })
    return {
      ok: true,
      status: 200,
      json: async () => ({ code: 0, data: { itemType: 'word', itemId: 42, level: 3 } }),
    }
  }

  const result = await recordFavoriteAction({
    itemType: 'word',
    itemId: 42,
    eventId: 'event-12345678',
    action: 'lookup_opened',
    source: 'web_video',
  })
  assert.equal(result.level, 3)
})

test('recordFavoriteAction surfaces server errors', async (t) => {
  const originalFetch = globalThis.fetch
  t.after(() => {
    globalThis.fetch = originalFetch
  })
  globalThis.fetch = async () => ({
    ok: false,
    status: 409,
    json: async () => ({ code: 409, msg: 'event conflict' }),
  })

  await assert.rejects(
    recordFavoriteAction({
      itemType: 'word',
      itemId: 42,
      eventId: 'event-12345678',
      action: 'lookup_opened',
      source: 'web_video',
    }),
    /event conflict/,
  )
})
