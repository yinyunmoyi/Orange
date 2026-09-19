import assert from 'node:assert/strict'
import test from 'node:test'
import {
  formatLastPlayedTime,
  formatMediaTime,
} from './videoTime.js'

test('formatMediaTime formats minutes and hours', () => {
  assert.equal(formatMediaTime(0), '00:00')
  assert.equal(formatMediaTime(72), '01:12')
  assert.equal(formatMediaTime(3672), '01:01:12')
})

test('formatMediaTime treats invalid values as zero', () => {
  assert.equal(formatMediaTime(Number.NaN), '00:00')
  assert.equal(formatMediaTime(-5), '00:00')
})

test('formatLastPlayedTime formats a local date and minute', () => {
  const timestamp = new Date(2026, 7, 23, 9, 5, 47).getTime()
  assert.equal(formatLastPlayedTime(timestamp), '2026-08-23 09:05')
})

test('formatLastPlayedTime handles missing and invalid timestamps', () => {
  assert.equal(formatLastPlayedTime(0), '时间未知')
  assert.equal(formatLastPlayedTime('invalid'), '时间未知')
})
