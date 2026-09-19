import assert from 'node:assert/strict'
import test from 'node:test'
import {
  parseSubtitleHandle,
  registerSubtitleParser,
} from './index.js'

test('subtitle parser registry accepts a new file format adapter', async () => {
  registerSubtitleParser('.demo', (text, { fileName }) => ({
    format: 'demo',
    fileName,
    cues: [{ id: '1', sourceLines: [text] }],
  }))

  const handle = {
    getFile: async () => new File(['adapter-content'], 'sample.demo'),
  }
  const document = await parseSubtitleHandle(handle)

  assert.equal(document.format, 'demo')
  assert.equal(document.fileName, 'sample.demo')
  assert.deepEqual(document.cues[0].sourceLines, ['adapter-content'])
})
