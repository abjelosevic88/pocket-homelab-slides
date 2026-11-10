// The start page: what is live, the next talk, every deck as a thumbnail.
import { useCallback, useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useOnChange } from '../lib/data'
import type { Deck, Home as HomeT } from '../lib/types'
import { lookOf } from '../lib/slides'
import { ownImage, talkWhen } from '../lib/deck'
import { ago, fmtDateTime } from '../lib/format'
import { Card, Empty, Loading, useIsMobile } from '../components/ui'
import { Icon } from '../components/icons'
import { SlideView } from '../components/SlideView'
import { NewDeck } from '../components/NewDeck'
import { useCrumb } from '../components/Layout'
import { fmtMs } from '../lib/slides'

export default function Home() {
  const loc = useLocation()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const q = new URLSearchParams(loc.search)
  const archived = q.get('archived') === '1'
  const newParam = q.get('new')
  const [home, setHome] = useState<HomeT | null>(null)
  const [arch, setArch] = useState<Deck[] | null>(null)
  const load = useCallback(() => {
    api.get<HomeT>(`${V1}/home`).then(setHome).catch(() => {})
    if (archived) api.get<{ decks: Deck[] }>(`${V1}/decks?archived=1`).then(r => setArch(r.decks)).catch(() => {})
  }, [archived])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  useCrumb(archived ? 'Archived' : undefined)
  const closeNew = () => { q.delete('new'); navigate({ search: q.toString() }, { replace: true }) }
  const decks = archived ? arch : home?.decks
  const next = home?.upcoming[0]
  return (
    <div className="page">
      {!archived && !!home?.live.length && home.live.map(d => (
        <Link key={d.id} to={`/d/${d.id}/present`} className="next-talk live">
          <span className="live-pill">Live</span>
          <div className="grow"><h3>{d.title}</h3><span className="muted small">Being presented now — open the controls</span></div>
          <Icon name="chevron-right" size={18} />
        </Link>
      ))}
      {!archived && next && (
        <div className="next-talk">
          <Icon name="calendar" size={22} />
          <div className="grow">
            <div className="when">{talkWhen(next.talk_at)}{next.talk_place && ` · ${next.talk_place}`}</div>
            <h3>{next.talk_title || next.title}</h3>
            <span className="muted small">{next.rehearsals ? `Rehearsed ${next.rehearsals} ${next.rehearsals === 1 ? 'time' : 'times'}` : 'Not rehearsed yet'}{next.target_minutes ? ` · ${next.target_minutes} minutes` : ''}</span>
          </div>
          <div className="row wrap">
            {!mobile && <button type="button" className="btn" onClick={() => navigate(`/d/${next.id}`)}><Icon name="edit" size={15} />Edit</button>}
            <button type="button" className="btn primary" onClick={() => navigate(`/d/${next.id}/present`)}><Icon name="play" size={15} />Rehearse</button>
          </div>
        </div>
      )}
      <div className="page-head">
        <div className="grow"><h2 className="page-title">{archived ? 'Archived decks' : 'Decks'}</h2></div>
        {!mobile && !archived && <div className="row">
          <button type="button" className="btn" onClick={() => navigate('/?new=import')}><Icon name="upload" size={15} />Bring in</button>
          <button type="button" className="btn" onClick={() => navigate('/?new=ai')}><Icon name="sparkles" size={15} />Draft with AI</button>
          <button type="button" className="btn primary" onClick={() => navigate('/?new=1')}><Icon name="plus" size={15} />New deck</button>
        </div>}
      </div>
      {!decks ? <Loading /> : decks.length === 0 ? (
        archived ? <Empty title="Nothing archived" icon="archive">Decks you archive from their menu wait here.</Empty> : (
          <Empty title="Your first deck" icon="presentation">
            Start from a template, bring in a PowerPoint, or let AI draft one from a few words.
            <div className="row wrap" style={{ justifyContent: 'center', marginTop: 12 }}>
              <button type="button" className="btn primary" onClick={() => navigate('/?new=1')}><Icon name="plus" size={15} />New deck</button>
              <button type="button" className="btn" onClick={() => navigate('/?new=import')}><Icon name="upload" size={15} />Bring in</button>
            </div>
          </Empty>
        )
      ) : (
        <div className="deck-grid">{decks.map(d => <DeckCard key={d.id} d={d} onOpen={() => navigate(`/d/${d.id}`)} />)}</div>
      )}
      {!archived && !!home?.shows.length && (
        <Card title="Lately" actions={<Link to="/talks" className="small">All talks</Link>} flush>
          <div className="rows">{home.shows.slice(0, 5).map(h => (
            <Link key={h.id} to={`/d/${h.deck_id}/shows/${h.id}`} className="row">
              <Icon name={h.kind === 'talk' ? 'presentation' : 'timer'} size={16} />
              <span className="grow ellipsis">{h.kind === 'talk' ? 'Talk' : 'Rehearsal'}: {h.deck_title}</span>
              <span className="muted small nowrap tnum">{h.ended_at ? fmtMs(h.total_ms) : <span className="live-pill">Live</span>}</span>
              <span className="muted small nowrap" title={fmtDateTime(h.started_at)}>{ago(h.started_at)}</span>
            </Link>
          ))}</div>
        </Card>
      )}
      {newParam && <NewDeck start={newParam} onClose={closeNew} />}
    </div>
  )
}

function DeckCard({ d, onOpen }: { d: Deck; onOpen: () => void }) {
  const look = lookOf(d, d.own_theme)
  return (
    <button type="button" className="deck-card" onClick={onOpen}>
      <div className="deck-thumb">
        <SlideView slide={d.cover || null} deck={d} look={look} imageUrl={ownImage} thumb index={0} total={d.slides} />
        <div className="deck-badges">
          {d.live && <span className="live-pill">Live</span>}
          {d.shared && <span className="soft-pill">Shared</span>}
          {d.template && <span className="soft-pill">Template</span>}
          {d.pinned && <span className="soft-pill"><Icon name="pin" size={10} /></span>}
        </div>
      </div>
      <div className="deck-card-meta">
        <div className="grow">
          <div className="deck-card-title">{d.title}</div>
          <div className="deck-card-sub">{d.slides} {d.slides === 1 ? 'slide' : 'slides'} · {d.talk_at && d.talk_at >= new Date().toISOString().slice(0, 10) ? talkWhen(d.talk_at) : `changed ${ago(d.updated_at)}`}</div>
        </div>
      </div>
    </button>
  )
}
