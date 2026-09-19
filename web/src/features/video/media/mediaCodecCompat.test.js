import assert from 'node:assert/strict'
import test from 'node:test'
import {
  createCompatibleAudioDecoderConfig,
  registerCompatibleAudioCodec,
  resolveCompatibleAudioCodec,
} from './mediaCodecCompat.js'

test('resolveCompatibleAudioCodec maps Matroska DTS identifiers', () => {
  assert.equal(resolveCompatibleAudioCodec(null, 'A_DTS'), 'dts')
  assert.equal(resolveCompatibleAudioCodec(null, 'A_DTS/LOSSLESS'), 'dts')
})

test('resolveCompatibleAudioCodec preserves known codecs and rejects unknown ids', () => {
  assert.equal(resolveCompatibleAudioCodec('aac', 'A_DTS'), 'aac')
  assert.equal(resolveCompatibleAudioCodec(null, 'A_TRUEHD'), null)
})

test('createCompatibleAudioDecoderConfig preserves Matroska audio metadata', () => {
  assert.deepEqual(
    createCompatibleAudioDecoderConfig(null, 'A_DTS/LOSSLESS', {
      numberOfChannels: 6,
      sampleRate: 48_000,
    }),
    {
      codec: 'dts',
      numberOfChannels: 6,
      sampleRate: 48_000,
    },
  )
})

test('registerCompatibleAudioCodec adds a future container codec adapter', () => {
  registerCompatibleAudioCodec({
    codec: 'future-audio',
    matchesInternalCodecId: (codecId) => codecId === 'A_FUTURE',
  })

  assert.equal(resolveCompatibleAudioCodec(null, 'A_FUTURE'), 'future-audio')
})
