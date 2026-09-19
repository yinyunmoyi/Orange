import { Input } from 'mediabunny-official'
import {
  createCompatibleAudioDecoderConfig,
  resolveCompatibleAudioCodec,
} from './mediaCodecCompat.js'

if (typeof Input.prototype.getSubtitleTracks !== 'function') {
  Input.prototype.getSubtitleTracks = async () => []
}

const audioCodecPatch = Symbol.for('orange.mediabunny.audio-codec-compat')

if (!Input.prototype[audioCodecPatch]) {
  const getPrimaryAudioTrack = Input.prototype.getPrimaryAudioTrack
  Input.prototype.getPrimaryAudioTrack = async function (...args) {
    const track = await getPrimaryAudioTrack.apply(this, args)
    if (!track || track.codec) return track

    const codec = resolveCompatibleAudioCodec(
      track.codec,
      await track.getInternalCodecId(),
    )
    if (!codec) return track

    Object.defineProperty(track, 'codec', {
      configurable: true,
      value: codec,
    })
    track.getCodec = async () => codec
    track.getDecoderConfig = async () =>
      createCompatibleAudioDecoderConfig(codec, await track.getInternalCodecId(), {
        numberOfChannels: await track.getNumberOfChannels(),
        sampleRate: await track.getSampleRate(),
      })
    return track
  }
  Object.defineProperty(Input.prototype, audioCodecPatch, { value: true })
}

export * from 'mediabunny-official'
export { formatCuesToWebVTT } from './webVtt.js'
