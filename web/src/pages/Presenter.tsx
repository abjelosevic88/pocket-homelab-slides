// The presenter screen, on a phone or a laptop's own screen: the slide on
// the big screen, the next one, the speaker notes, the timer, questions from
// the audience — and big buttons. /r/<key> works without signing in (the
// phone link of one show); /present/<id>/presenter is the signed-in one.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { DeckPayload, LiveState, Question } from '../lib/types'
import { fmtMs, lookOf, slideName, stepsOf } from '../lib/slides'
import { abs, agendaOf, keyImage, origin, ownImage, shown, useLive, useNow, useWakeLock } from '../lib/deck'
import { Loading, errMsg, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { SlideView } from '../components/SlideView'
import { useTouch } from './Present'

interface Prefs { warn_minutes: number; notes_size: number; keep_awake: boolean }

export default function Presenter({ remote }: { remote?: boolean }) {
  const { id, key } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const base = remote ? `${V1}/public/remote/${key}` : `${V1}/decks/${id}`
  const [p, setP] = useState<DeckPayload | null>(null)
  const [prefs, setPrefs] = useState<Prefs>({ warn_minutes: 2, notes_size: 20, keep_awake: true })
  const [err, setErr] = useState('')
  const [size, setSize] = useState(() => Number(localStorage.getItem('psld-notes-size')) || 0)
  const [list, setList] = useState(false)
  const [ended, setEnded] = useState<LiveState | null>(null)
  const load = useCallback(async () => {
    try {
      if (remote) {
        const r = await api.get<DeckPayload & { prefs: Prefs }>(base)
        setP(r); setPrefs(r.prefs)
      } else {
        const [r, s] = await Promise.all([api.get<DeckPayload>(base), api.get<{ warn_minutes: number; notes_size: number; keep_awake: boolean }>(`${V1}/settings`)])
        setP(r); setPrefs({ warn_minutes: s.warn_minutes, notes_size: s.notes_size, keep_awake: s.keep_awake })
      }
    } catch (e) { setErr(errMsg(e)) }
  }, [base, remote])
  useEffect(() => { load() }, [load])
  const onEvent = useCallback((e: LiveState) => {
    if (e.type === 'deck') load()
    if (e.type === 'ended') setEnded(e)
    if (e.type === 'state' && e.live) setEnded(null)
  }, [load])
  const { state, online } = useLive(p ? `${base}/${remote ? 'stream' : 'live/stream'}` : null, onEvent)
  useWakeLock(!!prefs.keep_awake && !!state?.live)
  const now = useNow(!!state?.live)
  const send = useCallback((body: Record<string, unknown>) => api.post(remote ? base : `${base}/live`, body).catch(e => toast(errMsg(e), 'err')), [base, remote, toast])
  const next = useCallback(() => send({ action: 'next' }), [send])
  const prev = useCallback(() => send({ action: 'prev' }), [send])
  const touch = useTouch(next, prev)
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input, textarea')) return
      if (['ArrowRight', 'ArrowDown', ' ', 'PageDown'].includes(e.key)) { e.preventDefault(); next() }
      if (['ArrowLeft', 'ArrowUp', 'PageUp'].includes(e.key)) { e.preventDefault(); prev() }
      if (e.key === 'b' || e.key === '.') send({ action: 'black' })
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [next, prev, send])
  const look = useMemo(() => p ? lookOf(p.deck, p.theme) : null, [p])
  // warn once when the time is nearly up
  const target = (p?.deck.target_minutes || 0) * 60000
  const elapsed = state?.started_at ? now - Date.parse(state.started_at) : 0
  const warnAt = target - prefs.warn_minutes * 60000
  const warn = target > 0 && prefs.warn_minutes > 0 && elapsed >= warnAt
  const over = target > 0 && elapsed >= target
  useEffect(() => { if (warn && !over) navigator.vibrate?.([120, 80, 120]) }, [warn, over])
  useEffect(() => { if (over) navigator.vibrate?.([300]) }, [over])

  if (err) return <div className="pres"><div className="pres-ended"><Icon name="smartphone" size={30} /><h2>{err}</h2>{!remote && <Link to="/" className="btn">All decks</Link>}</div></div>
  if (!p || !look) return <div className="pres"><Loading /></div>
  const img = remote ? keyImage('remote', key!) : ownImage
  const vis = shown(p.slides)
  const notesPx = size || prefs.notes_size || 20
  const setNotes = (v: number) => { const n = Math.max(14, Math.min(44, v)); setSize(n); localStorage.setItem('psld-notes-size', String(n)) }

  if (!state) return <div className="pres"><Loading /></div>
  if (!state.live) {
    return (
      <div className="pres">
        <div className="pres-ended">
          <Icon name={ended ? 'check-circle' : 'presentation'} size={34} />
          <h2>{ended ? 'That was it' : p.deck.title}</h2>
          {ended ? <p className="muted">{ended.kind === 'rehearsal' ? 'The rehearsal is over; the time of every slide is kept.' : 'The talk is over.'}</p> : <p className="muted">Not being presented right now.</p>}
          {!remote && <div className="row wrap" style={{ justifyContent: 'center' }}>
            <button type="button" className="pres-btn next" style={{ flex: 'none', padding: '0 22px' }} onClick={() => send({ action: 'start', kind: 'talk' })}><Icon name="play" size={18} />Start the talk</button>
            <button type="button" className="pres-btn" style={{ flex: 'none', padding: '0 22px' }} onClick={() => send({ action: 'start', kind: 'rehearsal' })}><Icon name="timer" size={18} />Rehearse</button>
          </div>}
          {!remote && <button type="button" className="pres-ghost" onClick={() => navigate(`/d/${p.deck.id}/practice`)}>See the timings<Icon name="chevron-right" size={15} /></button>}
          {!remote && <p className="muted small">Then open the slides on the big screen: {origin().replace(/^https?:\/\//, '')}/present/{p.deck.id}</p>}
        </div>
      </div>
    )
  }
  const pos = Math.min(state.pos, vis.length - 1)
  const s = vis[pos]
  const nx = vis[pos + 1]
  const steps = s ? stepsOf(s) : 0
  const slideFor = (x: typeof s, i: number, step?: number) => x && <SlideView slide={x} deck={p.deck} look={look} imageUrl={img} index={i} total={vis.length} step={step} agenda={agendaOf(p.slides)}
    tally={x.layout === 'poll' ? state.tally[String(x.id)] || {} : undefined} showAnswer={step === undefined || step >= 1} qr={{ share: abs(p.share_url), live: origin() + '/w/' + p.deck.live_key }} thumb />
  const slideMs = state.slide_at ? now - Date.parse(state.slide_at) : 0
  const qs = state.questions.filter(q => !q.hidden)
  const mark = (q: Question, patch: Record<string, boolean>) => api.put(remote ? `${base}/questions/${q.id}` : `${V1}/questions/${q.id}`, patch).catch(e => toast(errMsg(e), 'err'))
  return (
    <div className="pres" style={{ ['--accent' as string]: '#f08a4b' }}>
      <div className="pres-top">
        <div>
          <div className={`pres-clock ${over ? 'over' : warn ? 'warn' : ''}`}>{fmtMs(elapsed)}</div>
          <div className="pres-sub">{target ? (over ? `${fmtMs(elapsed - target)} over` : `${fmtMs(target - elapsed)} left`) : new Date(now).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · this slide {fmtMs(slideMs)}</div>
        </div>
        <span className="grow" />
        <div style={{ textAlign: 'right' }}>
          <div className="pres-sub">{state.kind === 'rehearsal' ? 'Rehearsal' : `${state.viewers} following`}{!online && ' · reconnecting…'}</div>
          <div style={{ fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>{pos + 1} / {vis.length}</div>
        </div>
        <button type="button" className="pres-ghost" aria-label="All slides" onClick={() => setList(true)}><Icon name="grid" size={18} /></button>
        <button type="button" className="pres-ghost" aria-label="End" onClick={() => { if (window.confirm(state.kind === 'rehearsal' ? 'End the rehearsal?' : 'End the talk?')) send({ action: 'end' }) }}><Icon name="stop" size={18} /></button>
      </div>
      <div className="pres-body" {...touch}>
        <div className="pres-now">
          <div className="pres-a">
            <div className="pres-label">Now{steps > 0 && <span className="pres-step">{Array.from({ length: steps }, (_, i) => <i key={i} className={i < state.step ? 'on' : ''} />)}</span>}{state.black && <span style={{ color: '#fbbf24' }}>· black screen</span>}</div>
            {slideFor(s, pos, state.step)}
          </div>
          <div className="pres-c">
            <div className="pres-label" style={{ marginTop: 4 }}>
              Notes
              <span className="grow" />
              <button type="button" className="pres-ghost" aria-label="Smaller notes" onClick={() => setNotes(notesPx - 2)}>A−</button>
              <button type="button" className="pres-ghost" aria-label="Bigger notes" onClick={() => setNotes(notesPx + 2)}>A+</button>
            </div>
            <div className={`pres-notes ${s?.notes.trim() ? '' : 'empty'}`} style={{ fontSize: notesPx }}>{s?.notes.trim() || 'No notes on this slide.'}</div>
          </div>
        </div>
        <div className="pres-side">
          <div className="pres-next-wrap">
            <div className="pres-label">Next</div>
            {nx ? slideFor(nx, pos + 1) : <div className="pres-notes empty">The end — the next press does nothing.</div>}
            {nx && <div className="pres-sub" style={{ marginTop: 6 }}>{slideName(nx)}</div>}
          </div>
          {state.kind === 'talk' && <div className="pres-d">
            <div className="pres-label">Questions {qs.length > 0 && `· ${qs.filter(q => !q.answered).length} open`}{state.reactions > 0 && <span>· {state.reactions} reactions</span>}</div>
            <div className="pres-qs">
              {qs.length === 0 && <div className="pres-sub">None yet. People ask on their phones with code {p.deck.live_key.toUpperCase()}.</div>}
              {qs.map(q => (
                <div key={q.id} className="pres-q" style={{ opacity: q.answered ? .5 : 1 }}>
                  <span className="n">{q.votes > 0 ? `+${q.votes}` : ''}</span>
                  <span className="grow">{q.body}{q.name && <span className="pres-sub"> — {q.name}</span>}</span>
                  <button type="button" className="pres-ghost" aria-label={q.answered ? 'Not answered' : 'Answered'} onClick={() => mark(q, { answered: !q.answered })}><Icon name={q.answered ? 'rotate-ccw' : 'check'} size={15} /></button>
                  <button type="button" className="pres-ghost" aria-label="Hide" onClick={() => mark(q, { hidden: true })}><Icon name="eye-off" size={15} /></button>
                </div>
              ))}
            </div>
          </div>}
        </div>
      </div>
      <div className="pres-bottom">
        <button type="button" className="pres-btn" onClick={prev} disabled={pos === 0 && state.step === 0} aria-label="Back"><Icon name="chevron-left" size={24} /></button>
        <button type="button" className={`pres-btn ${state.black ? 'on' : ''}`} onClick={() => send({ action: 'black' })} aria-label="Black screen" title="Black screen"><Icon name="square" size={20} /></button>
        <button type="button" className="pres-btn next" onClick={next} disabled={!nx && state.step >= steps}>{state.step < steps ? 'Next point' : nx ? 'Next' : 'The end'}<Icon name="chevron-right" size={24} /></button>
      </div>
      {list && <div className="pres-list" onClick={() => setList(false)}>
        <div className="pres-list-panel" onClick={e => e.stopPropagation()}>
          {vis.map((x, i) => <button key={x.id} type="button" className={i === pos ? 'on' : ''} onClick={() => { send({ action: 'go', pos: i }); setList(false) }}>{slideFor(x, i)}<span>{i + 1}. {slideName(x)}</span></button>)}
        </div>
      </div>}
    </div>
  )
}
