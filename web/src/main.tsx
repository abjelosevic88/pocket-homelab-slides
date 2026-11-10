import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/slides.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Home from './pages/Home'
import DeckPage from './pages/DeckPage'
import Present from './pages/Present'
import Presenter from './pages/Presenter'
import Audience, { FollowCode } from './pages/Audience'
import Shared, { Print } from './pages/Shared'
import Talks, { ShowPage } from './pages/Talks'
import Library from './pages/Library'
import Settings from './pages/Settings'
import { DataProvider } from './lib/data'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <>{children}</>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/w" element={<ErrorBoundary where="audience"><FollowCode /></ErrorBoundary>} />
      <Route path="/w/:key" element={<ErrorBoundary where="audience"><Audience /></ErrorBoundary>} />
      <Route path="/s/:key" element={<ErrorBoundary where="share"><Shared /></ErrorBoundary>} />
      <Route path="/s/:key/print" element={<ErrorBoundary where="share"><Print shared /></ErrorBoundary>} />
      <Route path="/r/:key" element={<ErrorBoundary where="presenter"><Presenter remote /></ErrorBoundary>} />
      <Route path="/present/:id" element={<Gate><ErrorBoundary where="present"><Present /></ErrorBoundary></Gate>} />
      <Route path="/present/:id/presenter" element={<Gate><ErrorBoundary where="presenter"><Presenter /></ErrorBoundary></Gate>} />
      <Route path="/d/:id/print" element={<Gate><ErrorBoundary where="print"><Print /></ErrorBoundary></Gate>} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><DataProvider><Settings /></DataProvider></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="slides"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Home />} />
        <Route path="/d/:id" element={<DeckPage />} />
        <Route path="/d/:id/shows/:sid" element={<ShowPage />} />
        <Route path="/d/:id/:tab" element={<DeckPage />} />
        <Route path="/talks" element={<Talks />} />
        <Route path="/library" element={<Library />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
