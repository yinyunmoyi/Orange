const adapters = []

export function registerMediaFormat(adapter) {
  const normalized = normalizeAdapter(adapter)
  const existingIndex = adapters.findIndex((item) => item.id === normalized.id)
  if (existingIndex >= 0) adapters.splice(existingIndex, 1)
  adapters.push(normalized)
}

export function resolveMediaFormat(file) {
  const extension = fileExtension(file?.name)
  const mimeType = normalizeMimeType(file?.type)
  const adapter = adapters.find(
    (item) =>
      item.extensions.includes(extension) ||
      (mimeType && item.mimeTypes.includes(mimeType)),
  )
  if (!adapter) {
    const suffix = extension ? `.${extension}` : '该'
    throw new Error(`当前版本不支持 ${suffix} 视频`)
  }
  return adapter
}

export function supportedMediaDescription() {
  return adapters
    .flatMap((adapter) => adapter.extensions)
    .map((extension) => extension.toUpperCase())
    .join(' / ')
}

function normalizeAdapter(adapter) {
  const id = String(adapter?.id || '').trim()
  const description = String(adapter?.description || '').trim()
  const extensions = uniqueValues(adapter?.extensions, normalizeExtension)
  const mimeTypes = uniqueValues(adapter?.mimeTypes, normalizeMimeType)
  if (!id || !description || !extensions.length || !mimeTypes.length) {
    throw new Error('无效的视频格式适配器')
  }
  return {
    id,
    description,
    extensions,
    mimeTypes,
    fallbackMimeType: normalizeMimeType(adapter.fallbackMimeType) || mimeTypes[0],
    metadataAdapter:
      adapter.metadataAdapter === 'mediabunny' ? 'mediabunny' : 'native',
  }
}

function uniqueValues(values, normalize) {
  return [
    ...new Set(
      (Array.isArray(values) ? values : []).map(normalize).filter(Boolean),
    ),
  ]
}

function fileExtension(name) {
  const dot = String(name || '').lastIndexOf('.')
  return dot >= 0 ? normalizeExtension(String(name).slice(dot + 1)) : ''
}

function normalizeExtension(extension) {
  return String(extension || '').replace(/^\./, '').trim().toLowerCase()
}

function normalizeMimeType(mimeType) {
  return String(mimeType || '').split(';', 1)[0].trim().toLowerCase()
}

registerMediaFormat({
  id: 'quicktime',
  description: 'QuickTime 视频',
  extensions: ['mov'],
  mimeTypes: ['video/quicktime'],
})

registerMediaFormat({
  id: 'mp4',
  description: 'MPEG-4 视频',
  extensions: ['mp4', 'm4v'],
  mimeTypes: ['video/mp4', 'video/x-m4v'],
  fallbackMimeType: 'video/mp4',
})

registerMediaFormat({
  id: 'matroska',
  description: 'Matroska 视频',
  extensions: ['mkv'],
  mimeTypes: ['video/x-matroska', 'video/matroska'],
  fallbackMimeType: 'video/x-matroska',
  metadataAdapter: 'mediabunny',
})
