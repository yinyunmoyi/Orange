import assert from 'node:assert/strict'
import test from 'node:test'
import { getNote, saveNote } from '../../../api/word.js'

async function withMockFetch(mock, run) {
  const original = globalThis.fetch
  globalThis.fetch = mock
  try {
    return await run()
  } finally {
    globalThis.fetch = original
  }
}

test('getNote loads an encoded word note and forwards AbortSignal', async () => {
  const controller = new AbortController()
  let request
  const result = await withMockFetch(
    async (url, options) => {
      request = { url, options }
      return Response.json({
        code: 0,
        data: {
          itemType: 'word',
          text: 'can-do',
          note: 'word note',
          updatedAt: '2026-08-16T10:00:00Z',
        },
      })
    },
    async () => getNote('word', 'Can-Do', { signal: controller.signal }),
  )

  assert.equal(
    request.url,
    '/api/v1/notes?itemType=word&text=Can-Do',
  )
  assert.equal(request.options.signal, controller.signal)
  assert.deepEqual(result, {
    itemType: 'word',
    text: 'can-do',
    note: 'word note',
    updatedAt: '2026-08-16T10:00:00Z',
  })
})

test('getNote encodes phrase text and fills missing optional fields', async () => {
  let requestUrl
  const result = await withMockFetch(
    async (url) => {
      requestUrl = url
      return Response.json({ code: 0 })
    },
    async () => getNote('phrase', 'look up & learn'),
  )

  assert.equal(
    requestUrl,
    '/api/v1/notes?itemType=phrase&text=look+up+%26+learn',
  )
  assert.deepEqual(result, {
    itemType: 'phrase',
    text: 'look up & learn',
    note: '',
    updatedAt: '',
  })
})

test('saveNote sends a PUT JSON request and forwards AbortSignal', async () => {
  const controller = new AbortController()
  let request
  const result = await withMockFetch(
    async (url, options) => {
      request = { url, options }
      return Response.json({
        code: 0,
        data: { itemType: 'phrase', text: 'look up', note: 'saved' },
      })
    },
    async () =>
      saveNote('phrase', 'Look Up', 'saved', {
        signal: controller.signal,
      }),
  )

  assert.equal(request.url, '/api/v1/notes')
  assert.equal(request.options.method, 'PUT')
  assert.equal(request.options.headers['Content-Type'], 'application/json')
  assert.equal(request.options.body, JSON.stringify({
    itemType: 'phrase',
    text: 'Look Up',
    note: 'saved',
  }))
  assert.equal(request.options.signal, controller.signal)
  assert.equal(result.note, 'saved')
})

test('note APIs surface backend and HTTP errors', async () => {
  await withMockFetch(
    async () =>
      Response.json({ code: 400, msg: 'note is too long' }, { status: 400 }),
    async () => {
      await assert.rejects(
        saveNote('word', 'hello', 'x'),
        /note is too long/,
      )
    },
  )

  await withMockFetch(
    async () => Response.json({ code: 500 }, { status: 500 }),
    async () => {
      await assert.rejects(
        getNote('phrase', 'look up'),
        /备注加载失败（500）/,
      )
    },
  )
})

test('note APIs reject invalid parameters before fetch', async () => {
  let calls = 0
  await withMockFetch(
    async () => {
      calls += 1
      return Response.json({ code: 0 })
    },
    async () => {
      await assert.rejects(getNote('other', 'hello'), /无效的备注目标/)
      await assert.rejects(getNote('word', '  '), /无效的备注目标/)
      await assert.rejects(
        saveNote('phrase', 'look up', null),
        /无效的备注内容/,
      )
    },
  )

  assert.equal(calls, 0)
})
