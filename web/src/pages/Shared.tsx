// The share link: look through a deck (no notes, no skipped slides), print it.
import { useEffect, useMemo, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Deck, Slide, Theme, DeckPayload } from '../lib/types'
import { canvasOf, lookOf } from '../lib/slides'
import { agendaOf, keyImage, ownImage, shown } from '../lib/deck'
import { Loading, errMsg } from '../components/ui'
import { Icon } from '../components/icons'
import { SlideView } from '../components/SlideView'
import { useTouch } from './Present'

type PublicDeck = Pick<Deck, 'title' | 'about' | 'theme' | 'aspect' | 'footer' | 'numbers' | 'transition' | 'live_key' | 'share_key'>
interface View { deck: PublicDeck; slides: Slide[]; theme?: Theme; owner: string }

export default function Shared() {
  const { key = '' } = useParams()
  const [v, setV] = useState<View | null>(null)
  const [err, setErr] = useState('')
  const [i, setI] = useState(() => Math.max(0, Number(window.location.hash.slice(1)) - 1 || 0))
  useEffect(() => { api.get<View>(`${V1}/public/s/${encodeURIComponent(key)}`).then(setV).catch(e => setErr(errMsg(e))) }, [key])
  const n = v?.slides.length || 0
  const go = (d: number) => setI(x => Math.max(0, Math.min(n - 1, x + d)))
  const touch = useTouch(() => go(1), () => go(-1))
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (['ArrowRight', 'ArrowDown', ' ', 'PageDown'].includes(e.key)) { e.preventDefault(); go(1) }
      if (['ArrowLeft', 'ArrowUp', 'PageUp'].includes(e.key)) { e.preventDefault(); go(-1) }
      if (e.key === 'f') document.documentElement.requestFullscreen?.().catch(() => {})
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })
  useEffect(() => { if (v) { window.history.replaceState(null, '', `#${i + 1}`); document.title = v.deck.title } }, [i, v])
  const look = useMemo(() => v ? lookOf(v.deck, v.theme) : null, [v])
  const [win, setWin] = useState({ w: window.innerWidth, h: window.innerHeight })
  useEffect(() => { const r = () => setWin({ w: window.innerWidth, h: window.innerHeight }); window.addEventListener('resize', r); return () => window.removeEventListener('resize', r) }, [])
  if (err) return <div className="share-page"><div className="share-stage"><div className="empty"><h3>{err}</h3></div></div></div>
  if (!v || !look) return <div className="share-page"><Loading /></div>
  const { w, h } = canvasOf(v.deck.aspect)
  const W = Math.floor(Math.min(win.w - 32, (win.h - 130) * w / h, 1600))
  return (
    <div className="share-page" {...touch}>
      <div className="share-top"><img src="/favicon.svg" alt="" width={22} height={22} /><b className="grow ellipsis">{v.deck.title}</b>{v.owner && <span className="small" style={{ color: '#999' }}>{v.owner}</span>}<Link className="btn sm ghost" to={`/s/${key}/print`}><Icon name="printer" size={14} />Print</Link></div>
      <div className="share-stage"><div style={{ width: W }}>
        <SlideView key={v.slides[i]?.id} slide={v.slides[i] || null} deck={v.deck} look={look} imageUrl={keyImage('s', key)} index={i} total={n} agenda={agendaOf(v.slides)} qr={{ share: window.location.origin + '/s/' + key, live: window.location.origin + '/w/' + v.deck.live_key }} width={W} />
      </div></div>
      <div className="share-bottom">
        <button type="button" className="btn" onClick={() => go(-1)} disabled={i === 0} aria-label="Back"><Icon name="chevron-left" size={18} /></button>
        <span>{i + 1} / {n}</span>
        <button type="button" className="btn" onClick={() => go(1)} disabled={i >= n - 1} aria-label="Next"><Icon name="chevron-right" size={18} /></button>
      </div>
    </div>
  )
}

/** Every slide one under the other, a page each when printed (or saved as PDF). */
export function Print({ shared }: { shared?: boolean }) {
  const { id, key = '' } = useParams()
  const [q] = useSearchParams()
  const notes = !shared && q.get('notes') === '1'
  const [p, setP] = useState<{ deck: PublicDeck; slides: Slide[]; theme?: Theme } | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    (shared ? api.get<View>(`${V1}/public/s/${encodeURIComponent(key)}`) : api.get<DeckPayload>(`${V1}/decks/${id}`)).then(r => setP({ deck: r.deck, slides: shared ? r.slides : shown(r.slides), theme: r.theme })).catch(e => setErr(errMsg(e)))
  }, [id, key, shared])
  if (err) return <div className="empty"><h3>{err}</h3></div>
  if (!p) return <Loading />
  const look = lookOf(p.deck, p.theme)
  const { w, h } = canvasOf(p.deck.aspect)
  const W = Math.round(w * 0.8), H = Math.round(h * 0.8)
  const img = shared ? keyImage('s', key) : ownImage
  return (
    <div className={`print-page ${notes ? 'with-notes' : ''}`}>
      <style>{`@media print { @page { size: ${W}px ${notes ? H + 260 : H}px; margin: 0 } .print-page .print-slide { width: ${W}px } }`}</style>
      <div className="print-bar"><b className="grow">{p.deck.title}</b><span className="muted small">{p.slides.length} slides · use “Save as PDF” to keep a file</span><button type="button" className="btn primary" onClick={() => window.print()}><Icon name="printer" size={15} />Print</button></div>
      {p.slides.map((s, i) => (
        <div key={s.id} className="print-slide" style={{ width: W }}>
          <SlideView slide={s} deck={p.deck} look={look} imageUrl={img} index={i} total={p.slides.length} agenda={agendaOf(p.slides)} qr={{ share: window.location.origin + '/s/' + p.deck.share_key, live: window.location.origin + '/w/' + p.deck.live_key }} width={W} />
          {notes && <div className="print-notes" style={{ height: 260, overflow: 'hidden' }}>{s.notes || ' '}</div>}
        </div>
      ))}
    </div>
  )
}
