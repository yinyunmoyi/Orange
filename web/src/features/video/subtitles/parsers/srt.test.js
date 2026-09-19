import assert from 'node:assert/strict'
import test from 'node:test'
import { validateSubtitleTimeline } from '../index.js'
import { cleanSubtitleLine, parseSrt } from './srt.js'

const SAMPLE = `\uFEFF1\r
00:00:10,010 --> 00:00:11,250\r
{\\fs16\\an2\\b0}大家好\r
Hello,\r
\r
2\r
00:00:20,000 --> 00:00:35,000\r
{\\an8}<font color=#FFFFFF>添加微信公众号：ABContainer\r
\r
3\r
00:00:20,450 --> 00:00:24,060\r
{\\an8}旗帜学说明\r
\r
4\r
00:00:20,450 --> 00:00:24,060\r
the dynamic world of vexillology.\r
`

test('parseSrt cleans tags and merges timeline annotations into dialogue', () => {
  const document = parseSrt(SAMPLE, { fileName: 'sample.srt' })

  assert.equal(document.format, 'srt')
  assert.equal(document.fileName, 'sample.srt')
  assert.equal(document.cues.length, 2)
  assert.deepEqual(document.cues[0].primaryLines, ['Hello,'])
  assert.deepEqual(document.cues[0].translationLines, ['大家好'])
  assert.equal(document.cues[1].startSeconds, 20.45)
  assert.deepEqual(document.cues[1].primaryLines, [
    'the dynamic world of vexillology.',
  ])
  assert.deepEqual(document.cues[1].annotationLines, ['旗帜学说明'])
})

test('cleanSubtitleLine removes display controls without removing text', () => {
  assert.equal(
    cleanSubtitleLine(
      '{\\an8}{\\fn方正黑体简体\\fs18}<font color=#D9D919>术语说明',
    ),
    '术语说明',
  )
})

test('validateSubtitleTimeline accepts a compatible timeline', () => {
  const document = parseSrt(SAMPLE)
  assert.doesNotThrow(() => validateSubtitleTimeline(document, 40))
})

test('validateSubtitleTimeline rejects empty and overflowing timelines', () => {
  assert.throws(
    () => validateSubtitleTimeline({ cues: [] }, 40),
    /没有可用内容/,
  )
  assert.throws(
    () => validateSubtitleTimeline(parseSrt(SAMPLE), 10),
    /超出视频时长/,
  )
})

test('validateSubtitleTimeline rejects invalid cue ranges', () => {
  const document = {
    cues: [{ startSeconds: 5, endSeconds: 4 }],
  }
  assert.throws(() => validateSubtitleTimeline(document, 40), /无效时间轴/)
})
