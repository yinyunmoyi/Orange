import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Captions,
  Clock3,
  FileVideo,
  LoaderCircle,
  Play,
} from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import {
  createVideoEntry,
  getProgressPercent,
  getResumeTime,
} from '../features/video/library/videoLibraryModel.js'
import {
  getVideoEntry,
  listVideoEntries,
  replaceChangedVideoEntry,
  touchVideoEntry,
  upsertVideoEntry,
} from '../features/video/library/videoLibraryRepository.js'
import {
  formatLastPlayedTime,
  formatMediaTime,
} from '../features/video/library/videoTime.js'
import {
  createEphemeralFileHandle,
  createVideoThumbnail,
  ensureFileReadPermission,
  isPersistentFileHandle,
  loadMediaAsset,
  pickFileHandle,
  revokeMediaUrl,
  SUBTITLE_PICKER_OPTIONS,
  VIDEO_FORMAT_DESCRIPTION,
  VIDEO_PICKER_OPTIONS,
} from '../features/video/media/localMediaSource.js'
import { useVideoSession } from '../features/video/session/useVideoSession.js'
import {
  parseSubtitleHandle,
  validateSubtitleTimeline,
} from '../features/video/subtitles/index.js'
import './VideoPage.css'

export default function VideoPage() {
  const navigate = useNavigate()
  const { replaceSession, clearSession } = useVideoSession()
  const [videoHandle, setVideoHandle] = useState(null)
  const [subtitleHandle, setSubtitleHandle] = useState(null)
  const [processing, setProcessing] = useState(false)
  const [entries, setEntries] = useState([])
  const [libraryLoading, setLibraryLoading] = useState(true)
  const [openingEntryId, setOpeningEntryId] = useState('')
  const [error, setError] = useState('')
  const videoInputRef = useRef(null)
  const subtitleInputRef = useRef(null)
  const taskRef = useRef(0)

  useEffect(() => {
    clearSession()
    let active = true
    listVideoEntries()
      .then((records) => {
        if (active) setEntries(records)
      })
      .catch((reason) => {
        if (active) setError(reason.message || '读取播放列表失败')
      })
      .finally(() => {
        if (active) setLibraryLoading(false)
      })
    return () => {
      active = false
    }
  }, [clearSession])

  useEffect(() => {
    if (!videoHandle || !subtitleHandle) return undefined

    const taskId = ++taskRef.current
    let candidateUrl = ''
    setProcessing(true)
    setError('')

    ;(async () => {
      try {
        const subtitles = await parseSubtitleHandle(subtitleHandle)
        if (taskRef.current !== taskId) return

        const media = await loadMediaAsset(videoHandle)
        candidateUrl = media.videoUrl
        if (taskRef.current !== taskId) {
          revokeMediaUrl(candidateUrl)
          candidateUrl = ''
          return
        }

        validateSubtitleTimeline(subtitles, media.asset.duration)
        const persistent =
          isPersistentFileHandle(videoHandle) &&
          isPersistentFileHandle(subtitleHandle)
        let entry

        if (persistent) {
          const [existingEntry, thumbnailDataUrl] = await Promise.all([
            getVideoEntry(media.asset.id),
            createVideoThumbnail(media.videoUrl, media.asset.duration),
          ])
          if (taskRef.current !== taskId) {
            revokeMediaUrl(media.videoUrl)
            candidateUrl = ''
            return
          }
          entry = createVideoEntry({
            existingEntry,
            asset: media.asset,
            subtitleFileName: subtitles.fileName,
            subtitleHandle,
            thumbnailDataUrl,
          })
          await upsertVideoEntry(entry)
          if (taskRef.current !== taskId) {
            revokeMediaUrl(media.videoUrl)
            candidateUrl = ''
            return
          }
        }

        replaceSession({
          asset: media.asset,
          videoUrl: media.videoUrl,
          playbackSource: media.playbackSource,
          subtitles,
          libraryEntryId: entry?.id || null,
          initialTime: entry ? getResumeTime(entry) : 0,
          persistenceWarning: persistent
            ? ''
            : '当前浏览器只能临时播放，播放列表与进度不会跨重启保存',
        })
        candidateUrl = ''
        navigate('/console/videos/player')
      } catch (reason) {
        revokeMediaUrl(candidateUrl)
        candidateUrl = ''
        if (taskRef.current === taskId) {
          setError(reason.message || '视频与字幕加载失败')
        }
      } finally {
        if (taskRef.current === taskId) setProcessing(false)
      }
    })()

    return () => {
      if (taskRef.current === taskId) taskRef.current += 1
      revokeMediaUrl(candidateUrl)
    }
  }, [videoHandle, subtitleHandle, navigate, replaceSession])

  const openLibraryEntry = useCallback(
    async (entry) => {
      if (openingEntryId) return
      let candidateUrl = ''
      setOpeningEntryId(entry.id)
      setError('')

      try {
        await ensureFileReadPermission(entry.videoHandle)
        await ensureFileReadPermission(entry.subtitleHandle)

        const subtitles = await parseSubtitleHandle(entry.subtitleHandle)
        const media = await loadMediaAsset(entry.videoHandle)
        candidateUrl = media.videoUrl
        validateSubtitleTimeline(subtitles, media.asset.duration)

        let activeEntry
        if (media.asset.id === entry.id) {
          activeEntry = await touchVideoEntry(entry.id)
        } else {
          const thumbnailDataUrl = await createVideoThumbnail(
            media.videoUrl,
            media.asset.duration,
          )
          activeEntry = createVideoEntry({
            asset: media.asset,
            subtitleFileName: subtitles.fileName,
            subtitleHandle: entry.subtitleHandle,
            thumbnailDataUrl,
          })
          await replaceChangedVideoEntry(entry.id, activeEntry)
        }

        replaceSession({
          asset: media.asset,
          videoUrl: media.videoUrl,
          playbackSource: media.playbackSource,
          subtitles,
          libraryEntryId: activeEntry.id,
          initialTime: getResumeTime(activeEntry),
          persistenceWarning: '',
        })
        candidateUrl = ''
        navigate('/console/videos/player')
      } catch (reason) {
        revokeMediaUrl(candidateUrl)
        setError(reason.message || '无法打开该视频记录')
      } finally {
        setOpeningEntryId('')
      }
    },
    [navigate, openingEntryId, replaceSession],
  )

  const chooseFile = async (kind) => {
    setError('')
    const options =
      kind === 'video' ? VIDEO_PICKER_OPTIONS : SUBTITLE_PICKER_OPTIONS
    const fallbackRef =
      kind === 'video' ? videoInputRef.current : subtitleInputRef.current

    try {
      const handle = await pickFileHandle(options)
      if (!handle) {
        fallbackRef?.click()
        return
      }
      setHandle(kind, handle)
    } catch (reason) {
      if (reason.name !== 'AbortError') {
        setError(reason.message || '无法打开文件选择器')
      }
    }
  }

  const handleFallbackFile = (kind, event) => {
    const [file] = event.target.files || []
    event.target.value = ''
    if (file) setHandle(kind, createEphemeralFileHandle(file))
  }

  const setHandle = (kind, handle) => {
    taskRef.current += 1
    setProcessing(false)
    setError('')
    if (kind === 'video') setVideoHandle(handle)
    else setSubtitleHandle(handle)
  }

  return (
    <main className="video-picker-page">
      <header className="video-picker-page__header">
        <h1 className="video-picker-page__title">视频管理</h1>
      </header>

      <section className="video-picker" aria-label="选择视频和字幕">
        <FileRow
          label="视频文件"
          value={videoHandle?.name || ''}
          placeholder={`请选择 ${VIDEO_FORMAT_DESCRIPTION} 视频`}
          buttonLabel="选视频"
          icon={FileVideo}
          onChoose={() => chooseFile('video')}
          disabled={processing}
        />
        <FileRow
          label="字幕文件"
          value={subtitleHandle?.name || ''}
          placeholder="请选择双语 SRT 字幕"
          buttonLabel="选字幕文件"
          icon={Captions}
          onChoose={() => chooseFile('subtitle')}
          disabled={processing}
        />

        <input
          ref={videoInputRef}
          className="video-picker__native-input"
          type="file"
          aria-label="上传视频文件"
          onChange={(event) => handleFallbackFile('video', event)}
          tabIndex={-1}
        />
        <input
          ref={subtitleInputRef}
          className="video-picker__native-input"
          type="file"
          accept=".srt,application/x-subrip"
          aria-label="上传字幕文件"
          onChange={(event) => handleFallbackFile('subtitle', event)}
          tabIndex={-1}
        />

        {processing && (
          <p className="video-picker__status" role="status">
            <LoaderCircle size={17} aria-hidden="true" />
            正在解析视频与字幕
          </p>
        )}
      </section>

      {error && (
        <p className="video-library__error" role="alert">
          {error}
        </p>
      )}

      <section className="video-library" aria-labelledby="video-library-title">
        <div className="video-library__heading">
          <h2 id="video-library-title">播放列表</h2>
          {!libraryLoading && (
            <span className="video-library__count">{entries.length} 个视频</span>
          )}
        </div>

        {libraryLoading ? (
          <div className="video-library__state" role="status">
            <LoaderCircle size={20} aria-hidden="true" />
            正在读取播放列表
          </div>
        ) : entries.length === 0 ? (
          <div className="video-library__state">
            <FileVideo size={28} aria-hidden="true" />
            暂无视频
          </div>
        ) : (
          <div className="video-library__list" aria-busy={Boolean(openingEntryId)}>
            {entries.map((entry) => (
              <VideoLibraryItem
                key={entry.id}
                entry={entry}
                loading={openingEntryId === entry.id}
                disabled={Boolean(openingEntryId)}
                onOpen={() => openLibraryEntry(entry)}
              />
            ))}
          </div>
        )}
      </section>
    </main>
  )
}

function FileRow({
  label,
  value,
  placeholder,
  buttonLabel,
  icon: Icon,
  onChoose,
  disabled,
}) {
  return (
    <div className="video-file-row">
      <label className="video-file-row__label">
        <span>{label}</span>
        <input
          className="video-file-row__input"
          value={value}
          placeholder={placeholder}
          readOnly
        />
      </label>
      <button
        type="button"
        className="video-file-row__button"
        onClick={onChoose}
        disabled={disabled}
      >
        <Icon size={18} aria-hidden="true" />
        {buttonLabel}
      </button>
    </div>
  )
}

function VideoLibraryItem({ entry, loading, disabled, onOpen }) {
  const progress = getProgressPercent(entry)

  return (
    <button
      type="button"
      className={`video-library-item${loading ? ' video-library-item--loading' : ''}`}
      onClick={onOpen}
      disabled={disabled}
      title={entry.fileName}
    >
      <span className="video-library-item__thumbnail">
        {entry.thumbnailDataUrl ? (
          <img src={entry.thumbnailDataUrl} alt="" />
        ) : (
          <FileVideo size={30} aria-hidden="true" />
        )}
        <span className="video-library-item__play">
          {loading ? (
            <LoaderCircle size={20} aria-hidden="true" />
          ) : (
            <Play size={19} fill="currentColor" aria-hidden="true" />
          )}
        </span>
      </span>
      <span className="video-library-item__body">
        <span className="video-library-item__title">{entry.title}</span>
        <span className="video-library-item__subtitle">
          {entry.subtitleFileName}
        </span>
        <span className="video-library-item__last-played">
          <Clock3 size={13} strokeWidth={1.8} aria-hidden="true" />
          最近播放 {formatLastPlayedTime(entry.lastPlayedAt || entry.createdAt)}
        </span>
        <span className="video-library-item__progress-row">
          <progress
            className="video-library-item__progress"
            max="100"
            value={progress}
            aria-label={`${entry.title} 播放进度`}
          />
          <span className="video-library-item__time">
            {formatMediaTime(entry.currentTime)} /{' '}
            {formatMediaTime(entry.duration)}
          </span>
        </span>
      </span>
    </button>
  )
}
