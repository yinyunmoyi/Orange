import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { revokeMediaUrl } from '../media/localMediaSource.js'
import { VideoSessionContext } from './videoSessionContext.js'

export function VideoSessionProvider({ children }) {
  const [session, setSession] = useState(null)
  const sessionRef = useRef(null)

  const replaceSession = useCallback((nextSession) => {
    const previous = sessionRef.current
    if (previous?.videoUrl && previous.videoUrl !== nextSession.videoUrl) {
      revokeMediaUrl(previous.videoUrl)
    }
    sessionRef.current = nextSession
    setSession(nextSession)
  }, [])

  const clearSession = useCallback(() => {
    if (sessionRef.current?.videoUrl) {
      revokeMediaUrl(sessionRef.current.videoUrl)
    }
    sessionRef.current = null
    setSession(null)
  }, [])

  useEffect(() => {
    return () => {
      if (sessionRef.current?.videoUrl) {
        revokeMediaUrl(sessionRef.current.videoUrl)
      }
    }
  }, [])

  const value = useMemo(
    () => ({ session, replaceSession, clearSession }),
    [session, replaceSession, clearSession],
  )

  return (
    <VideoSessionContext.Provider value={value}>
      {children}
    </VideoSessionContext.Provider>
  )
}
