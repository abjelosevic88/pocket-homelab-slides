// Following a talk on your phone: the slide on the screen (and the ones
// before it), the poll to answer, questions to ask, a few reactions — and
// afterwards, how it was. No account needed.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Deck, LiveState, Question, Slide, Theme } from '../lib/types'
import { lookOf, voterId } from '../lib/slides'
import { keyImage, useLive } from '../lib/deck'
import { Loading, errMsg, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { SlideView } from '../components/SlideView'

type PublicDeck = Pick<Deck, 'title' | 'about' | 'theme' | 'aspect' | 'footer' | 'numbers' | 'transition' | 'talk_title' | 'talk_at' | 'talk_place' | 'questions' | 'live_key' | 'share_key'>
interface View { deck: PublicDeck; state: LiveState; seen: Slide[]; theme?: Theme; share_key?: string; mine?: Record<string, number> }

export function FollowCode() {
  const navigate = useNavigate()
  const [code, setCode] = useState('')
  return (
    <div className="aud">
      <div className="aud-top"><img src="/favicon.svg" alt="" /><b>Follow a talk</b></div>
      <div className="aud-main" style={{ maxWidth: 420, paddingTop: 40 }}>
        <p className="muted">Type the code from the screen.</p>
        <form className="aud-code" onSubmit={e => { e.preventDefault(); if (code.trim()) navigate(`/w/${code.trim().toLowerCase()}`) }}>
          <input autoFocus value={code} onChange={e => setCode(e.target.value.replace(/[^a-z0-9]/gi, '').slice(0, 12))} placeholder="ABC123" aria-label="Code" autoCapitalize="characters" />
          <button type="submit" className="btn primary lg" disabled={code.length < 4}>Go</button>
        </form>
      </div>
    </div>
  )
}

export default function Audience() {
  const { key = '' } = useParams()
  const { toast } = useToast()
  const voter = useMemo(voterId, [])
  const [v, setV] = useState<View | null>(null)
  const [err, setErr] = useState('')
  const [seen, setSeen] = useState<Slide[]>([])
  const [mine, setMine] = useState<Record<string, number>>({})
  const [ask, setAsk] = useState('')
  const [name, setName] = useState(() => localStorage.getItem('psld-name') || '')
  const [liked, setLiked] = useState<number[]>([])
  const [earlier, setEarlier] = useState(false)
  const base = `${V1}/public/live/${encodeURIComponent(key)}`
  const load = useCallback(() => api.get<View>(`${base}?voter=${voter}`).then(r => { setV(r); setSeen(r.seen); setMine(r.mine || {}) }).catch(e => setErr(errMsg(e))), [base, voter])
  useEffect(() => { load() }, [load])
  const onEvent = useCallback((e: LiveState) => {
    if (e.type === 'state' && e.live && e.slide) {
      const sl = e.slide
      setSeen(list => {
        const i = list.findIndex(x => x.id === sl.id)
        if (i >= 0) { const c = [...list]; c[i] = sl; return c }
        return e.pos >= list.length ? [...list, sl] : list
      })
    }
    if (e.type === 'state' && e.live && seen.length === 0) load()
  }, [load, seen.length])
  const { state } = useLive(v ? `${base}/stream` : null, onEvent)
  const st = state || v?.state
  const look = useMemo(() => v ? lookOf(v.deck, v.theme) : null, [v])
  useEffect(() => { document.title = v ? v.deck.title : 'Follow a talk' }, [v])
  if (err) return <div className="aud"><div className="aud-top"><img src="/favicon.svg" alt="" /><b>Pocket Slides</b></div><div className="aud-main"><h2>{err}</h2><Link to="/w" className="btn">Type another code</Link></div></div>
  if (!v || !look || !st) return <div className="aud"><Loading /></div>
  const img = keyImage('live', key)
  const cur = st.live ? st.slide : undefined
  const tally = cur ? st.tally[String(cur.id)] : undefined
  const voted = cur ? mine[String(cur.id)] : undefined
  const vote = async (choice: number) => {
    if (!cur) return
    setMine(m => ({ ...m, [String(cur.id)]: choice }))
    try { await api.post(`${base}/vote`, { slide_id: cur.id, choice, voter }) } catch (e) { toast(errMsg(e), 'err') }
  }
  const send = async () => {
    if (!ask.trim()) return
    localStorage.setItem('psld-name', name)
    try { await api.post(`${base}/questions`, { body: ask, name, voter }); setAsk(''); toast('Asked — the speaker sees it') } catch (e) { toast(errMsg(e), 'err') }
  }
  const like = async (q: Question) => { if (liked.includes(q.id)) return; setLiked(l => [...l, q.id]); await api.post(`${base}/questions/${q.id}/like`, { voter }).catch(() => {}) }
  const react = (emoji: string) => api.post(`${base}/react`, { emoji, voter }).catch(() => {})
  const ago = seen.filter(s => !cur || s.id !== cur.id)
  return (
    <div className="aud">
      <div className="aud-top"><img src="/favicon.svg" alt="" /><b className="grow ellipsis">{v.deck.talk_title || v.deck.title}</b>{st.live && <span className="live-pill">Live</span>}</div>
      <div className="aud-main">
        {!st.live ? (
          st.feedback ? <Feedback base={base} voter={voter} title={v.deck.title} shareKey={v.share_key} /> : (
            <div className="empty"><Icon name="presentation" size={28} /><h3>The talk has not started</h3><p className="muted">Keep this page open: the slides come up here as soon as it does.</p>
              {v.share_key && <Link className="btn" to={`/s/${v.share_key}`}>See the slides</Link>}</div>
          )
        ) : <>
          {cur && !st.black ? <SlideView slide={cur} deck={v.deck} look={look} imageUrl={img} index={st.pos} total={st.total} step={st.step} tally={cur.layout === 'poll' && voted !== undefined ? tally || {} : undefined} showAnswer={st.step >= 1} qr={{}} />
            : <div className="sv-box" style={{ aspectRatio: v.deck.aspect === '4:3' ? '4/3' : '16/9', display: 'grid', placeItems: 'center', color: '#888' }}>…</div>}
          {cur?.layout === 'poll' && !!cur.data.options?.length && (
            <div className="aud-vote">
              <b>{voted === undefined ? 'Your answer' : 'You answered — change it if you like'}</b>
              {cur.data.options.map((o, i) => <button key={i} type="button" className={voted === i ? 'on' : ''} onClick={() => vote(i)}><span className="l">{String.fromCharCode(65 + i)}</span>{o}</button>)}
            </div>
          )}
          <div className="aud-react">{['👏', '❤️', '😂', '🤯', '👍', '🎉'].map(e => <button key={e} type="button" onClick={() => react(e)} aria-label={`React ${e}`}>{e}</button>)}</div>
          {st.asking && v.deck.questions && <div className="card"><div className="pad stack" style={{ paddingTop: 14 }}>
            <b>Ask a question</b>
            <textarea rows={2} value={ask} onChange={e => setAsk(e.target.value)} placeholder="The speaker sees it on their phone" maxLength={500} />
            <div className="row"><input value={name} onChange={e => setName(e.target.value)} placeholder="Your name (or leave it empty)" maxLength={60} /><button type="button" className="btn primary" onClick={send} disabled={!ask.trim()}>Ask</button></div>
            {st.questions.length > 0 && <div className="rows" style={{ margin: '0 -18px' }}>{st.questions.map(q => (
              <div key={q.id} className="row">
                <button type="button" className={`chip ${liked.includes(q.id) ? 'on' : ''}`} onClick={() => like(q)} aria-label="Me too"><Icon name="thumbs-up" size={13} />{q.votes}</button>
                <span className="grow" style={{ textDecoration: q.answered ? 'line-through' : undefined, color: q.answered ? 'var(--muted)' : undefined }}>{q.body}{q.name && <span className="muted small"> — {q.name}</span>}</span>
              </div>
            ))}</div>}
          </div></div>}
          {ago.length > 0 && <div className="stack">
            <button type="button" className="btn ghost" onClick={() => setEarlier(x => !x)}><Icon name={earlier ? 'chevron-up' : 'chevron-down'} size={15} />The slides so far ({ago.length})</button>
            {earlier && <div className="seen-list">{[...ago].reverse().map(s => <SlideView key={s.id} slide={s} deck={v.deck} look={look} imageUrl={img} thumb qr={{}} />)}</div>}
          </div>}
        </>}
      </div>
    </div>
  )
}

function Feedback({ base, voter, title, shareKey }: { base: string; voter: string; title: string; shareKey?: string }) {
  const { toast } = useToast()
  const [stars, setStars] = useState(0)
  const [text, setText] = useState('')
  const [done, setDone] = useState(false)
  const send = async () => { try { await api.post(`${base}/feedback`, { rating: stars, comment: text, name: localStorage.getItem('psld-name') || '', voter }); setDone(true) } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <div className="card"><div className="pad stack" style={{ paddingTop: 16 }}>
      <h3 style={{ margin: 0 }}>{done ? 'Thank you!' : `How was “${title}”?`}</h3>
      {!done && <>
        <div className="star-pick">{[1, 2, 3, 4, 5].map(n => <button key={n} type="button" className={n <= stars ? 'on' : ''} onClick={() => setStars(n)} aria-label={`${n} stars`}>★</button>)}</div>
        <textarea rows={3} value={text} onChange={e => setText(e.target.value)} placeholder="What did you like? What could be better?" maxLength={2000} />
        <div className="row"><button type="button" className="btn primary" onClick={send} disabled={!stars}>Send</button></div>
      </>}
      {shareKey && <Link to={`/s/${shareKey}`} className="btn">See the slides again</Link>}
    </div></div>
  )
}
