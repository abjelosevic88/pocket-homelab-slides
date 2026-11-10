// Every talk and rehearsal, and one of them in full: the time per slide,
// the poll results, the questions and what the audience thought.
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { DeckPayload, Feedback, Question, Show, Slide, Tally } from '../lib/types'
import { fmtMs, lookOf } from '../lib/slides'
import { ownImage, shown, talkWhen } from '../lib/deck'
import { fmtDateTime } from '../lib/format'
import { Card, Confirm, Empty, Loading, Segmented, errMsg, useAsync, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { SlideView } from '../components/SlideView'
import { TimingCard } from './DeckPage'
import { useCrumb } from '../components/Layout'

export default function Talks() {
  const [kind, setKind] = useState<'all' | 'talk' | 'rehearsal'>('all')
  const { data: shows } = useAsync(() => api.get<{ shows: Show[] }>(`${V1}/shows?limit=300`).then(r => r.shows), [])
  const { data: home } = useAsync(() => api.get<{ upcoming: DeckPayload['deck'][] }>(`${V1}/home`), [])
  const list = (shows || []).filter(h => kind === 'all' || h.kind === kind)
  const talks = (shows || []).filter(h => h.kind === 'talk')
  const people = talks.reduce((a, h) => a + h.viewers, 0)
  return (
    <div className="page">
      <div className="page-head"><div className="grow"><h2 className="page-title">Talks</h2></div>
        <Segmented size="sm" value={kind} onChange={setKind} options={[{ value: 'all', label: 'All' }, { value: 'talk', label: 'Talks' }, { value: 'rehearsal', label: 'Rehearsals' }]} /></div>
      {!!shows?.length && <div className="stats-row">
        <div className="stat"><span className="stat-l">Talks</span><span className="stat-v">{talks.length}</span></div>
        <div className="stat"><span className="stat-l">Rehearsals</span><span className="stat-v">{shows.length - talks.length}</span></div>
        <div className="stat"><span className="stat-l">People followed</span><span className="stat-v">{people}</span></div>
        <div className="stat"><span className="stat-l">Time on stage</span><span className="stat-v">{fmtMs(talks.reduce((a, h) => a + h.total_ms, 0))}</span></div>
      </div>}
      {!!home?.upcoming.length && <Card title="Coming up" flush><div className="rows">{home.upcoming.map(d => (
        <Link key={d.id} to={`/d/${d.id}/present`} className="row"><Icon name="calendar" size={16} /><span className="grow"><b>{d.talk_title || d.title}</b><span className="muted small"> · {talkWhen(d.talk_at)}{d.talk_place && ` · ${d.talk_place}`}</span></span><span className="muted small">{d.rehearsals ? `${d.rehearsals} rehearsals` : 'not rehearsed'}</span></Link>
      ))}</div></Card>}
      {!shows ? <Loading /> : list.length === 0 ? <Empty title="No talks yet" icon="play">Present or rehearse a deck: the time of each slide, the questions and the poll results are kept here.</Empty> : (
        <Card flush><div className="rows">{list.map(h => (
          <Link key={h.id} to={`/d/${h.deck_id}/shows/${h.id}`} className="row">
            <Icon name={h.kind === 'talk' ? 'presentation' : 'timer'} size={16} />
            <span className="grow"><b>{h.deck_title}</b><span className="muted small"> · {h.kind === 'talk' ? 'talk' : 'rehearsal'} · {fmtDateTime(h.started_at)}</span></span>
            {h.kind === 'talk' && <span className="muted small nowrap hide-sm">{h.viewers} following · {h.questions} questions{h.rating ? ` · ${h.rating.toFixed(1)}★` : ''}</span>}
            <span className="small nowrap tnum">{h.ended_at ? fmtMs(h.total_ms) : 'running'}</span>
          </Link>
        ))}</div></Card>
      )}
    </div>
  )
}

export function ShowPage() {
  const { id, sid } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [data, setData] = useState<{ show: Show; questions: Question[]; feedback: Feedback[]; tally: Tally; slides: Slide[] } | null>(null)
  const [deck, setDeck] = useState<DeckPayload | null>(null)
  const [note, setNote] = useState('')
  const [del, setDel] = useState(false)
  useEffect(() => {
    api.get<typeof data>(`${V1}/shows/${sid}`).then(r => { setData(r); setNote(r!.show.note) }).catch(e => toast(errMsg(e), 'err'))
    api.get<DeckPayload>(`${V1}/decks/${id}`).then(setDeck).catch(() => {})
  }, [id, sid]) // eslint-disable-line react-hooks/exhaustive-deps
  useCrumb(data ? `${data.show.kind === 'talk' ? 'Talk' : 'Rehearsal'}, ${fmtDateTime(data.show.started_at)}` : undefined, '/talks')
  if (!data || !deck) return <Loading />
  const h = data.show
  const vis = shown(data.slides)
  const look = lookOf(deck.deck, deck.theme)
  const polls = vis.filter(s => s.layout === 'poll' && data.tally[String(s.id)])
  const mark = async (q: Question, patch: Partial<Question>) => {
    const r = await api.put<{ question: Question }>(`${V1}/questions/${q.id}`, patch)
    setData(d => d && ({ ...d, questions: d.questions.map(x => x.id === q.id ? r.question : x) }))
  }
  const avg = data.feedback.length ? data.feedback.reduce((a, f) => a + f.rating, 0) / data.feedback.length : 0
  return (
    <div className="page narrow">
      <div className="page-head">
        <div className="grow"><h2 className="page-title">{deck.deck.title}</h2><p className="muted small">{h.kind === 'talk' ? 'Talk' : 'Rehearsal'} · {fmtDateTime(h.started_at)} · {h.ended_at ? fmtMs(h.total_ms) : 'still running'}</p></div>
        <Link to={`/d/${deck.deck.id}`} className="btn"><Icon name="layers" size={15} />The deck</Link>
        <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>
      </div>
      {h.kind === 'talk' && <div className="stats-row">
        <div className="stat"><span className="stat-l">Following</span><span className="stat-v">{h.viewers}</span></div>
        <div className="stat"><span className="stat-l">Reactions</span><span className="stat-v">{h.reactions}</span></div>
        <div className="stat"><span className="stat-l">Questions</span><span className="stat-v">{data.questions.length}</span></div>
        <div className="stat"><span className="stat-l">How it was</span><span className="stat-v">{avg ? <><span className="stars">★</span> {avg.toFixed(1)}</> : '—'}</span></div>
      </div>}
      <TimingCard show={h} slides={vis} target={deck.deck.target_minutes} />
      {polls.length > 0 && <Card title="Polls"><div className="pad stack">{polls.map(s => (
        <SlideView key={s.id} slide={s} deck={deck.deck} look={look} imageUrl={ownImage} tally={data.tally[String(s.id)]} showAnswer thumb />
      ))}</div></Card>}
      {data.questions.length > 0 && <Card title="Questions" flush><div>{data.questions.map(q => (
        <div key={q.id} className={`q-row ${q.answered ? 'done' : ''}`}>
          <span className="q-votes">{q.votes}</span>
          <span className="grow"><span className="q-body">{q.body}</span>{q.name && <span className="muted small"> — {q.name}</span>}{q.hidden && <span className="badge">hidden</span>}</span>
          <button type="button" className="btn sm ghost" onClick={() => mark(q, { answered: !q.answered })}>{q.answered ? 'Not answered' : 'Answered'}</button>
        </div>
      ))}</div></Card>}
      {data.feedback.length > 0 && <Card title="What they thought" flush><div className="rows">{data.feedback.map(f => (
        <div key={f.id} className="row"><span className="stars">{'★'.repeat(f.rating)}</span><span className="grow">{f.comment || <span className="muted">—</span>}{f.name && <span className="muted small"> — {f.name}</span>}</span></div>
      ))}</div></Card>}
      <Card title="How it went" sub="A note for next time.">
        <div className="pad stack">
          <textarea rows={3} value={note} onChange={e => setNote(e.target.value)} placeholder="Too fast in the middle; the poll worked well" />
          <div className="row"><button type="button" className="btn" disabled={note === h.note} onClick={async () => { await api.put(`${V1}/shows/${h.id}`, { note }); setData(d => d && ({ ...d, show: { ...d.show, note } })); toast('Saved') }}>Save</button></div>
        </div>
      </Card>
      {del && <Confirm title="Delete this show?" message="Its timings, questions, poll answers and feedback go." confirmLabel="Delete" onCancel={() => setDel(false)}
        onConfirm={async () => { await api.del(`${V1}/shows/${h.id}`); navigate(`/d/${deck.deck.id}/practice`) }} />}
    </div>
  )
}
