import {
  normalizeProgress,
  sortVideoEntries,
} from './videoLibraryModel.js'

const DATABASE_NAME = 'orange-video-library'
const DATABASE_VERSION = 1
const VIDEO_STORE = 'videos'
const LAST_PLAYED_INDEX = 'lastPlayedAt'

let databasePromise

export async function listVideoEntries() {
  const database = await openDatabase()
  try {
    const transaction = database.transaction(VIDEO_STORE, 'readonly')
    const entries = await requestResult(transaction.objectStore(VIDEO_STORE).getAll())
    return sortVideoEntries(entries)
  } catch (error) {
    throw repositoryError('读取播放列表失败', error)
  }
}

export async function getVideoEntry(id) {
  const database = await openDatabase()
  try {
    const transaction = database.transaction(VIDEO_STORE, 'readonly')
    return await requestResult(transaction.objectStore(VIDEO_STORE).get(id))
  } catch (error) {
    throw repositoryError('读取视频记录失败', error)
  }
}

export async function upsertVideoEntry(entry) {
  const database = await openDatabase()
  try {
    const transaction = database.transaction(VIDEO_STORE, 'readwrite')
    transaction.objectStore(VIDEO_STORE).put(entry)
    await transactionComplete(transaction)
    return entry
  } catch (error) {
    throw repositoryError('保存视频记录失败', error)
  }
}

export async function updateVideoProgress(
  id,
  { currentTime, duration, completed, updatedAt = Date.now() },
) {
  return updateEntry(id, '保存播放进度失败', (entry) => {
    const nextDuration =
      Number.isFinite(Number(duration)) && Number(duration) > 0
        ? Number(duration)
        : entry.duration
    return {
      ...entry,
      duration: nextDuration,
      currentTime: normalizeProgress(currentTime, nextDuration),
      completed: Boolean(completed),
      lastPlayedAt: updatedAt,
      updatedAt,
    }
  })
}

export async function touchVideoEntry(id, timestamp = Date.now()) {
  return updateEntry(id, '更新最近播放时间失败', (entry) => ({
    ...entry,
    lastPlayedAt: timestamp,
    updatedAt: timestamp,
  }))
}

export async function replaceChangedVideoEntry(previousId, nextEntry) {
  const database = await openDatabase()
  try {
    const transaction = database.transaction(VIDEO_STORE, 'readwrite')
    const store = transaction.objectStore(VIDEO_STORE)
    if (previousId !== nextEntry.id) store.delete(previousId)
    store.put(nextEntry)
    await transactionComplete(transaction)
    return nextEntry
  } catch (error) {
    throw repositoryError('更新已变化的视频文件失败', error)
  }
}

async function updateEntry(id, message, transform) {
  const database = await openDatabase()
  try {
    return await new Promise((resolve, reject) => {
      const transaction = database.transaction(VIDEO_STORE, 'readwrite')
      const store = transaction.objectStore(VIDEO_STORE)
      const request = store.get(id)
      let nextEntry
      let settled = false

      const fail = (error) => {
        if (settled) return
        settled = true
        reject(error)
      }

      request.onsuccess = () => {
        if (!request.result) {
          fail(new Error('视频记录不存在'))
          transaction.abort()
          return
        }
        nextEntry = transform(request.result)
        store.put(nextEntry)
      }
      request.onerror = () => fail(request.error)
      transaction.oncomplete = () => {
        if (settled) return
        settled = true
        resolve(nextEntry)
      }
      transaction.onerror = () => fail(transaction.error)
      transaction.onabort = () =>
        fail(transaction.error || new Error('本地视频数据库事务已取消'))
    })
  } catch (error) {
    throw repositoryError(message, error)
  }
}

function openDatabase() {
  if (databasePromise) return databasePromise
  if (typeof indexedDB === 'undefined') {
    return Promise.reject(new Error('当前浏览器不支持本地视频播放列表'))
  }

  databasePromise = new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION)

    request.onupgradeneeded = () => {
      const database = request.result
      const store = database.objectStoreNames.contains(VIDEO_STORE)
        ? request.transaction.objectStore(VIDEO_STORE)
        : database.createObjectStore(VIDEO_STORE, { keyPath: 'id' })
      if (!store.indexNames.contains(LAST_PLAYED_INDEX)) {
        store.createIndex(LAST_PLAYED_INDEX, LAST_PLAYED_INDEX)
      }
    }
    request.onsuccess = () => {
      request.result.onversionchange = () => request.result.close()
      resolve(request.result)
    }
    request.onerror = () => reject(request.error)
    request.onblocked = () => {
      reject(new Error('本地视频数据库正在被其他页面占用'))
    }
  }).catch((error) => {
    databasePromise = undefined
    throw repositoryError('打开本地视频数据库失败', error)
  })

  return databasePromise
}

function requestResult(request) {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function transactionComplete(transaction) {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onerror = () => reject(transaction.error)
    transaction.onabort = () => reject(transaction.error)
  })
}

function repositoryError(message, error) {
  if (error?.message?.startsWith(message)) return error
  return new Error(`${message}${error?.message ? `：${error.message}` : ''}`)
}
