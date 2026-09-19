import assert from 'node:assert/strict'
import test from 'node:test'
import { resetFavoriteLearning } from '../../../api/word.js'

async function withMockFetch(mock, run) {
  const original = globalThis.fetch
  globalThis.fetch = mock
  try {
    return await run()
  } finally {
    globalThis.fetch = original
  }
}

test('resetFavoriteLearning sends the word reset request', async () => {
  let request
  await withMockFetch(
    async (url, options) => {
      request = { url, options }
      return Response.json({ code: 0 })
    },
    async () => resetFavoriteLearning('word', 42),
  )

  assert.equal(
    request.url,
    '/api/v1/favorites/word/42/learning/reset',
  )
  assert.equal(request.options.method, 'POST')
  assert.equal(request.options.headers['Content-Type'], 'application/json')
  assert.equal(request.options.body, '{}')
})

test('resetFavoriteLearning supports phrase favorites and AbortSignal', async () => {
  const controller = new AbortController()
  let request
  await withMockFetch(
    async (url, options) => {
      request = { url, options }
      return Response.json({ code: 0 })
    },
    async () =>
      resetFavoriteLearning('phrase', 17, { signal: controller.signal }),
  )

  assert.equal(
    request.url,
    '/api/v1/favorites/phrase/17/learning/reset',
  )
  assert.equal(request.options.signal, controller.signal)
})

test('resetFavoriteLearning surfaces backend and HTTP errors', async () => {
  await withMockFetch(
    async () =>
      Response.json(
        { code: 404, msg: 'favorite item not found' },
        { status: 404 },
      ),
    async () => {
      await assert.rejects(
        resetFavoriteLearning('word', 42),
        /favorite item not found/,
      )
    },
  )

  await withMockFetch(
    async () => Response.json({ code: 500 }, { status: 500 }),
    async () => {
      await assert.rejects(
        resetFavoriteLearning('phrase', 17),
        /重置学习进度失败（500）/,
      )
    },
  )
})

test('resetFavoriteLearning rejects invalid parameters before fetch', async () => {
  let calls = 0
  await withMockFetch(
    async () => {
      calls += 1
      return Response.json({ code: 0 })
    },
    async () => {
      for (const [itemType, itemId] of [
        ['other', 1],
        ['word', 0],
        ['phrase', -1],
        ['word', 1.5],
        ['word', '1'],
      ]) {
        await assert.rejects(
          resetFavoriteLearning(itemType, itemId),
          /无效的学习进度重置参数/,
        )
      }
    },
  )

  assert.equal(calls, 0)
})
