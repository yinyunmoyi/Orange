import assert from 'node:assert/strict'
import test from 'node:test'
import {
  selectAudioRecorderMimeType,
  selectRecorderMimeType,
  validateRecordingRange,
} from './recordContextClip.js'

test('selectRecorderMimeType prefers VP8 and falls back to WebM', () => {
  const supported = new Set(['video/webm;codecs=vp9,opus', 'video/webm'])
  const Recorder = {
    isTypeSupported: (mime) => supported.has(mime),
  }

  assert.equal(
    selectRecorderMimeType(Recorder),
    'video/webm;codecs=vp9,opus',
  )
  assert.equal(selectRecorderMimeType({}), 'video/webm')
  assert.equal(selectRecorderMimeType(null), '')
})

test('selectAudioRecorderMimeType prefers Opus WebM', () => {
  const supported = new Set(['audio/webm;codecs=opus', 'audio/webm'])
  const Recorder = {
    isTypeSupported: (mime) => supported.has(mime),
  }

  assert.equal(
    selectAudioRecorderMimeType(Recorder),
    'audio/webm;codecs=opus',
  )
  assert.equal(selectAudioRecorderMimeType({}), 'audio/webm')
  assert.equal(selectAudioRecorderMimeType(null), '')
})

test('validateRecordingRange accepts clips up to sixty seconds', () => {
  assert.equal(validateRecordingRange(10, 19), 9)
  assert.equal(validateRecordingRange(0, 60), 60)
})

test('validateRecordingRange rejects invalid and overlong clips', () => {
  for (const [start, end] of [
    [-1, 2],
    [2, 2],
    [3, 2],
    [0, 60.01],
  ]) {
    assert.throws(() => validateRecordingRange(start, end), /时长无效/)
  }
})
