import assert from 'node:assert/strict'
import test from 'node:test'
import { formatCuesToWebVTT } from './webVtt.js'

test('formatCuesToWebVTT preserves timing, settings, and multiline text', () => {
  assert.equal(
    formatCuesToWebVTT([
      {
        timestamp: 61.25,
        duration: 2.5,
        text: 'First line\nSecond line',
        settings: 'align:start',
      },
    ]),
    [
      'WEBVTT',
      '',
      '1',
      '00:01:01.250 --> 00:01:03.750 align:start',
      'First line',
      'Second line',
      '',
    ].join('\n'),
  )
})
