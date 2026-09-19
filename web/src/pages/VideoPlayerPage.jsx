import { useCallback, useState } from 'react'
import { ArrowLeftRight, Pause, Play, RotateCcw } from 'lucide-react'
import { Navigate, useNavigate } from 'react-router-dom'
import { useVideoProgressPersistence } from '../features/video/library/useVideoProgressPersistence.js'
import { formatMediaTime } from '../features/video/library/videoTime.js'
import { useVideoPlayback } from '../features/video/player/useVideoPlayback.js'
import { useVideoSession } from '../features/video/session/useVideoSession.js'
import SubtitlePanel from '../features/video/subtitles/SubtitlePanel.jsx'
import './VideoPlayerPage.css'

export default function VideoPlayerPage() {
  const { session, clearSession } = useVideoSession()
  if (!session) {
    return <Navigate to="/console/videos" replace />
  }
  return <VideoPlayerContent session={session} clearSession={clearSession} />
}

function VideoPlayerContent({ session, clearSession }) {
  const navigate = useNavigate()
  const [showTranslation, setShowTranslation] = useState(true)
  const [progressError, setProgressError] = useState('')
  const [prepareProgress, setPrepareProgress] = useState(null)
  const { onProgress, onPlaybackEvent } = useVideoProgressPersistence({
    entryId: session.libraryEntryId,
    onError: setProgressError,
  })
  const handlePrepareProgress = useCallback((info) => {
    setPrepareProgress(info)
  }, [])
  const {
    videoRef,
    currentTime,
    duration,
    isPlaying,
    isPreparing,
    error,
    togglePlayback,
    pausePlayback,
    seekTo,
  } = useVideoPlayback({
    playbackSource: session.playbackSource,
    initialTime: session.initialTime || 0,
    onProgress,
    onPlaybackEvent,
    onPrepareProgress: handlePrepareProgress,
  })

  const totalDuration = duration || session.asset.duration

  const handleReset = () => {
    clearSession()
    navigate('/console/videos')
  }

  const preparingMessage = describePrepareProgress(prepareProgress)

  return (
    <main className="video-player-page">
      <header className="video-player-page__header">
        <div className="video-player-page__heading">
          <h1 className="video-player-page__title">{session.asset.name}</h1>
          <p className="video-player-page__subtitle">
            {session.subtitles.fileName}
          </p>
        </div>
        <div className="video-player-page__actions">
          <button
            type="button"
            className="video-player-page__subtitle-mode"
            aria-label={
              showTranslation ? '切换为仅英文字幕' : '切换为中英字幕'
            }
            aria-pressed={!showTranslation}
            onClick={() => setShowTranslation((visible) => !visible)}
          >
            <ArrowLeftRight size={17} aria-hidden="true" />
            {showTranslation ? '中英' : '仅英文'}
          </button>
          <button
            type="button"
            className="video-player-page__reset"
            onClick={handleReset}
          >
            <RotateCcw size={17} aria-hidden="true" />
            重新选择
          </button>
        </div>
      </header>

      <div className="video-player-layout">
        <section className="video-playback" aria-label="视频播放器">
          <div className="video-playback__stage">
            <video
              ref={videoRef}
              className="video-playback__video"
              preload="auto"
              onClick={togglePlayback}
              aria-label={session.asset.name}
            />
          </div>

          <div className="video-controls">
            <button
              type="button"
              className="video-controls__play"
              onClick={togglePlayback}
              aria-label={isPlaying ? '暂停' : '播放'}
              title={isPlaying ? '暂停' : '播放'}
            >
              {isPlaying ? (
                <Pause size={20} fill="currentColor" aria-hidden="true" />
              ) : (
                <Play size={20} fill="currentColor" aria-hidden="true" />
              )}
            </button>
            <input
              className="video-controls__progress"
              type="range"
              min="0"
              max={totalDuration || 0}
              step="0.05"
              value={Math.min(currentTime, totalDuration || 0)}
              onChange={(event) => seekTo(Number(event.target.value))}
              aria-label="视频进度"
            />
            <span className="video-controls__time">
              {formatMediaTime(currentTime)} / {formatMediaTime(totalDuration)}
            </span>
          </div>

          {isPreparing && (
            <p className="video-playback__notice" role="status">
              {preparingMessage}
            </p>
          )}
          {error && (
            <div className="video-playback__error" role="alert">
              <span>{error}</span>
              <button type="button" onClick={handleReset}>
                返回重选
              </button>
            </div>
          )}
          {(progressError || session.persistenceWarning) && (
            <p className="video-playback__notice" role="status">
              {progressError || session.persistenceWarning}
            </p>
          )}
        </section>

        <SubtitlePanel
          cues={session.subtitles.cues}
          currentTime={currentTime}
          onSeek={seekTo}
          showTranslation={showTranslation}
          videoUrl={session.videoUrl}
          playbackSource={session.playbackSource}
          sourceFingerprint={session.asset.id}
          onSentenceAnalysisOpen={pausePlayback}
        />
      </div>
    </main>
  )
}

function describePrepareProgress(progress) {
  if (!progress) return '正在准备视频'
  if (progress.phase === 'probing') return '正在检测视频兼容性'
  if (progress.phase === 'cache-check') return '正在检查转码缓存'
  if (progress.phase === 'upload' && progress.total > 0) {
    const percent = Math.min(
      100,
      Math.floor((progress.loaded / progress.total) * 100),
    )
    return `正在上传视频 ${percent}%`
  }
  if (progress.phase === 'transcode') {
    const percent = Math.min(99, Math.max(0, Number(progress.progress) || 0))
    return `正在转码 ${percent}%`
  }
  if (progress.phase === 'loading') return '转码完成，正在加载视频'
  if (progress.phase === 'ready') return '转码完成，正在加载视频'
  return '正在准备视频'
}
