import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import './index.css'
import AppLayout from './components/AppLayout.jsx'
import HomePage from './pages/HomePage.jsx'
import FavoriteListPage from './pages/FavoriteListPage.jsx'
import WordDetailPage from './pages/WordDetailPage.jsx'
import SimilarGroupListPage from './pages/SimilarGroupListPage.jsx'
import SimilarGroupDetailPage from './pages/SimilarGroupDetailPage.jsx'
import SentencePage from './pages/SentencePage.jsx'
import VideoPage from './pages/VideoPage.jsx'
import VideoPlayerPage from './pages/VideoPlayerPage.jsx'
import { VideoSessionProvider } from './features/video/session/VideoSessionContext.jsx'
import { installRuntimeDiagnostics } from './diagnostics/runtimeDiagnostics.js'

installRuntimeDiagnostics()

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <BrowserRouter>
      <VideoSessionProvider>
        <Routes>
          <Route path="/" element={<AppLayout />}>
            <Route index element={<HomePage />} />
            <Route path="console/list" element={<FavoriteListPage />} />
            <Route path="console/word/:word" element={<WordDetailPage />} />
            <Route path="console/same_list" element={<SimilarGroupListPage />} />
            <Route
              path="console/same_group/:id"
              element={<SimilarGroupDetailPage />}
            />
            <Route path="console/sentence" element={<SentencePage />} />
            <Route path="console/videos" element={<VideoPage />} />
            <Route
              path="console/videos/player"
              element={<VideoPlayerPage />}
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </VideoSessionProvider>
    </BrowserRouter>
  </StrictMode>,
)
