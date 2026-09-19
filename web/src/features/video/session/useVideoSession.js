import { useContext } from 'react'
import { VideoSessionContext } from './videoSessionContext.js'

export function useVideoSession() {
  const value = useContext(VideoSessionContext)
  if (!value) {
    throw new Error('useVideoSession must be used within VideoSessionProvider')
  }
  return value
}
