// Start a deck: from a template, drafted by AI, or brought in (PowerPoint,
// Markdown, a note, a file, a voice memo).
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, uploadForm, V1 } from '../lib/api'
import { useAppState } from '../lib/data'
import { announceChange } from '../lib/data'
import type { Builtins, Deck, Theme } from '../lib/types'
import { FONTS, THEMES, builtinLook } from '../lib/slides'
import { Field, Modal, Segmented, Tabs, errMsg, useAsync, useToast, CoreHint } from './ui'
import { Icon } from './icons'
import { FilePicker, NotePicker, VoicePicker } from './pickers'

let builtinsCache: Promise<Builtins> | null = null
export function useBuiltins() {
  return useAsync(() => (builtinsCache ||= api.get<Builtins>(`${V1}/builtins`)), [])
}

type Tab = 'start' | 'ai' | 'import'

export function NewDeck({ start, onClose }: { start?: string; onClose: () => void }) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const appState = useAppState()
  const [tab, setTab] = useState<Tab>(start === 'ai' ? 'ai' : start === 'import' ? 'import' : 'start')
  const { data: b } = useBuiltins()
  const { data: mine } = useAsync(() => api.get<{ decks: Deck[] }>(`${V1}/decks`).then(r => r.decks.filter(d => d.template)), [])
  const { data: themes } = useAsync(() => api.get<{ themes: Theme[] }>(`${V1}/themes`).then(r => r.themes), [])
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; detail: string }>(`${V1}/ai/status`).catch(() => ({ ready: false, detail: '' })), [])
  const [title, setTitle] = useState('')
  const [tpl, setTpl] = useState<string>(start && !['1', 'ai', 'import'].includes(start) ? start : 'talk')
  const [theme, setTheme] = useState<{ key: string; own?: number } | null>(null)
  const [busy, setBusy] = useState(false)
  const [topic, setTopic] = useState('')
  const [who, setWho] = useState('')
  const [minutes, setMinutes] = useState(15)
  const [points, setPoints] = useState('')
  const [md, setMd] = useState('')
  const [pick, setPick] = useState<'' | 'notes' | 'files' | 'voice'>('')
  const file = useRef<HTMLInputElement>(null)

  const go = (d: Deck, text?: string) => { announceChange(); if (text) toast(text); onClose(); navigate(`/d/${d.id}`) }
  const run = async (fn: () => Promise<void>) => { setBusy(true); try { await fn() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  const tplTheme = b?.templates.find(t => t.key === tpl)?.theme
  const create = () => run(async () => {
    const own = mine?.find(d => `deck:${d.id}` === tpl)
    const body: Record<string, unknown> = own ? { from_deck: own.id, title: title.trim() || undefined } : { title: title.trim() || 'Untitled deck', template: tpl }
    if (!own && theme) { if (theme.own) { body.theme_id = theme.own } else body.theme = theme.key }
    const r = await api.post<{ deck: Deck }>(`${V1}/decks`, body)
    go(r.deck)
  })
  const draft = () => run(async () => {
    const r = await api.post<{ deck: Deck; text: string }>(`${V1}/ai/draft`, { topic, audience: who, minutes, points, theme: theme && !theme.own ? theme.key : '' })
    go(r.deck, r.text)
  })
  const imported = (r: { deck: Deck; text?: string }) => go(r.deck, r.text)
  const onFile = (f?: File) => { if (f) run(async () => imported(await uploadForm<{ deck: Deck; text: string }>(`${V1}/import/deck`, f))) }
  const shownTheme = theme?.key || tplTheme || 'paper'

  return (
    <Modal title="New deck" onClose={onClose} size="lg"
      footer={tab === 'start' ? <button type="button" className="btn primary" onClick={create} disabled={busy}>{busy && <span className="spinner" />}Make the deck</button>
        : tab === 'ai' ? <button type="button" className="btn primary" onClick={draft} disabled={busy || !topic.trim() || !ai?.ready}>{busy ? <><span className="spinner" />Writing… (up to a minute)</> : <><Icon name="sparkles" size={15} />Draft it</>}</button>
          : md.trim() ? <button type="button" className="btn primary" disabled={busy} onClick={() => run(async () => imported(await api.post(`${V1}/import/deck`, { text: md })))}>{busy && <span className="spinner" />}Make slides from it</button> : undefined}>
      <div className="stack">
        <Tabs tabs={[{ id: 'start', label: 'Start', icon: 'layers' }, { id: 'ai', label: 'Draft with AI', icon: 'sparkles' }, { id: 'import', label: 'Bring in', icon: 'upload' }]} value={tab} onChange={setTab} />
        {tab === 'start' && <>
          <Field label="Title"><input autoFocus value={title} onChange={e => setTitle(e.target.value)} placeholder="What is it about?" onKeyDown={e => { if (e.key === 'Enter') create() }} /></Field>
          <Field group label="Start from">
            <div className="tpl-grid">
              {b?.templates.map(t => (
                <button key={t.key} type="button" className={`tpl ${tpl === t.key ? 'on' : ''}`} onClick={() => setTpl(t.key)}>
                  <b>{t.name}</b><span>{t.about}{t.slides > 1 ? ` · ${t.slides} slides` : ''}</span>
                </button>
              ))}
              {mine?.map(d => (
                <button key={d.id} type="button" className={`tpl ${tpl === `deck:${d.id}` ? 'on' : ''}`} onClick={() => setTpl(`deck:${d.id}`)}>
                  <b><Icon name="bookmark" size={12} /> {d.title}</b><span>Your template · {d.slides} slides</span>
                </button>
              ))}
            </div>
          </Field>
          {!tpl.startsWith('deck:') && <ThemePick value={shownTheme} own={theme?.own} themes={themes || []} onChange={setTheme} />}
        </>}
        {tab === 'ai' && <>
          {ai && !ai.ready && <div className="callout info"><Icon name="info" size={15} /><div>No AI is connected{ai.detail ? ` (${ai.detail})` : ''}: connect Pocket AI or your own key in Settings → AI.<CoreHint /></div></div>}
          <Field label="What is the talk about?"><input autoFocus value={topic} onChange={e => setTopic(e.target.value)} placeholder="Why our town needs a repair café" /></Field>
          <div className="two-col tight">
            <Field label="Who is it for?"><input value={who} onChange={e => setWho(e.target.value)} placeholder="Neighbours at the town hall" /></Field>
            <Field group label="How long?"><Segmented size="sm" value={minutes} onChange={setMinutes} options={[{ value: 5, label: '5 min' }, { value: 10, label: '10' }, { value: 15, label: '15' }, { value: 30, label: '30' }, { value: 45, label: '45' }]} /></Field>
          </div>
          <Field label="What you want to say" help="A few lines, a list, anything. The AI keeps your ideas and puts what to say in the speaker notes.">
            <textarea rows={6} value={points} onChange={e => setPoints(e.target.value)} placeholder={'- things break and we throw them away\n- volunteers can fix most of it\n- first Saturday of the month, in the library'} />
          </Field>
          <ThemePick value={theme?.key || 'paper'} themes={[]} onChange={setTheme} />
        </>}
        {tab === 'import' && <>
          <div className="drop" onClick={() => file.current?.click()} onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); onFile(e.dataTransfer.files[0]) }}>
            <Icon name="upload" size={22} />
            <b>Drop a PowerPoint (.pptx) or Markdown (.md) file</b>
            <span className="muted small">Or press to pick one. Keynote and Google Slides: save as PowerPoint first. The words, speaker notes and the first picture of every slide come in.</span>
            {busy && <span className="spinner" />}
          </div>
          <input ref={file} type="file" accept=".pptx,.potx,.ppsx,.md,.markdown,.txt" hidden onChange={e => { onFile(e.target.files?.[0]); e.target.value = '' }} />
          <div className="app-picks">
            {([['notes', 'note', 'From a note', 'Each heading becomes a slide', 'notes:read'], ['files', 'archive', 'From your files', 'A PowerPoint or Markdown file in Pocket Files', 'files:read'], ['voice', 'mic', 'From a voice memo', 'Talk it through, get a first draft', 'voice:read']] as const).map(([slug, icon, label, sub, scope]) => {
              const st = appState(slug, scope)
              return <button key={slug} type="button" className="app-pick" disabled={!st.ok} title={st.ok ? undefined : st.app?.fix || `Connect ${st.app?.name || slug} in Settings → Connected apps`} onClick={() => setPick(slug)}>
                <Icon name={icon} size={18} /><span className="grow"><b>{label}</b><span className="muted small block-line">{st.ok ? sub : `Needs ${st.app?.name || 'the app'} — Settings → Connected apps`}</span></span>
              </button>
            })}
          </div>
          <Field label="Or paste Markdown" help="A slide per # heading, or between --- lines. “Note:” starts the speaker notes.">
            <textarea rows={6} className="mono" value={md} onChange={e => setMd(e.target.value)} placeholder={'# My talk\n\n---\n\n## The first point\n- one\n- two\n\nNote: say hello first'} />
          </Field>
        </>}
      </div>
      {pick === 'notes' && <NotePicker title="Make a deck from a note" onClose={() => setPick('')} onPick={n => run(async () => imported(await api.post(`${V1}/apps/notes/deck`, { note_id: n.id })))} />}
      {pick === 'files' && <FilePicker want="deck" title="Bring in a file" onClose={() => setPick('')} onPick={(space, path) => run(async () => imported(await api.post(`${V1}/apps/files/import`, { space, path })))} />}
      {pick === 'voice' && <VoicePicker onClose={() => setPick('')} onPick={m => run(async () => imported(await api.post(`${V1}/apps/voice/deck`, { memo_id: m.id })))} />}
    </Modal>
  )
}

export function ThemePick({ value, own, themes, onChange, label = 'Theme' }: { value: string; own?: number; themes: Theme[]; onChange: (t: { key: string; own?: number }) => void; label?: string }) {
  return (
    <Field group label={label}>
      <div className="theme-row">
        {THEMES.map(t => (
          <button key={t.key} type="button" className={`theme-chip ${!own && value === t.key ? 'on' : ''}`} onClick={() => onChange({ key: t.key })} title={t.name}>
            <i style={{ background: `linear-gradient(160deg, ${t.bg}, ${t.bg2})`, ['--a' as string]: t.accent, ['--f' as string]: t.fg, ['--ff' as string]: FONTS[t.head].css }} />{t.name}
          </button>
        ))}
        {themes.map(t => {
          const b = builtinLook(t.base)
          return <button key={t.id} type="button" className={`theme-chip ${own === t.id ? 'on' : ''}`} onClick={() => onChange({ key: t.base, own: t.id })} title={t.name}>
            <i style={{ background: `linear-gradient(160deg, ${t.bg || b.bg}, ${t.bg2 || t.bg || b.bg2})`, ['--a' as string]: t.accent || b.accent, ['--f' as string]: t.fg || b.fg, ['--ff' as string]: FONTS[t.head_font || b.head].css }} />{t.name}
          </button>
        })}
      </div>
    </Field>
  )
}

export function useNewDeckParam(): [string | null, () => void] {
  const [p, setP] = useState<string | null>(() => new URLSearchParams(window.location.search).get('new'))
  useEffect(() => {
    const h = () => setP(new URLSearchParams(window.location.search).get('new'))
    window.addEventListener('popstate', h)
    return () => window.removeEventListener('popstate', h)
  }, [])
  return [p, () => setP(null)]
}
