import assert from 'node:assert/strict'
import test from 'node:test'
import {
  createVideoEntry,
  getProgressPercent,
  getResumeTime,
  normalizeProgress,
  sortVideoEntries,
  titleFromFileName,
} from './videoLibraryModel.js'

test('normalizeProgress clamps invalid and out-of-range values', () => {
  assert.equal(normalizeProgress(Number.NaN, 100), 0)
  assert.equal(normalizeProgress(-4, 100), 0)
  assert.equal(normalizeProgress(140, 100), 100)
  assert.equal(normalizeProgress(42.5, 100), 42.5)
})

test('getResumeTime keeps active progress and restarts completed videos', () => {
  assert.equal(getResumeTime({ currentTime: 42, duration: 100 }), 42)
  assert.equal(
    getResumeTime({ currentTime: 100, duration: 100, completed: true }),
    0,
  )
  assert.equal(getProgressPercent({ currentTime: 100, duration: 100 }), 100)
})

test('createVideoEntry preserves progress when the same video is imported', () => {
  const existingEntry = {
    id: 'same-id',
    currentTime: 35,
    completed: false,
    createdAt: 10,
    thumbnailDataUrl: 'old-thumbnail',
  }
  const videoHandle = { name: 'video-handle' }
  const subtitleHandle = { name: 'subtitle-handle' }

  const entry = createVideoEntry({
    existingEntry,
    asset: {
      id: 'same-id',
      name: 'Lesson.mov',
      size: 500,
      lastModified: 20,
      mimeType: 'video/quicktime',
      duration: 120,
      source: { handle: videoHandle },
    },
    subtitleFileName: 'Lesson.srt',
    subtitleHandle,
    now: 30,
  })

  assert.equal(entry.id, 'same-id')
  assert.equal(entry.title, 'Lesson')
  assert.equal(entry.currentTime, 35)
  assert.equal(entry.createdAt, 10)
  assert.equal(entry.lastPlayedAt, 30)
  assert.equal(entry.thumbnailDataUrl, 'old-thumbnail')
  assert.equal(entry.videoHandle, videoHandle)
  assert.equal(entry.subtitleHandle, subtitleHandle)
})

test('sortVideoEntries puts the most recently played entry first', () => {
  const entries = [
    { id: 'old', lastPlayedAt: 10, createdAt: 10 },
    { id: 'new', lastPlayedAt: 30, createdAt: 5 },
    { id: 'middle', lastPlayedAt: 20, createdAt: 20 },
  ]

  assert.deepEqual(
    sortVideoEntries(entries).map((entry) => entry.id),
    ['new', 'middle', 'old'],
  )
})

test('titleFromFileName removes any supported media extension', () => {
  assert.equal(titleFromFileName('Lesson.mov'), 'Lesson')
  assert.equal(titleFromFileName('Episode.01.mp4'), 'Episode.01')
})
