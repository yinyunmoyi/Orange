import assert from 'node:assert/strict'
import test from 'node:test'
import {
  registerMediaFormat,
  resolveMediaFormat,
  supportedMediaDescription,
} from './mediaFormats.js'

test('built-in media adapters accept MOV, MP4, and MKV files', () => {
  assert.equal(
    resolveMediaFormat({ name: 'lesson.mov', type: '' }).id,
    'quicktime',
  )
  const mp4 = resolveMediaFormat({
    name: 'lesson.MP4',
    type: 'video/mp4',
  })
  assert.equal(mp4.id, 'mp4')
  assert.equal(mp4.metadataAdapter, 'native')
  const mkv = resolveMediaFormat({
    name: 'lesson.MKV',
    type: '',
  })
  assert.equal(mkv.id, 'matroska')
  assert.equal(mkv.metadataAdapter, 'mediabunny')
  assert.equal(supportedMediaDescription(), 'MOV / MP4 / M4V / MKV')
})

test('media format registry accepts another adapter without loader changes', () => {
  registerMediaFormat({
    id: 'test-webm',
    description: 'WebM test video',
    extensions: ['testwebm'],
    mimeTypes: ['video/x-test-webm'],
  })

  assert.equal(
    resolveMediaFormat({
      name: 'lesson.testwebm',
      type: 'video/x-test-webm',
    }).id,
    'test-webm',
  )
})

test('media format registry rejects unknown files', () => {
  assert.throws(
    () => resolveMediaFormat({ name: 'lesson.avi', type: 'video/avi' }),
    /不支持 .avi 视频/,
  )
})
