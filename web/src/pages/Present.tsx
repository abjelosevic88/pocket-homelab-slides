// The big screen: one slide, full screen, following the show. Moves with the
// keys, a click, a swipe — or the phone. Before the first press it shows the
// codes for your phone and for the audience.
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { DeckPayload, LiveState } from '../lib/types'
import { canvasOf, lookOf } from '../lib/slides'
import { abs, agendaOf, origin, ownImage, shown, useLive, useWakeLock } from '../lib/deck'
import { Loading, errMsg, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { SlideView } from '../components/SlideView'
import { QRCode } from '../components/QRCode'

export default function Present() {
  const { id } = useParams()
  const [q] = useSearchParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  const rehearse = q.get('rehearse') === '1'
  const from = Number(q.get('from')) || 0
  const [p, setP] = useState<DeckPayload | null>(null)
  const [remote, setRemote] = useState('')
  const [pair, setPair] = useState(true)
  const [cursor, setCursor] = useState(true)
  const [leave, setLeave] = useState(false)
  const [reacts, setReacts] = useState<{ id: number; e: string; x: number }[]>([])
  const [dir, setDir] = useState(1)
  const load = useCallback(() => api.get<DeckPayload>(`${V1}/decks/${id}`).then(setP), [id])
  const onEvent = useCallback((e: LiveState) => {
    if (e.type === 'deck') load()
    if (e.type === 'reaction' && e.emoji) { const r = { id: Math.random(), e: e.emoji, x: 10 + Math.random() * 80 }; setReacts(x => [...x.slice(-12), r]); window.setTimeout(() => setReacts(x => x.filter(y => y.id !== r.id)), 2700) }
  }, [load])
  const { state } = useLive(p ? `${V1}/decks/${id}/live/stream` : null, onEvent)
  useWakeLock(true)

  // start (or join) the show
  useEffect(() => {
    let live = true
    ;(async () => {
      try {
        const d = await api.get<DeckPayload>(`${V1}/decks/${id}`)
        if (!live) return
        setP(d)
        const r = await api.post<{ remote_url: string }>(`${V1}/decks/${id}/live`, { action: 'start', kind: rehearse ? 'rehearsal' : 'talk', pos: from })
        if (!live) return
        setRemote(abs(r.remote_url))
        if (d.show && from) await api.post(`${V1}/decks/${id}/live`, { action: 'go', pos: from })
        if (d.show) setPair(false)
      } catch (e) { toast(errMsg(e), 'err') }
    })()
    return () => { live = false }
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps

  const send = useCallback((body: Record<string, unknown>) => api.post(`${V1}/decks/${id}/live`, body).catch(e => toast(errMsg(e), 'err')), [id, toast])
  const next = useCallback(() => { setPair(false); setDir(1); send({ action: 'next' }) }, [send])
  const prev = useCallback(() => { setDir(-1); send({ action: 'prev' }) }, [send])
  const end = async () => { await send({ action: 'end' }); exitFull(); navigate(`/d/${id}/${rehearse ? 'practice' : 'present'}`) }
  const stay = () => { exitFull(); navigate(`/d/${id}/present`) }

  // keys
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input, textarea')) return
      if (leave) { if (e.key === 'Escape') setLeave(false); return }
      switch (e.key) {
        case 'ArrowRight': case 'ArrowDown': case ' ': case 'PageDown': case 'Enter': case 'n': e.preventDefault(); next(); break
        case 'ArrowLeft': case 'ArrowUp': case 'PageUp': case 'Backspace': e.preventDefault(); prev(); break
        case 'p': case 'P': setPair(x => !x); break
        case 'b': case 'B': case '.': send({ action: 'black' }); break
        case 'f': case 'F': toggleFull(); break
        case 'g': case 'G': { const v = window.prompt('Go to slide'); if (v && Number(v) > 0) send({ action: 'go', pos: Number(v) - 1 }); break }
        case 'Home': send({ action: 'go', pos: 0 }); break
        case 'Escape': if (pair) setPair(false); else if (!document.fullscreenElement) setLeave(true); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [next, prev, send, pair, leave])

  // the pointer and the HUD hide after a moment
  const hide = useRef(0)
  const wake = () => { setCursor(true); window.clearTimeout(hide.current); hide.current = window.setTimeout(() => setCursor(false), 2500) }
  useEffect(() => { wake(); return () => window.clearTimeout(hide.current) }, [])

  // fit the slide to the window
  const [win, setWin] = useState({ w: window.innerWidth, h: window.innerHeight })
  useEffect(() => { const r = () => setWin({ w: window.innerWidth, h: window.innerHeight }); window.addEventListener('resize', r); return () => window.removeEventListener('resize', r) }, [])
  const look = useMemo(() => p ? lookOf(p.deck, p.theme) : null, [p])
  const touch = useTouch(next, prev)
  if (!p || !look) return <div className="present"><Loading /></div>
  const vis = shown(p.slides)
  const { w, h } = canvasOf(p.deck.aspect)
  const k = Math.min(win.w / w, win.h / h)
  const W = Math.floor(w * k), H = Math.floor(h * k)
  const pos = Math.min(state?.pos ?? from, Math.max(0, vis.length - 1))
  const s = vis[pos]
  const tally = s && state?.tally?.[String(s.id)]
  const liveUrl = origin() + '/w/' + p.deck.live_key
  const kind = state?.kind || (rehearse ? 'rehearsal' : 'talk')
  return (
    <div className={`present ${cursor ? 'show-cursor' : ''}`} onMouseMove={wake} {...touch}
      onClick={e => { if (pair || leave || (e.target as HTMLElement).closest('.present-hud, .present-pair')) return; if (e.clientX < window.innerWidth / 3) prev(); else next() }}>
      <div className="present-frame" style={{ width: W, height: H }}>
        {s && <div key={s.id} className={`present-slide t-${p.deck.transition} ${dir < 0 ? 'back' : ''}`}>
          <SlideView slide={s} deck={p.deck} look={look} imageUrl={ownImage} index={pos} total={vis.length} step={state?.step ?? 0} agenda={agendaOf(p.slides)}
            tally={s.layout === 'poll' && kind === 'talk' ? tally || {} : undefined} showAnswer={(state?.step ?? 0) >= 1} qr={{ share: abs(p.share_url), live: liveUrl }} width={W}
            follow={pos === 0 && kind === 'talk' && settings?.audience_link !== false ? { url: origin().replace(/^https?:\/\//, '') + '/w', code: p.deck.live_key } : undefined} />
        </div>}
        <div className="present-black" style={{ opacity: state?.black ? 1 : 0, pointerEvents: 'none' }} />
      </div>
      <div className="present-hud" onClick={e => e.stopPropagation()}>
        <button type="button" className="btn ghost icon-only" aria-label="Back" onClick={prev}><Icon name="chevron-left" size={18} /></button>
        <span className="count">{pos + 1} / {vis.length}{kind === 'rehearsal' ? ' · rehearsal' : state?.viewers ? ` · ${state.viewers} following` : ''}</span>
        <button type="button" className="btn ghost icon-only" aria-label="Next" onClick={next}><Icon name="chevron-right" size={18} /></button>
        <button type="button" className="btn ghost icon-only" title="Phone and audience codes (P)" aria-label="Codes" onClick={() => setPair(true)}><Icon name="smartphone" size={16} /></button>
        <button type="button" className="btn ghost icon-only" title="Black screen (B)" aria-label="Black screen" onClick={() => send({ action: 'black' })}><Icon name="square" size={16} /></button>
        <button type="button" className="btn ghost icon-only" title="Full screen (F)" aria-label="Full screen" onClick={toggleFull}><Icon name="maximize" size={16} /></button>
        <button type="button" className="btn ghost icon-only" title="Stop (esc)" aria-label="Stop" onClick={() => setLeave(true)}><Icon name="x" size={16} /></button>
      </div>
      {reacts.map(r => <span key={r.id} className="reaction-float" style={{ left: `${r.x}%` }}>{r.e}</span>)}
      {pair && <div className="present-pair" onClick={e => { e.stopPropagation(); setPair(false) }}>
        <div className="present-pair-card" onClick={e => e.stopPropagation()}>
          <h2>{kind === 'rehearsal' ? 'Rehearsal' : p.deck.title}</h2>
          <div className="pair-cols">
            <div className="pair-col">
              <b>Your phone</b>
              {remote ? <QRCode value={remote} size={190} label="Presenter screen for your phone" /> : <span className="spinner" />}
              <p>Scan it: the next slide, your notes and a timer, and big buttons to move. No sign-in; the link ends with the show.</p>
            </div>
            {kind === 'talk' ? <div className="pair-col">
              <b>The audience</b>
              <QRCode value={liveUrl} size={190} label="Follow along" />
              <p>{origin().replace(/^https?:\/\//, '')}/w · code <b className="big-code" style={{ fontSize: 18 }}>{p.deck.live_key.toUpperCase()}</b></p>
            </div> : <div className="pair-col"><b>Just you</b><p>A rehearsal: every slide's time is kept and nobody can follow along. You will see where you ran over.</p></div>}
          </div>
          <div className="row wrap"><span className="muted small grow">→ or space to begin · F full screen · B black · esc stop</span><button type="button" className="btn" onClick={toggleFull}><Icon name="maximize" size={15} />Full screen</button><button type="button" className="btn primary" onClick={() => setPair(false)}>Begin</button></div>
        </div>
      </div>}
      {leave && <div className="present-pair" onClick={e => e.stopPropagation()}>
        <div className="present-pair-card" style={{ maxWidth: 460 }}>
          <h2>Stop presenting?</h2>
          <p className="muted">{kind === 'rehearsal' ? 'Ending keeps the time of every slide.' : 'Ending the talk closes the phone link; the audience can then say how it was.'} Or keep it running and come back (from the phone, or this deck's Present tab).</p>
          <div className="row wrap"><button type="button" className="btn" onClick={() => setLeave(false)}>Go on</button><span className="grow" /><button type="button" className="btn" onClick={stay}>Keep it running</button><button type="button" className="btn primary" onClick={end}>End {kind === 'rehearsal' ? 'the rehearsal' : 'the talk'}</button></div>
        </div>
      </div>}
    </div>
  )
}

function toggleFull() { if (document.fullscreenElement) exitFull(); else document.documentElement.requestFullscreen?.().catch(() => {}) }
function exitFull() { if (document.fullscreenElement) document.exitFullscreen?.().catch(() => {}) }

/** Swipe left or right to move. */
export function useTouch(next: () => void, prev: () => void) {
  const x0 = useRef<number | null>(null)
  return {
    onTouchStart: (e: React.TouchEvent) => { x0.current = e.touches[0].clientX },
    onTouchEnd: (e: React.TouchEvent) => { if (x0.current === null) return; const dx = e.changedTouches[0].clientX - x0.current; x0.current = null; if (Math.abs(dx) > 40) (dx < 0 ? next : prev)() },
  }
}
