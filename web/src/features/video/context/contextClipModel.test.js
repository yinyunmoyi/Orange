import assert from 'node:assert/strict'
import test from 'node:test'
import { buildContextClip } from './contextClipModel.js'

const cueRanges = [
  cue('one', 0, 3, 1, 3),
  cue('two', 3, 7, 3, 7),
  cue('three', 7, 11, 7, 11),
]

test('buildContextClip covers every cue intersecting the backend sentence', () => {
  const result = buildContextClip({
    sentenceStart: 2,
    sentenceEnd: 9,
    cueRanges,
  })

  assert.equal(result.clipStartSeconds, 1)
  assert.equal(result.clipEndSeconds, 11)
  assert.equal(result.durationSeconds, 10)
  assert.deepEqual(result.subtitles, [
    { startMs: 0, endMs: 2_000, text: 'one' },
    { startMs: 2_000, endMs: 6_000, text: 'two' },
    { startMs: 6_000, endMs: 10_000, text: 'three' },
  ])
})

test('buildContextClip uses exact half-open sentence boundaries', () => {
  const result = buildContextClip({
    sentenceStart: 3,
    sentenceEnd: 7,
    cueRanges,
  })

  assert.equal(result.clipStartSeconds, 3)
  assert.equal(result.clipEndSeconds, 7)
  assert.deepEqual(result.subtitles, [
    { startMs: 0, endMs: 4_000, text: 'two' },
  ])
})

test('buildContextClip focuses long sentences around the selected cue', () => {
  const result = buildContextClip({
    sentenceStart: 0,
    sentenceEnd: 80,
    focusStart: 33,
    focusEnd: 36,
    cueRanges: [
      cue('dialogue-1', 0, 10, 33.42, 36.571),
      cue('dialogue-2', 10, 20, 36.74, 42.27),
      cue('dialogue-3', 20, 30, 42.34, 43.84),
      cue('statutory', 30, 40, 43.84, 47.175),
      cue('dialogue-5', 40, 50, 47.24, 49.855),
      cue('dialogue-6', 50, 60, 49.855, 51.355),
      cue('dialogue-7', 60, 70, 51.36, 53.36),
      cue('dialogue-8', 70, 80, 53.36, 54.89),
    ],
  })

  assert.equal(result.clipStartSeconds, 42.34)
  assert.equal(result.clipEndSeconds, 53.36)
  assert.ok(Math.abs(result.durationSeconds - 11.02) < 0.001)
  assert.deepEqual(
    result.subtitles.map((subtitle) => subtitle.text),
    [
      'dialogue-3',
      'statutory',
      'dialogue-5',
      'dialogue-6',
      'dialogue-7',
    ],
  )
})

test('buildContextClip rejects missing and overlong ranges', () => {
  assert.throws(
    () =>
      buildContextClip({
        sentenceStart: 30,
        sentenceEnd: 40,
        cueRanges,
      }),
    /无法定位/,
  )
  assert.throws(
    () =>
      buildContextClip({
        sentenceStart: 0,
        sentenceEnd: 5,
        cueRanges: [cue('long', 0, 5, 0, 61)],
      }),
    /时长无效/,
  )
})

function cue(text, textStart, textEnd, startSeconds, endSeconds) {
  return { text, textStart, textEnd, startSeconds, endSeconds }
}
