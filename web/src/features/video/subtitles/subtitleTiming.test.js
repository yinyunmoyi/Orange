import assert from 'node:assert/strict'
import test from 'node:test'
import {
  findActiveCues,
  findHighlightedCue,
  isCueActive,
} from './subtitleTiming.js'

const cue = { startSeconds: 129.54, endSeconds: 132.52 }

test('isCueActive tolerates media floating point drift at cue start', () => {
  assert.equal(isCueActive(cue, 129.539999), true)
})

test('isCueActive does not extend the cue end', () => {
  assert.equal(isCueActive(cue, 132.52), false)
})

const cues = [
  { id: 'first', startSeconds: 2, endSeconds: 4 },
  { id: 'second', startSeconds: 6, endSeconds: 8 },
  { id: 'third', startSeconds: 10, endSeconds: 12 },
]

test('findHighlightedCue always returns a cue across timeline gaps', () => {
  assert.equal(findHighlightedCue(cues, 0).id, 'first')
  assert.equal(findHighlightedCue(cues, 3).id, 'first')
  assert.equal(findHighlightedCue(cues, 5).id, 'first')
  assert.equal(findHighlightedCue(cues, 6).id, 'second')
  assert.equal(findHighlightedCue(cues, 9).id, 'second')
  assert.equal(findHighlightedCue(cues, 20).id, 'third')
})

test('findHighlightedCue handles empty and near-start timelines', () => {
  assert.equal(findHighlightedCue([], 5), null)
  assert.equal(findHighlightedCue(cues, 5.99).id, 'second')
})

test('findActiveCues retains every subtitle visible in an overlap', () => {
  const overlappingCues = [
    { id: 'annotation', startSeconds: 12.16, endSeconds: 16.69 },
    { id: 'dialogue', startSeconds: 14.03, endSeconds: 16.69 },
  ]

  assert.deepEqual(
    findActiveCues(overlappingCues, 14.5).map((item) => item.id),
    ['annotation', 'dialogue'],
  )
  assert.deepEqual(
    findActiveCues(overlappingCues, 13).map((item) => item.id),
    ['annotation'],
  )
})
