import { parseSrt } from './parsers/srt.js'

const parsers = new Map()

export function registerSubtitleParser(extension, parser) {
  const normalized = normalizeExtension(extension)
  if (!normalized || typeof parser !== 'function') {
    throw new Error('无效的字幕解析器')
  }
  parsers.set(normalized, parser)
}

export async function parseSubtitleHandle(handle) {
  const file = await handle.getFile()
  const extension = fileExtension(file.name)
  const parser = parsers.get(extension)
  if (!parser) {
    throw new Error(`当前版本不支持 .${extension || '未知'} 字幕`)
  }

  return parser(await file.text(), { fileName: file.name })
}

export function validateSubtitleTimeline(document, duration, tolerance = 2) {
  if (!document?.cues?.length) {
    throw new Error('字幕文件中没有可用内容')
  }
  if (!Number.isFinite(duration) || duration <= 0) {
    throw new Error('视频时长无效')
  }

  for (const cue of document.cues) {
    if (
      !Number.isFinite(cue.startSeconds) ||
      !Number.isFinite(cue.endSeconds) ||
      cue.startSeconds < 0 ||
      cue.endSeconds <= cue.startSeconds
    ) {
      throw new Error('字幕文件包含无效时间轴')
    }
  }

  const latestEnd = Math.max(...document.cues.map((cue) => cue.endSeconds))
  if (latestEnd > duration + tolerance) {
    throw new Error('字幕时间轴超出视频时长，请确认文件是否匹配')
  }
}

function fileExtension(name) {
  const dot = name.lastIndexOf('.')
  return dot >= 0 ? normalizeExtension(name.slice(dot + 1)) : ''
}

function normalizeExtension(extension) {
  return String(extension).replace(/^\./, '').trim().toLowerCase()
}

registerSubtitleParser('srt', parseSrt)
