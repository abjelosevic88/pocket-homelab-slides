// Left column: a new deck, the sections, the talks coming up and the
// decks changed last.
import { useCallback, useEffect, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import type { Home } from '../lib/types'
import { talkWhen } from '../lib/deck'
import { Icon } from './icons'

export function Sidebar({ onCreate, onNavigate }: { onCreate: () => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [home, setHome] = useState<Home | null>(null)
  const load = useCallback(() => { api.get<Home>(`${V1}/home`).then(setHome).catch(() => {}) }, [])
  useEffect(() => { load() }, [load, loc.pathname])
  useOnChange(load)

  const title = settings?.title || status?.title || 'Pocket Slides'
  const path = loc.pathname
  const archived = new URLSearchParams(loc.search).get('archived') === '1'
  const nav: { to: string; label: string; icon: string; active: boolean; count?: number; hint?: string; strong?: boolean }[] = [
    { to: '/', label: 'All decks', icon: 'presentation', active: path === '/' && !archived, count: home?.live.length || undefined, hint: home?.live.length ? 'Being presented now' : undefined, strong: true },
    { to: '/talks', label: 'Talks', icon: 'play', active: path.startsWith('/talks'), hint: 'Talks and rehearsals: timings, questions, polls' },
    { to: '/library', label: 'Layouts and themes', icon: 'palette', active: path.startsWith('/library'), hint: 'Your own slide layouts, colours, fonts and logo' },
    { to: '/?archived=1', label: 'Archived', icon: 'archive', active: path === '/' && archived, count: home?.archived || undefined },
  ]
  const recent = (home?.decks || []).slice(0, 6)
  return (
    <aside className="sidebar" aria-label="Pocket Slides">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Slides' ? title : <>Pocket <b>Slides</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" onClick={onCreate} title="New deck (N)">
        <Icon name="plus" size={15} strokeWidth={2.2} />New deck
      </button>

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} title={n.hint} className={() => `sb-nav-item ${n.active ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className={`sb-count ${n.strong ? 'strong' : ''}`} title={n.hint}>{n.strong ? 'live' : n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {!!home?.upcoming.length && <section className="sb-sec">
        <h4 className="sb-h">Coming up</h4>
        {home.upcoming.slice(0, 4).map(d => (
          <button key={d.id} type="button" className={`sb-row sb-list ${path.startsWith(`/d/${d.id}`) ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/d/${d.id}/present`) }} title={d.talk_title || d.title}>
            <Icon name="calendar" size={15} className="sb-list-ico" />
            <span className="grow ellipsis">{d.talk_title || d.title}</span>
            <span className="muted small nowrap">{talkWhen(d.talk_at, true)}</span>
          </button>
        ))}
      </section>}

      <section className="sb-sec">
        <h4 className="sb-h"><Link to="/" onClick={onNavigate} className="sb-h-link">Recent</Link></h4>
        {recent.map(d => (
          <button key={d.id} type="button" className={`sb-row sb-list ${path.startsWith(`/d/${d.id}`) ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/d/${d.id}`) }} title={d.title}>
            <Icon name={d.live ? 'play' : 'layers'} size={15} className="sb-list-ico" />
            <span className="grow ellipsis">{d.title}</span>
            {d.live ? <span className="sb-count strong">live</span> : <span className="muted small nowrap">{d.slides}</span>}
          </button>
        ))}
        {home && recent.length === 0 && <button type="button" className="sb-row sb-list" onClick={() => { onNavigate(); onCreate() }}><Icon name="plus" size={15} className="sb-list-ico" /><span className="grow">Make your first deck</span></button>}
      </section>

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('psld:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
