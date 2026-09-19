// 单词查询接口封装 —— 调用后端 aaa_word（经 Vite proxy 转发到 :8888）
import { fetchWithDiagnostics } from '../diagnostics/runtimeDiagnostics.js'

const API_BASE = '/api/v1/word'
const PHRASE_API_BASE = '/api/v1/phrase'
const CONTEXT_API_BASE = '/api/v1/contexts'
const FAVORITES_API_BASE = '/api/v1/favorites'
const NOTES_API_BASE = '/api/v1/notes'
const SENTENCE_FAVORITES_API_BASE = '/api/v1/sentences/favorites'
const fetch = (input, init) => fetchWithDiagnostics('word-api', input, init)

function requestFailure(response) {
  if (response.status === 502 || response.status === 503 || response.status === 504) {
    return '后端服务不可用，请确认 aaa_word 已启动'
  }
  return `请求失败（${response.status}）`
}

export function createActionEventId() {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID()
  }
  return `evt-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}

// 查询单词的英文信息（音标、发音、英英释义）
export async function lookupWord(word) {
  const res = await fetch(`${API_BASE}/lookup`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ word }),
  })

  if (!res.ok) throw new Error(requestFailure(res))
  const json = await res.json()

  if (json.code !== 0) {
    throw new Error(json.msg || requestFailure(res))
  }

  return json.data
}

// 内部通用：POST 请求并解析统一响应结构
async function postWord(path, word) {
  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ word }),
  })

  if (!res.ok) throw new Error(requestFailure(res))
  const json = await res.json()

  if (json.code !== 0) {
    throw new Error(json.msg || requestFailure(res))
  }

  return json.data
}

// 查询单词的汉语释义
export function getMeaning(word) {
  return postWord('/meaning', word)
}

// 查询单词或短语在当前字幕语境中的含义
export async function explainSelection({
  word,
  context,
  wordStart,
  wordEnd,
}) {
  const res = await fetch(`${API_BASE}/explain`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ word, context, wordStart, wordEnd }),
  })
  if (!res.ok) throw new Error(requestFailure(res))
  const json = await res.json()
  if (json.code !== 0) {
    throw new Error(json.msg || requestFailure(res))
  }
  return json.data
}

// 查询短语的整体汉语释义
export async function lookupPhrase(phrase) {
  const res = await fetch(`${API_BASE}/phrase`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ phrase }),
  })
  if (!res.ok) throw new Error(requestFailure(res))
  const json = await res.json()
  if (json.code !== 0) {
    throw new Error(json.msg || requestFailure(res))
  }
  return json.data
}

// 查询单词的所有语境
export function getContexts(word) {
  return postWord('/contexts', word)
}

// 收藏单词：把当前页面已加载的所有数据一并送到后端
export async function favoriteWord(payload) {
  const res = await fetch(`${API_BASE}/favorite`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `收藏失败（${res.status}）`)
  }
  return json.data || {}
}

// 查询该词的收藏状态
export async function getFavoriteStatus(word) {
  const res = await fetch(`${API_BASE}/favorite/status?word=${encodeURIComponent(word)}`)
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    return { favorited: false }
  }
  return json.data || { favorited: false }
}

// 查询短语收藏状态
export async function getPhraseFavoriteStatus(phrase) {
  const res = await fetch(
    `${PHRASE_API_BASE}/favorite/status?phrase=${encodeURIComponent(phrase)}`,
  )
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `请求失败（${res.status}）`)
  }
  return json.data || { favorited: false }
}

// 收藏短语；短语释义必须已加载
export async function favoritePhrase(payload) {
  const res = await fetch(`${PHRASE_API_BASE}/favorite`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `收藏失败（${res.status}）`)
  }
  return json.data || {}
}

function validateNoteTarget(itemType, text) {
  if (
    !['word', 'phrase'].includes(itemType) ||
    typeof text !== 'string' ||
    !text.trim()
  ) {
    throw new Error('无效的备注目标')
  }
}

function noteData(data, itemType, text) {
  return {
    itemType: data?.itemType || itemType,
    text: data?.text || text,
    note: typeof data?.note === 'string' ? data.note : '',
    updatedAt: data?.updatedAt || '',
  }
}

export async function getNote(itemType, text, { signal } = {}) {
  validateNoteTarget(itemType, text)
  const query = new URLSearchParams({ itemType, text })
  const res = await fetch(`${NOTES_API_BASE}?${query}`, { signal })
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `备注加载失败（${res.status}）`)
  }
  return noteData(json.data, itemType, text)
}

export async function saveNote(itemType, text, note, { signal } = {}) {
  validateNoteTarget(itemType, text)
  if (typeof note !== 'string') {
    throw new Error('无效的备注内容')
  }

  const res = await fetch(NOTES_API_BASE, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ itemType, text, note }),
    signal,
  })
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `备注保存失败（${res.status}）`)
  }
  return noteData(json.data, itemType, text)
}

export async function resetFavoriteLearning(
  itemType,
  itemId,
  { signal } = {},
) {
  if (
    !['word', 'phrase'].includes(itemType) ||
    !Number.isInteger(itemId) ||
    itemId <= 0
  ) {
    throw new Error('无效的学习进度重置参数')
  }

  const res = await fetch(
    `${FAVORITES_API_BASE}/${encodeURIComponent(itemType)}/${encodeURIComponent(itemId)}/learning/reset`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
      signal,
    },
  )
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `重置学习进度失败（${res.status}）`)
  }
}

export async function recordFavoriteAction({
  itemType,
  itemId,
  eventId,
  action,
  source,
  metadata = {},
}) {
  if (
    !['word', 'phrase'].includes(itemType) ||
    !Number.isInteger(itemId) ||
    itemId <= 0 ||
    typeof eventId !== 'string' ||
    !eventId
  ) {
    throw new Error('无效的动作事件参数')
  }
  const res = await fetch(
    `${FAVORITES_API_BASE}/${encodeURIComponent(itemType)}/${itemId}/actions`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ eventId, action, source, metadata }),
    },
  )
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `动作记录失败（${res.status}）`)
  }
  return json.data || {}
}

export async function createContextTask(payload, { signal } = {}) {
  const res = await fetch(`${CONTEXT_API_BASE}/tasks`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  })
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `语境保存失败（${res.status}）`)
  }
  return json.data
}

export async function uploadContextVideo(
  contextId,
  blob,
  audioBlob,
  metadata,
  { signal } = {},
) {
  const form = new FormData()
  form.append('file', blob, 'context.webm')
  form.append('audioFile', audioBlob, 'context-audio.webm')
  form.append('sourceFingerprint', metadata.sourceFingerprint)
  form.append('clipStartMs', String(metadata.clipStartMs))
  form.append('clipEndMs', String(metadata.clipEndMs))
  form.append('durationMs', String(metadata.durationMs))
  form.append('subtitles', JSON.stringify(metadata.subtitles))

  const res = await fetch(`${CONTEXT_API_BASE}/${contextId}/videos`, {
    method: 'POST',
    body: form,
    signal,
  })
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `视频语境上传失败（${res.status}）`)
  }
  return json.data
}

// 列出已收藏的单词，query 为空则返回全部；非空按 word 模糊包含匹配
export async function listFavorites(query = '') {
  const url = query
    ? `${API_BASE}/favorites?q=${encodeURIComponent(query)}`
    : `${API_BASE}/favorites`
  const res = await fetch(url)
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `请求失败（${res.status}）`)
  }
  return json.data || { items: [], total: 0 }
}

// 列出所有相似单词组（不支持过滤）
export async function listSimilarGroups() {
  const res = await fetch(`${API_BASE}/groups`)
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `请求失败（${res.status}）`)
  }
  return json.data || { items: [], total: 0 }
}

// 查看单个相似单词组的成员详情；组不存在返回 null
export async function getSimilarGroup(id) {
  const res = await fetch(`${API_BASE}/groups/${encodeURIComponent(id)}`)
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `请求失败（${res.status}）`)
  }
  return json.data || null
}

// 长难句分析：SSE result 帧返回 structure + chunks（翻译走独立 SSE 接口）
export async function analyzeSentence(sentence, { signal } = {}) {
  const res = await fetch(`${API_BASE}/sentence/analyze`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify({ sentence }),
    signal,
  })
  if (!res.ok || !res.body) {
    throw new Error(requestFailure(res))
  }

  let result = null
  await consumeSSE(res, ({ event, data }) => {
    if (event === 'error') {
      throw new Error(parseSSEError(data, '分析失败'))
    }
    if (event === 'result') {
      const parsed = JSON.parse(data)
      const value = parsed?.data || parsed
      if (!value || !Array.isArray(value.chunks)) {
        throw new Error('分析结果格式错误')
      }
      result = value
      return false
    }
    return data === '[DONE]'
  })

  if (!result) throw new Error('分析未返回结果')
  return result
}

export async function getSentenceFavoriteStatus(sentence, { signal } = {}) {
  const query = new URLSearchParams({ sentence })
  const res = await fetch(
    `${SENTENCE_FAVORITES_API_BASE}/status?${query}`,
    { signal },
  )
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `收藏状态加载失败（${res.status}）`)
  }
  return json.data || { favorited: false }
}

export async function favoriteSentence(payload, { signal } = {}) {
  const res = await fetch(SENTENCE_FAVORITES_API_BASE, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  })
  const json = await res.json()
  if (!res.ok || json.code !== 0) {
    throw new Error(json.msg || `收藏失败（${res.status}）`)
  }
  return json.data || {}
}

// 长难句翻译：SSE 流式接口。返回一个 { cancel } 句柄，供组件卸载时中止。
// 参数 handlers: { onDelta(text), onDone(), onError(msg) }
export function translateSentenceStream(sentence, { onDelta, onDone, onError } = {}) {
  const controller = new AbortController()
  let finished = false

  const finish = (fn, arg) => {
    if (finished) return
    finished = true
    if (fn) fn(arg)
  }

  ;(async () => {
    try {
      const res = await fetch(`${API_BASE}/sentence/translate`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'text/event-stream',
        },
        body: JSON.stringify({ sentence }),
        signal: controller.signal,
      })
      if (!res.ok || !res.body) {
        finish(onError, requestFailure(res))
        return
      }

      await consumeSSE(res, ({ event, data }) => {
        if (event === 'error') {
          finish(onError, parseSSEError(data, '翻译失败'))
          return true
        }
        if (data === '[DONE]') {
          finish(onDone)
          return true
        }
        try {
          const parsed = JSON.parse(data)
          if (typeof parsed.delta === 'string' && parsed.delta && onDelta) {
            onDelta(parsed.delta)
          }
        } catch {
          // Ignore malformed non-error frames and wait for the next SSE event.
        }
        return false
      })
      finish(onDone)
    } catch (err) {
      if (err.name === 'AbortError') return
      finish(onError, err.message || '网络错误')
    }
  })()

  return {
    cancel() {
      finished = true
      controller.abort()
    },
  }
}

async function consumeSSE(response, onFrame) {
  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  let stopped = false

  const processFrame = (rawFrame) => {
    let event = 'message'
    const dataLines = []
    for (const line of rawFrame.split('\n')) {
      if (!line || line.startsWith(':')) continue
      if (line.startsWith('event:')) {
        event = line.slice(6).trim()
      } else if (line.startsWith('data:')) {
        dataLines.push(line.slice(5).trimStart())
      }
    }
    if (dataLines.length === 0) return false
    return onFrame({ event, data: dataLines.join('\n') }) === true
  }

  try {
    while (!stopped) {
      const { value, done } = await reader.read()
      buffer += decoder.decode(value || new Uint8Array(), { stream: !done })
      buffer = buffer.replace(/\r\n/g, '\n')

      let separator
      while (!stopped && (separator = buffer.indexOf('\n\n')) >= 0) {
        const frame = buffer.slice(0, separator)
        buffer = buffer.slice(separator + 2)
        stopped = processFrame(frame)
      }
      if (done) break
    }
    if (!stopped && buffer.trim()) {
      processFrame(buffer)
    }
  } finally {
    if (stopped) {
      await reader.cancel().catch(() => {})
    }
    reader.releaseLock()
  }
}

function parseSSEError(data, fallback) {
  try {
    return JSON.parse(data)?.msg || fallback
  } catch {
    return fallback
  }
}
