const compatibleAudioCodecs = []

export function registerCompatibleAudioCodec({
  codec,
  matchesInternalCodecId,
}) {
  const normalizedCodec = String(codec || '').trim()
  if (!normalizedCodec || typeof matchesInternalCodecId !== 'function') {
    throw new Error('无效的音频编码兼容适配器')
  }
  compatibleAudioCodecs.push({ codec: normalizedCodec, matchesInternalCodecId })
}

export function resolveCompatibleAudioCodec(codec, internalCodecId) {
  if (codec) return codec
  const normalizedId = String(internalCodecId || '').trim().toUpperCase()
  return (
    compatibleAudioCodecs.find((adapter) =>
      adapter.matchesInternalCodecId(normalizedId),
    )?.codec || null
  )
}

export function createCompatibleAudioDecoderConfig(
  codec,
  internalCodecId,
  { numberOfChannels, sampleRate } = {},
) {
  const compatibleCodec = resolveCompatibleAudioCodec(codec, internalCodecId)
  if (!compatibleCodec) return null

  return {
    codec: compatibleCodec,
    numberOfChannels: positiveInteger(numberOfChannels, 2),
    sampleRate: positiveInteger(sampleRate, 48000),
  }
}

function positiveInteger(value, fallback) {
  const number = Number(value)
  return Number.isInteger(number) && number > 0 ? number : fallback
}

registerCompatibleAudioCodec({
  codec: 'dts',
  matchesInternalCodecId: (codecId) => codecId.startsWith('A_DTS'),
})
