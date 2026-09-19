import assert from 'node:assert/strict'
import test from 'node:test'
import {
  analyzeSentence,
  favoriteSentence,
  getSentenceFavoriteStatus,
  translateSentenceStream,
} from '../../../api/word.js'

function streamResponse(chunks, init = {}) {
  const encoder = new TextEncoder()
  return new Response(
    new ReadableStream({
      start(controller) {
        chunks.forEach((chunk) => controller.enqueue(encoder.encode(chunk)))
        controller.close()
      },
    }),
    {
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' },
      ...init,
    },
  )
}

async function withMockFetch(mock, run) {
  const original = globalThis.fetch
  globalThis.fetch = mock
  try {
    return await run()
  } finally {
    globalThis.fetch = original
  }
}

test('analyzeSentence handles heartbeats, fragmented chunks, and CRLF', async () => {
  const analysis = {
    sentence: 'We test this.',
    structure: '主句',
    chunks: [
      {
        text: 'We test this.',
        type: 'main',
        role: '主句',
        translation: '我们测试这个。',
      },
    ],
  }

  await withMockFetch(
    async () =>
      streamResponse([
        ': keep-alive\r\n\r\nevent: res',
        `ult\r\ndata: ${JSON.stringify(analysis)}\r\n\r\n`,
        'data: [DONE]\r\n\r\n',
      ]),
    async () => {
      assert.deepEqual(await analyzeSentence('We test this.'), analysis)
    },
  )
})

test('analyzeSentence surfaces SSE errors and missing results', async () => {
  await withMockFetch(
    async () =>
      streamResponse([
        'event: error\n',
        'data: {"msg":"analysis unavailable"}\n\n',
        'data: [DONE]\n\n',
      ]),
    async () => {
      await assert.rejects(
        analyzeSentence('We test this.'),
        /analysis unavailable/,
      )
    },
  )

  await withMockFetch(
    async () => streamResponse(['data: [DONE]\n\n']),
    async () => {
      await assert.rejects(
        analyzeSentence('We test this.'),
        /分析未返回结果/,
      )
    },
  )
})

test('analyzeSentence forwards AbortSignal to fetch', async () => {
  const controller = new AbortController()

  await withMockFetch(
    async (_url, options) =>
      new Promise((_resolve, reject) => {
        options.signal.addEventListener('abort', () => {
          reject(new DOMException('Aborted', 'AbortError'))
        })
      }),
    async () => {
      const request = analyzeSentence('We test this.', {
        signal: controller.signal,
      })
      controller.abort()
      await assert.rejects(request, { name: 'AbortError' })
    },
  )
})

test('getSentenceFavoriteStatus encodes sentence and parses status', async () => {
  const controller = new AbortController()
  await withMockFetch(
    async (url, options) => {
      assert.equal(
        url,
        '/api/v1/sentences/favorites/status?sentence=We+test+this.',
      )
      assert.equal(options.signal, controller.signal)
      return Response.json({
        code: 0,
        data: { favorited: true, id: 7 },
      })
    },
    async () => {
      assert.deepEqual(
        await getSentenceFavoriteStatus('We test this.', {
          signal: controller.signal,
        }),
        { favorited: true, id: 7 },
      )
    },
  )
})

test('favoriteSentence posts English and Chinese text', async () => {
  const payload = {
    sentence: 'We test this.',
    translation: '我们测试这个。',
  }
  await withMockFetch(
    async (url, options) => {
      assert.equal(url, '/api/v1/sentences/favorites')
      assert.equal(options.method, 'POST')
      assert.deepEqual(JSON.parse(options.body), payload)
      return Response.json({
        code: 0,
        data: { id: 7, ...payload, createdAt: '2026-08-22T12:00:00Z' },
      })
    },
    async () => {
      const result = await favoriteSentence(payload)
      assert.equal(result.id, 7)
      assert.equal(result.translation, payload.translation)
    },
  )
})

test('sentence favorite APIs surface server errors', async () => {
  await withMockFetch(
    async () =>
      Response.json(
        { code: 503, msg: 'database not configured' },
        { status: 503 },
      ),
    async () => {
      await assert.rejects(
        getSentenceFavoriteStatus('We test this.'),
        /database not configured/,
      )
      await assert.rejects(
        favoriteSentence({
          sentence: 'We test this.',
          translation: '我们测试这个。',
        }),
        /database not configured/,
      )
    },
  )
})

test('translateSentenceStream emits ordered deltas and one completion', async () => {
  await withMockFetch(
    async () =>
      streamResponse([
        'data: {"delta":"我们"}\n\n',
        'data: {"delta":"测试"}\n\n',
        'data: [DONE]\n\n',
      ]),
    async () => {
      const deltas = []
      let doneCount = 0
      await new Promise((resolve, reject) => {
        translateSentenceStream('We test.', {
          onDelta: (delta) => deltas.push(delta),
          onDone: () => {
            doneCount += 1
            resolve()
          },
          onError: reject,
        })
      })

      assert.deepEqual(deltas, ['我们', '测试'])
      assert.equal(doneCount, 1)
    },
  )
})
