// Helpers every deck screen shares: the shown slides, picture addresses,
// the live event stream and a few words about times.
import { useEffect, useRef, useState } from 'react'
import type { LiveState, Slide } from './types'
import { V1 } from './api'
import { fmtDate, fmtTime } from './format'

export const shown = (slides: Slide[]) => slides.filter(s => !s.hidden)

/** The agenda: the titles of the section slides, in order. */
export const agendaOf = (slides: Slide[]) => shown(slides).filter(s => s.layout === 'section' && s.title).map(s => s.title)

export type ImgFn = (id: number, thumb?: boolean) => string
export const ownImage: ImgFn = (id, thumb) => `${V1}/uploads/${id}${thumb ? '?thumb=1' : ''}`
export const keyImage = (kind: 's' | 'live' | 'remote', key: string): ImgFn => (id, thumb) => `${V1}/public/${kind}/${encodeURIComponent(key)}/uploads/${id}${thumb ? '?thumb=1' : ''}`

export const origin = () => window.location.origin

/** A link from the server as a full address (it sends paths when it does not know its own address). */
export const abs = (u?: string) => !u ? '' : /^https?:\/\//.test(u) ? u : origin() + (u.startsWith('/') ? u : '/' + u)

/** "Thu 14 Nov, 10:30", "tomorrow", "in 3 days" for a talk's local time. */
export function talkWhen(at: string, short = false): string {
  if (!at) return ''
  const d = new Date(at.length === 10 ? at + 'T00:00' : at)
  if (isNaN(+d)) return at
  const today = new Date(); today.setHours(0, 0, 0, 0)
  const day = new Date(d); day.setHours(0, 0, 0, 0)
  const days = Math.round((+day - +today) / 86400000)
  const time = at.length > 10 ? fmtTime(d) : ''
  if (short) return days === 0 ? 'today' : days === 1 ? 'tomorrow' : days > 1 && days < 7 ? `in ${days} d` : fmtDate(at.slice(0, 10), false)
  const when = days === 0 ? 'Today' : days === 1 ? 'Tomorrow' : days === -1 ? 'Yesterday' : fmtDate(at.slice(0, 10))
  return time ? `${when}, ${time}` : when
}

/** Follow a deck's live state over an event stream; reconnects by itself. */
export function useLive(url: string | null, onEvent?: (e: LiveState) => void) {
  const [state, setState] = useState<LiveState | null>(null)
  const [online, setOnline] = useState(true)
  const cb = useRef(onEvent)
  cb.current = onEvent
  useEffect(() => {
    if (!url) return
    let es: EventSource | null = null
    let stop = false
    let wait = 1000
    let timer = 0
    const open = () => {
      es = new EventSource(url, { withCredentials: true })
      es.onopen = () => { setOnline(true); wait = 1000 }
      es.onmessage = m => {
        try {
          const ev = JSON.parse(m.data) as LiveState
          if (ev.type === 'state' || ev.type === 'ended') setState(ev)
          cb.current?.(ev)
        } catch { /* a ping */ }
      }
      es.onerror = () => {
        setOnline(false)
        es?.close()
        if (!stop) timer = window.setTimeout(open, wait)
        wait = Math.min(wait * 2, 15000)
      }
    }
    open()
    const vis = () => { if (document.visibilityState === 'visible' && es?.readyState === EventSource.CLOSED) { window.clearTimeout(timer); open() } }
    document.addEventListener('visibilitychange', vis)
    return () => { stop = true; window.clearTimeout(timer); es?.close(); document.removeEventListener('visibilitychange', vis) }
  }, [url])
  return { state, setState, online }
}

/** Ticks once a second (for timers). */
export function useNow(on = true) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!on) return
    const t = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(t)
  }, [on])
  return now
}

/** Keep the screen on while presenting (where the browser allows it). */
export function useWakeLock(on: boolean) {
  useEffect(() => {
    if (!on || !('wakeLock' in navigator)) return
    let lock: { release: () => Promise<void> } | null = null
    const get = () => { (navigator as unknown as { wakeLock: { request: (t: string) => Promise<{ release: () => Promise<void> }> } }).wakeLock.request('screen').then(l => { lock = l }).catch(() => {}) }
    get()
    const vis = () => { if (document.visibilityState === 'visible') get() }
    document.addEventListener('visibilitychange', vis)
    return () => { document.removeEventListener('visibilitychange', vis); lock?.release().catch(() => {}) }
  }, [on])
}
