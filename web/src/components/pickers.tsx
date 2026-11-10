// Pick something from another Pocket app: a note, a file, a memo, photos,
// a table, a calendar event, people, a chat. Each is a small sheet with a
// search box; when the app is not connected, it says how to connect it.
import React, { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtDate } from '../lib/format'
import { Empty, Loading, Modal, useDebounce, useToast, errMsg } from './ui'
import { Icon } from './icons'

function useList<T>(url: string | null, q = '') {
  const [rows, setRows] = useState<T[] | null>(null)
  const [err, setErr] = useState('')
  const dq = useDebounce(q, 250)
  useEffect(() => {
    if (!url) return
    let live = true
    setErr('')
    const u = url + (url.includes('?') ? '&' : '?') + (dq ? `q=${encodeURIComponent(dq)}` : '')
    api.get<Record<string, T[]>>(u).then(r => { if (live) setRows((Object.values(r).find(v => Array.isArray(v)) as T[]) || []) }).catch(e => { if (live) { setErr(errMsg(e)); setRows([]) } })
    return () => { live = false }
  }, [url, dq])
  return { rows, err }
}

function NotConnected({ err }: { err: string }) {
  return <div className="callout info"><Icon name="info" size={15} /><div>{err} <Link to="/settings/apps">Connected apps</Link></div></div>
}

function Search({ q, setQ, placeholder }: { q: string; setQ: (v: string) => void; placeholder: string }) {
  return <div className="search-box"><Icon name="search" size={15} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder={placeholder} aria-label="Search" /></div>
}

interface Note { id: string; title: string; folder: string; snippet: string }
export function NotePicker({ onPick, onClose, title = 'Pick a note' }: { onPick: (n: Note) => Promise<void> | void; onClose: () => void; title?: string }) {
  const [q, setQ] = useState('')
  const [busy, setBusy] = useState('')
  const { rows, err } = useList<Note>(`${V1}/apps/notes`, q)
  return (
    <Modal title={title} onClose={onClose}>
      <div className="stack">
        <Search q={q} setQ={setQ} placeholder="Search your notes" />
        {err && <NotConnected err={err} />}
        {!rows ? <Loading /> : !err && rows.length === 0 ? <Empty title="No notes found" /> : (
          <div className="pick-list">{rows.map(n => (
            <button key={n.id} type="button" className="pick-row" disabled={!!busy} onClick={async () => { setBusy(n.id); try { await onPick(n) } finally { setBusy('') } }}>
              <Icon name="note" size={16} /><span className="grow"><b>{n.title}</b>{n.folder && <span className="muted small"> · {n.folder}</span>}<span className="muted small block-line">{n.snippet}</span></span>
              {busy === n.id && <span className="spinner" />}
            </button>
          ))}</div>
        )}
      </div>
    </Modal>
  )
}

interface Memo { id: string; title: string; date: string; seconds: number; summary: string }
export function VoicePicker({ onPick, onClose }: { onPick: (m: Memo) => Promise<void> | void; onClose: () => void }) {
  const [q, setQ] = useState('')
  const [busy, setBusy] = useState('')
  const { rows, err } = useList<Memo>(`${V1}/apps/voice`, q)
  return (
    <Modal title="Pick a voice memo" onClose={onClose}>
      <div className="stack">
        <p className="muted small">Talk your talk through into Pocket Voice first; the memo becomes a first set of slides (with AI an outline, without one a slide per paragraph).</p>
        <Search q={q} setQ={setQ} placeholder="Search your memos" />
        {err && <NotConnected err={err} />}
        {!rows ? <Loading /> : !err && rows.length === 0 ? <Empty title="No memos found" /> : (
          <div className="pick-list">{rows.map(m => (
            <button key={m.id} type="button" className="pick-row" disabled={!!busy} onClick={async () => { setBusy(m.id); try { await onPick(m) } finally { setBusy('') } }}>
              <Icon name="mic" size={16} /><span className="grow"><b>{m.title}</b><span className="muted small"> · {m.date && fmtDate(m.date)} · {Math.round(m.seconds / 60)} min</span><span className="muted small block-line">{m.summary}</span></span>
              {busy === m.id && <span className="spinner" />}
            </button>
          ))}</div>
        )}
      </div>
    </Modal>
  )
}

interface FileEntry { name: string; path: string; kind: 'folder' | 'deck' | 'picture'; size: number }
export function FilePicker({ want, onPick, onClose, title }: { want: 'deck' | 'picture'; onPick: (space: string, path: string, name: string) => Promise<void> | void; onClose: () => void; title: string }) {
  const [space, setSpace] = useState('home')
  const [path, setPath] = useState('/')
  const [data, setData] = useState<{ entries: FileEntry[]; spaces: { id: string; name: string }[] } | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState('')
  useEffect(() => {
    let live = true
    setData(null)
    api.get<{ entries: FileEntry[]; spaces: { id: string; name: string }[] }>(`${V1}/apps/files?space=${encodeURIComponent(space)}&path=${encodeURIComponent(path)}`)
      .then(r => { if (live) { setData(r); setErr('') } }).catch(e => { if (live) { setErr(errMsg(e)); setData({ entries: [], spaces: [] }) } })
    return () => { live = false }
  }, [space, path])
  const up = path === '/' ? null : path.replace(/\/[^/]+\/?$/, '') || '/'
  const rows = (data?.entries || []).filter(e => e.kind === 'folder' || e.kind === want)
  return (
    <Modal title={title} onClose={onClose}>
      <div className="stack">
        {data && data.spaces.length > 1 && <div className="chips">{data.spaces.map(s => <button key={s.id} type="button" className={`chip ${s.id === space ? 'on' : ''}`} onClick={() => { setSpace(s.id); setPath('/') }}>{s.name}</button>)}</div>}
        <div className="row small muted"><Icon name="archive" size={14} /><span className="ellipsis">{path}</span></div>
        {err && <NotConnected err={err} />}
        {!data ? <Loading /> : (
          <div className="pick-list">
            {up !== null && <button type="button" className="pick-row" onClick={() => setPath(up)}><Icon name="arrow-left" size={16} /><span className="grow">Back</span></button>}
            {rows.length === 0 && !err && <Empty title={want === 'deck' ? 'No PowerPoint or Markdown files here' : 'No pictures here'} />}
            {rows.map(e => (
              <button key={e.path} type="button" className="pick-row" disabled={!!busy} onClick={async () => {
                if (e.kind === 'folder') { setPath(e.path); return }
                setBusy(e.path); try { await onPick(space, e.path, e.name) } finally { setBusy('') }
              }}>
                <Icon name={e.kind === 'folder' ? 'archive' : e.kind === 'picture' ? 'image' : 'presentation'} size={16} /><span className="grow ellipsis">{e.name}</span>
                {busy === e.path ? <span className="spinner" /> : e.kind === 'folder' && <Icon name="chevron-right" size={15} />}
              </button>
            ))}
          </div>
        )}
      </div>
    </Modal>
  )
}

interface Photo { id: string; name: string; date: string; thumb_url: string }
export function PhotoPicker({ multi, onPick, onClose }: { multi: boolean; onPick: (ids: string[]) => Promise<void> | void; onClose: () => void }) {
  const [q, setQ] = useState('')
  const [sel, setSel] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const { rows, err } = useList<Photo>(`${V1}/apps/photos`, q)
  const go = async (ids: string[]) => { setBusy(true); try { await onPick(ids) } finally { setBusy(false) } }
  return (
    <Modal title={multi ? 'Make slides from photos' : 'Pick a photo'} onClose={onClose} size="lg"
      footer={multi ? <><span className="muted small grow">{sel.length ? `${sel.length} picked — a full-picture slide each` : 'Pick a few'}</span><button type="button" className="btn primary" disabled={!sel.length || busy} onClick={() => go(sel)}>{busy && <span className="spinner" />}Add {sel.length || ''} {sel.length === 1 ? 'slide' : 'slides'}</button></> : undefined}>
      <div className="stack">
        <Search q={q} setQ={setQ} placeholder="Search your photos (“beach”, “Sarajevo”, “cake”)" />
        {err && <NotConnected err={err} />}
        {!rows ? <Loading /> : !err && rows.length === 0 ? <Empty title="No photos found" /> : (
          <div className="photo-grid">{rows.map(p => (
            <button key={p.id} type="button" className={`photo-cell ${sel.includes(p.id) ? 'on' : ''}`} disabled={busy} onClick={() => multi ? setSel(s => s.includes(p.id) ? s.filter(x => x !== p.id) : [...s, p.id].slice(0, 40)) : go([p.id])} title={p.name}>
              <img src={p.thumb_url} alt="" loading="lazy" />{sel.includes(p.id) && <span className="photo-check"><Icon name="check" size={14} /></span>}
            </button>
          ))}</div>
        )}
      </div>
    </Modal>
  )
}

interface TableRef { id: string; name: string; base: string }
export function TablePicker({ deckId, slideId, after, onDone, onClose }: { deckId: number; slideId?: number; after?: number; onDone: () => void; onClose: () => void }) {
  const { toast } = useToast()
  const { rows: tables, err } = useList<TableRef>(`${V1}/apps/tables`)
  const [pick, setPick] = useState<TableRef | null>(null)
  const [grid, setGrid] = useState<string[][] | null>(null)
  const [kind, setKind] = useState<'chart' | 'table'>('chart')
  const [chart, setChart] = useState('bar')
  const [label, setLabel] = useState(0)
  const [value, setValue] = useState(1)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!pick) return
    setGrid(null)
    api.get<{ rows: string[][] }>(`${V1}/apps/tables/${pick.id}`).then(r => {
      setGrid(r.rows)
      const head = r.rows[0] || []
      const num = head.findIndex((_, i) => i > 0 && r.rows.slice(1).some(row => /^-?[\d.,]+\s?[%€$]?$/.test((row[i] || '').trim())))
      setValue(num > 0 ? num : Math.min(1, head.length - 1))
    }).catch(e => toast(errMsg(e), 'err'))
  }, [pick]) // eslint-disable-line react-hooks/exhaustive-deps
  const make = async () => {
    if (!pick) return
    setBusy(true)
    try {
      const r = await api.post<{ text: string }>(`${V1}/apps/tables/slide`, { deck_id: deckId, slide_id: slideId, after, table_id: pick.id, name: pick.name, kind, chart, label_col: label, value_col: value })
      toast(r.text); onDone(); onClose()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const head = grid?.[0] || []
  return (
    <Modal title={pick ? pick.name : 'Pick a table'} onClose={onClose} size="lg"
      footer={pick && grid ? <button type="button" className="btn primary" onClick={make} disabled={busy}>{busy && <span className="spinner" />}{slideId ? 'Use on this slide' : 'Add the slide'}</button> : undefined}>
      {!pick ? <div className="stack">
        {err && <NotConnected err={err} />}
        {!tables ? <Loading /> : !err && tables.length === 0 ? <Empty title="No tables yet" /> : <div className="pick-list">{tables.map(t => (
          <button key={t.id} type="button" className="pick-row" onClick={() => setPick(t)}><Icon name="table" size={16} /><span className="grow"><b>{t.name}</b><span className="muted small"> · {t.base}</span></span><Icon name="chevron-right" size={15} /></button>
        ))}</div>}
      </div> : !grid ? <Loading /> : <div className="stack">
        <div className="seg-row">
          <button type="button" className={`chip ${kind === 'chart' ? 'on' : ''}`} onClick={() => setKind('chart')}><Icon name="chart" size={14} />A chart</button>
          <button type="button" className={`chip ${kind === 'table' ? 'on' : ''}`} onClick={() => setKind('table')}><Icon name="table" size={14} />A table</button>
        </div>
        {kind === 'chart' && <div className="two-col tight">
          <div className="field"><span className="field-label">Names from</span>{colPick(head, label, setLabel)}</div>
          <div className="field"><span className="field-label">Numbers from</span>{colPick(head, value, setValue)}</div>
          <div className="chips">{(['bar', 'hbar', 'line', 'pie', 'donut'] as const).map(k => <button key={k} type="button" className={`chip ${chart === k ? 'on' : ''}`} onClick={() => setChart(k)}>{({ bar: 'Bars', hbar: 'Bars across', line: 'Line', pie: 'Pie', donut: 'Ring' })[k]}</button>)}</div>
        </div>}
        <div className="scroll-x"><table className="mini-table"><tbody>{grid.slice(0, 8).map((r, i) => <tr key={i}>{r.slice(0, 8).map((c, j) => i === 0 ? <th key={j} className={kind === 'chart' && (j === label || j === value) ? 'on' : ''}>{c}</th> : <td key={j} className={kind === 'chart' && (j === label || j === value) ? 'on' : ''}>{c}</td>)}</tr>)}</tbody></table></div>
        <p className="muted small">{kind === 'chart' ? 'Up to 24 rows; rows without a number are left out.' : 'The first 15 rows and 6 columns.'}</p>
      </div>}
    </Modal>
  )
}

function colPick(head: string[], value: number, set: (n: number) => void) {
  return <div className="chips">{head.map((h, i) => <button key={i} type="button" className={`chip ${value === i ? 'on' : ''}`} onClick={() => set(i)}>{h || `Column ${i + 1}`}</button>)}</div>
}

interface Event { id: string; title: string; at: string; place: string }
export function CalendarPicker({ onPick, onClose }: { onPick: (e: Event) => void; onClose: () => void }) {
  const { rows, err } = useList<Event>(`${V1}/apps/calendar?days=120`)
  return (
    <Modal title="Pick the talk from your calendar" onClose={onClose}>
      <div className="stack">
        {err && <NotConnected err={err} />}
        {!rows ? <Loading /> : !err && rows.length === 0 ? <Empty title="Nothing on in the next months" /> : <div className="pick-list">{rows.map(e => (
          <button key={e.id + e.at} type="button" className="pick-row" onClick={() => onPick(e)}><Icon name="calendar" size={16} /><span className="grow"><b>{e.title}</b><span className="muted small block-line">{e.at.replace('T', ' ')}{e.place && ` · ${e.place}`}</span></span></button>
        ))}</div>}
      </div>
    </Modal>
  )
}

interface Person { id: string; name: string; emails: string[] }
export function EmailSheet({ deckId, onClose }: { deckId: number; onClose: () => void }) {
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const [to, setTo] = useState<{ name: string; addr: string }[]>([])
  const [typed, setTyped] = useState('')
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const { rows, err } = useList<Person>(`${V1}/apps/contacts`, q)
  const add = (name: string, addr: string) => setTo(t => t.some(x => x.addr === addr) ? t : [...t, { name, addr }])
  const send = async () => {
    const list = [...to]
    for (const a of typed.split(/[,;\s]+/)) if (a.includes('@')) list.push({ name: '', addr: a })
    setBusy(true)
    try { const r = await api.post<{ text: string }>(`${V1}/apps/email`, { deck_id: deckId, to: list, text }); toast(r.text); onClose() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Email the slides" onClose={onClose} footer={<button type="button" className="btn primary" onClick={send} disabled={busy || (!to.length && !typed.includes('@'))}>{busy && <span className="spinner" />}<Icon name="send" size={15} />Send</button>}>
      <div className="stack">
        <p className="muted small">From your own mail account (Pocket Email). The share link is switched on so they can open the slides (never your speaker notes).</p>
        {!!to.length && <div className="chips">{to.map(t => <span key={t.addr} className="chip on">{t.name || t.addr}<button type="button" className="chip-x" aria-label="Remove" onClick={() => setTo(x => x.filter(y => y.addr !== t.addr))}><Icon name="x" size={12} /></button></span>)}</div>}
        <input value={typed} onChange={e => setTyped(e.target.value)} placeholder="Type addresses, or pick people below" />
        <textarea rows={3} value={text} onChange={e => setText(e.target.value)} placeholder="Thank you for coming. Here are the slides." />
        <Search q={q} setQ={setQ} placeholder="Find people in Pocket Contacts" />
        {err && <NotConnected err={err} />}
        {rows && rows.length > 0 && <div className="pick-list short">{rows.map(p => p.emails.map(e => (
          <button key={p.id + e} type="button" className="pick-row" onClick={() => add(p.name, e)}><Icon name="user" size={16} /><span className="grow"><b>{p.name}</b><span className="muted small"> · {e}</span></span>{to.some(x => x.addr === e) && <Icon name="check" size={15} />}</button>
        )))}</div>}
      </div>
    </Modal>
  )
}

interface Conversation { id: string; name: string; kind: string; emoji: string }
export function ChatSheet({ deckId, what, onClose }: { deckId: number; what: 'share' | 'live'; onClose: () => void }) {
  const { toast } = useToast()
  const { rows, err } = useList<Conversation>(`${V1}/apps/chats`)
  const [busy, setBusy] = useState('')
  const send = async (to: string) => {
    setBusy(to)
    try { const r = await api.post<{ text: string }>(`${V1}/apps/chat`, { deck_id: deckId, to, what }); toast(r.text); onClose() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  return (
    <Modal title={what === 'live' ? 'Send the follow-along link to a chat' : 'Send the slides to a chat'} onClose={onClose}>
      <div className="stack">
        {err && <NotConnected err={err} />}
        {!rows ? <Loading /> : <div className="pick-list">
          <button type="button" className="pick-row" onClick={() => send('me')} disabled={!!busy}><Icon name="user" size={16} /><span className="grow">A note to myself</span>{busy === 'me' && <span className="spinner" />}</button>
          {rows.map(c => <button key={c.id} type="button" className="pick-row" onClick={() => send(c.id)} disabled={!!busy}><span className="pick-emoji">{c.emoji || '💬'}</span><span className="grow">{c.name || 'Chat'}</span>{busy === c.id && <span className="spinner" />}</button>)}
        </div>}
      </div>
    </Modal>
  )
}

export type { Note, Memo, Photo, Event as CalEvent }
