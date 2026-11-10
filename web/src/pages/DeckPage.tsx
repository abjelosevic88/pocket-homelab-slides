// One deck: the editor, presenting, practice, sharing and its settings.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useAppState, openAction } from '../lib/data'
import type { DeckPayload, Show, Theme } from '../lib/types'
import { fmtMs, lookOf, slideName } from '../lib/slides'
import { abs, origin, ownImage, shown, talkWhen } from '../lib/deck'
import { ago, fmtDateTime } from '../lib/format'
import { Card, Confirm, CopyField, Empty, Field, Loading, MenuButton, Segmented, SettingRow, Switch, Tabs, errMsg, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Editor } from '../components/Editor'
import { SlideView } from '../components/SlideView'
import { QRCode } from '../components/QRCode'
import { ThemePick } from '../components/NewDeck'
import { CalendarPicker, ChatSheet, EmailSheet } from '../components/pickers'
import { useCrumb } from '../components/Layout'


type Tab = 'slides' | 'present' | 'practice' | 'share' | 'settings'

export default function DeckPage() {
  const { id, tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const { toast } = useToast()
  const tab: Tab = (['present', 'practice', 'share', 'settings'] as Tab[]).find(t => t === raw) || 'slides'
  const [p, setPRaw] = useState<DeckPayload | null>(null)
  const [err, setErr] = useState('')
  const [title, setTitle] = useState('')
  const [del, setDel] = useState(false)
  const setP = useCallback((fn: (p: DeckPayload) => DeckPayload) => setPRaw(x => x ? fn(x) : x), [])
  const load = useCallback(() => api.get<DeckPayload>(`${V1}/decks/${id}`).then(r => { setPRaw(r); setTitle(r.deck.title) }).catch(e => setErr(errMsg(e))), [id])
  useEffect(() => { load() }, [load])
  useCrumb(p?.deck.title)
  if (err) return <Empty title="This deck is not here" icon="presentation"><Link to="/">All decks</Link></Empty>
  if (!p) return <Loading />
  const d = p.deck
  const save = async (patch: Record<string, unknown>, msg?: string) => {
    try {
      const r = await api.put<{ deck: typeof d; talk?: { calendar?: string; tasks?: number; tasks_url?: string } }>(`${V1}/decks/${d.id}`, patch)
      setP(x => ({ ...x, deck: r.deck }))
      if (patch.theme_id !== undefined || patch.theme !== undefined) load()
      announceChange()
      if (r.talk?.calendar) toast('The talk is in Pocket Calendar', 'ok', openAction(r.talk.calendar))
      if (r.talk?.tasks) toast(`${r.talk.tasks} tasks added to Pocket Tasks`, 'ok', openAction(r.talk.tasks_url))
      else if (msg) toast(msg)
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const go = (t: Tab) => navigate(t === 'slides' ? `/d/${d.id}` : `/d/${d.id}/${t}`, { replace: true })
  const copyDeck = async (asTemplate = false) => {
    const r = await api.post<DeckPayload>(`${V1}/decks/${d.id}/copy`, {})
    if (asTemplate) await api.put(`${V1}/decks/${r.deck.id}`, { template: true, title: d.title })
    announceChange(); toast(asTemplate ? 'Kept as a template: start new decks from it' : 'Copied'); navigate(`/d/${r.deck.id}`)
  }
  return (
    <div className={tab === 'slides' ? 'ed-page' : 'page'}>
      <div className={`deck-head ${tab === 'slides' ? 'pad-head' : ''}`}>
        <input className="deck-title-input" value={title} onChange={e => setTitle(e.target.value)} onBlur={() => { if (title.trim() && title !== d.title) save({ title }) }} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }} aria-label="Deck title" />
        {d.live && <span className="live-pill">Live</span>}
        <span className="grow" />
        {!mobile && <Tabs tabs={[{ id: 'slides', label: 'Slides', icon: 'layers' }, { id: 'present', label: 'Present', icon: 'play' }, { id: 'practice', label: 'Practice', icon: 'timer' }, { id: 'share', label: 'Share', icon: 'share' }, { id: 'settings', label: 'Deck', icon: 'settings' }]} value={tab} onChange={go} />}
        <button type="button" className="btn primary" onClick={() => navigate(`/present/${d.id}`)} title="Present (full screen)"><Icon name="play" size={15} />Present</button>
        <MenuButton label="More" align="right" width={240} items={[
          { label: d.pinned ? 'Unpin' : 'Pin to the top', icon: 'pin', onSelect: () => save({ pinned: !d.pinned }) },
          { label: 'Make a copy', icon: 'copy', onSelect: () => copyDeck() },
          { label: d.template ? 'Not a template any more' : 'Use as a template', icon: 'bookmark', onSelect: () => save({ template: !d.template }, d.template ? 'No longer a template' : 'New decks can start from this one') },
          { label: 'Print or save as PDF', icon: 'printer', onSelect: () => window.open(`/d/${d.id}/print`, '_blank') },
          { label: 'Outline as Markdown', icon: 'download', onSelect: () => { window.location.href = `${V1}/decks/${d.id}/export.md` } },
          'sep',
          { label: d.archived ? 'Bring back from the archive' : 'Archive', icon: 'archive', onSelect: () => save({ archived: !d.archived }, d.archived ? 'Back with your decks' : 'Archived') },
          { label: 'Delete the deck', icon: 'trash', danger: true, onSelect: () => setDel(true) },
        ]}><Icon name="more" size={16} /></MenuButton>
      </div>
      {mobile && <div className="mobile-tabs"><Tabs tabs={[{ id: 'slides', label: 'Slides', icon: 'layers' }, { id: 'present', label: 'Present', icon: 'play' }, { id: 'practice', label: 'Practice', icon: 'timer' }, { id: 'share', label: 'Share', icon: 'share' }, { id: 'settings', label: 'Deck', icon: 'settings' }]} value={tab} onChange={go} /></div>}
      {tab === 'slides' && <Editor p={p} setP={setP} />}
      {tab === 'present' && <PresentTab p={p} save={save} reload={load} />}
      {tab === 'practice' && <PracticeTab p={p} />}
      {tab === 'share' && <ShareTab p={p} save={save} reload={load} />}
      {tab === 'settings' && <SettingsTab p={p} save={save} />}
      {del && <Confirm title={`Delete “${d.title}”?`} message="Its slides, pictures, talks and rehearsals go with it. The share and audience links stop working." confirmLabel="Delete"
        onCancel={() => setDel(false)} onConfirm={async () => { await api.del(`${V1}/decks/${d.id}`); announceChange(); toast('Deleted'); navigate('/') }} />}
    </div>
  )
}

type Save = (patch: Record<string, unknown>, msg?: string) => Promise<void>

function PresentTab({ p, save, reload }: { p: DeckPayload; save: Save; reload: () => void }) {
  const d = p.deck
  const navigate = useNavigate()
  const { toast } = useToast()
  const appState = useAppState()
  const [cal, setCal] = useState(false)
  const [chat, setChat] = useState(false)
  const [talk, setTalk] = useState({ talk_title: d.talk_title, talk_at: d.talk_at, talk_place: d.talk_place, target_minutes: d.target_minutes })
  useEffect(() => { setTalk({ talk_title: d.talk_title, talk_at: d.talk_at, talk_place: d.talk_place, target_minutes: d.target_minutes }) }, [d.talk_title, d.talk_at, d.talk_place, d.target_minutes])
  const live = origin() + '/w/' + d.live_key
  const calendar = appState('calendar', 'calendar:write'), tasks = appState('tasks', 'tasks:write'), chatOk = appState('chat', 'chat:write')
  const act = async (url: string) => { try { const r = await api.post<{ text: string; url?: string }>(url); toast(r.text, 'ok', openAction(r.url)); reload() } catch (e) { toast(errMsg(e), 'err') } }
  const dirty = talk.talk_title !== d.talk_title || talk.talk_at !== d.talk_at || talk.talk_place !== d.talk_place || talk.target_minutes !== d.target_minutes
  return (
    <div className="stack loose">
      <div className="present-cards">
        <div className="present-card">
          <h3><Icon name="presentation" size={17} />Present</h3>
          <p>Full screen on this screen (or the projector). A code on the screen turns your phone into the presenter screen: the next slide, your notes, a timer.</p>
          <div className="row wrap">
            <button type="button" className="btn primary" onClick={() => navigate(`/present/${d.id}`)}><Icon name="play" size={15} />Present</button>
            <button type="button" className="btn" onClick={() => window.open(`/present/${d.id}/presenter`, 'psld-presenter', 'width=1100,height=720')} title="For a laptop with a second screen: the presenter screen in its own window">
              <Icon name="monitor" size={15} />Presenter window</button>
          </div>
        </div>
        <div className="present-card">
          <h3><Icon name="timer" size={17} />Rehearse</h3>
          <p>Go through it with a timer: every slide's time is kept, nobody can follow along, and you see where you ran over.</p>
          <div className="row wrap"><button type="button" className="btn" onClick={() => navigate(`/present/${d.id}?rehearse=1`)}><Icon name="timer" size={15} />Rehearse now</button>
            <Link to={`/d/${d.id}/practice`} className="btn ghost">{d.rehearsals ? `${d.rehearsals} so far` : 'None yet'}</Link></div>
        </div>
        <div className="present-card">
          <h3><Icon name="smartphone" size={17} />The audience</h3>
          <p>People follow on their phones: the slides so far, the polls, questions{d.questions ? '' : ' (off for this deck)'} and a few reactions. Only while you present a talk.</p>
          <div className="qr-pair"><QRCode value={live} size={120} label="Follow along" /><div><div className="big-code">{d.live_key.toUpperCase()}</div><span className="muted small">{origin().replace(/^https?:\/\//, '')}/w</span></div></div>
          <div className="row wrap">
            <button type="button" className="btn sm" disabled={!chatOk.ok} title={chatOk.ok ? undefined : 'Connect Pocket Chat in Settings → Connected apps'} onClick={() => setChat(true)}><Icon name="message" size={14} />Send to a chat</button>
            <label className="check-row small"><Switch size="sm" checked={d.questions} onChange={v => save({ questions: v })} />Questions</label>
          </div>
        </div>
      </div>
      {p.show && <LiveNow p={p} reload={reload} />}
      <Card title="When and where" sub="For the reminders the day before, your calendar and Pocket Day.">
        <div className="pad stack">
          <div className="two-col tight">
            <Field label="The event"><input value={talk.talk_title} onChange={e => setTalk({ ...talk, talk_title: e.target.value })} placeholder={d.title} /></Field>
            <Field label="Where"><input value={talk.talk_place} onChange={e => setTalk({ ...talk, talk_place: e.target.value })} placeholder="The town hall, room 2" /></Field>
            <Field label="Day and time"><input type="datetime-local" value={talk.talk_at.length === 10 ? talk.talk_at + 'T09:00' : talk.talk_at} onChange={e => setTalk({ ...talk, talk_at: e.target.value })} /></Field>
            <Field label="Minutes" help="How long it should take; the phone timer warns before the end."><input type="number" min={0} max={600} value={talk.target_minutes || ''} onChange={e => setTalk({ ...talk, target_minutes: Number(e.target.value) || 0 })} placeholder="20" /></Field>
          </div>
          <div className="row wrap">
            <button type="button" className="btn primary" disabled={!dirty} onClick={() => save(talk, 'Saved')}>Save</button>
            <button type="button" className="btn" disabled={!appState('calendar', 'calendar:read').ok} onClick={() => setCal(true)}><Icon name="calendar" size={15} />Pick from Pocket Calendar</button>
            <button type="button" className="btn" disabled={!calendar.ok || !d.talk_at} title={!calendar.ok ? 'Connect Pocket Calendar in Settings → Connected apps' : !d.talk_at ? 'Set the day first' : undefined} onClick={() => act(`${V1}/decks/${d.id}/calendar`)}><Icon name="calendar-plus" size={15} />{d.calendar_url ? 'Put it in again' : 'Put it in Pocket Calendar'}</button>
            <button type="button" className="btn" disabled={!tasks.ok || !d.talk_at} title={!tasks.ok ? 'Connect Pocket Tasks in Settings → Connected apps' : !d.talk_at ? 'Set the day first' : undefined} onClick={() => act(`${V1}/decks/${d.id}/tasks`)}><Icon name="check-square" size={15} />Rehearse tasks</button>
            {d.calendar_url && <a className="btn ghost" href={d.calendar_url} target="_blank" rel="noopener">Open in Calendar<Icon name="external" size={13} /></a>}
          </div>
          {d.talk_at && <p className="muted small">{talkWhen(d.talk_at)}{d.talk_place && ` · ${d.talk_place}`}</p>}
        </div>
      </Card>
      {cal && <CalendarPicker onClose={() => setCal(false)} onPick={e => { setCal(false); save({ talk_title: e.title, talk_at: e.at, talk_place: e.place }, 'The talk is set') }} />}
      {chat && <ChatSheet deckId={d.id} what="live" onClose={() => setChat(false)} />}
    </div>
  )
}

function LiveNow({ p, reload }: { p: DeckPayload; reload: () => void }) {
  const { toast } = useToast()
  const h = p.show!
  const vis = shown(p.slides)
  const end = async () => { await api.post(`${V1}/decks/${p.deck.id}/live`, { action: 'end' }); toast('The show ended'); reload() }
  return (
    <Card title={<><span className="live-pill">Live</span> {h.kind === 'talk' ? 'Being presented now' : 'A rehearsal is running'}</>} sub={`Started ${ago(h.started_at)} · slide ${h.pos + 1} of ${vis.length}`}
      actions={<><a className="btn sm" href={`/present/${p.deck.id}/presenter`}>Presenter screen</a><button type="button" className="btn sm danger-text" onClick={end}>End it</button></>}>
      <div className="pad qr-pair">
        {abs(p.remote_url) && <><QRCode value={abs(p.remote_url)} size={120} label="The phone link" /><p className="muted small" style={{ maxWidth: 360 }}>Scan with your phone to get the presenter screen without signing in. The link ends with the show.</p></>}
      </div>
    </Card>
  )
}

function PracticeTab({ p }: { p: DeckPayload }) {
  const d = p.deck
  const { toast } = useToast()
  const { data: shows } = useAsync(() => api.get<{ shows: Show[] }>(`${V1}/shows?deck=${d.id}`).then(r => r.shows), [d.id])
  const [tips, setTips] = useState<{ summary: string; tips: { slide: number; tip: string }[] } | null>(null)
  const [busy, setBusy] = useState(false)
  const last = shows?.find(h => h.ended_at && h.kind === 'rehearsal') || shows?.find(h => h.ended_at)
  const vis = shown(p.slides)
  const check = async () => { setBusy(true); try { setTips(await api.post(`${V1}/decks/${d.id}/ai/check`)) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  return (
    <div className="stack loose">
      <Card title="What to fix before the talk" sub="Pocket AI reads the slides and your notes the way a friendly listener would." actions={<button type="button" className="btn sm" onClick={check} disabled={busy}>{busy ? <span className="spinner" /> : <Icon name="sparkles" size={14} />}Read it through</button>}>
        {tips ? <div className="pad stack"><p><b>{tips.summary}</b></p><ul className="tips">{tips.tips.map((t, i) => <li key={i}>{t.slide > 0 && <span className="badge">Slide {t.slide}</span>} {t.tip}</li>)}</ul></div> : <div className="pad muted small">Nothing read yet.</div>}
      </Card>
      {last && <TimingCard show={last} slides={vis} target={d.target_minutes} />}
      <Card title="Talks and rehearsals" flush>
        {!shows ? <Loading /> : shows.length === 0 ? <div className="pad muted small">Rehearse from the Present tab: every slide's time is kept here.</div> : (
          <div className="rows">{shows.map(h => (
            <Link key={h.id} to={`/d/${d.id}/shows/${h.id}`} className="row">
              <Icon name={h.kind === 'talk' ? 'presentation' : 'timer'} size={16} />
              <span className="grow"><b>{h.kind === 'talk' ? 'Talk' : 'Rehearsal'}</b> <span className="muted small">· {fmtDateTime(h.started_at)}</span></span>
              {h.kind === 'talk' && <span className="muted small nowrap">{h.viewers} following · {h.questions} questions</span>}
              <span className={`small nowrap tnum ${d.target_minutes && h.total_ms > d.target_minutes * 60000 + 60000 ? 'bad' : ''}`}>{h.ended_at ? fmtMs(h.total_ms) : 'running'}</span>
            </Link>
          ))}</div>
        )}
      </Card>
    </div>
  )
}

export function TimingCard({ show, slides, target }: { show: Show; slides: { id: number; title: string; subtitle: string; body: string; layout: string; seconds: number }[]; target: number }) {
  const total = show.total_ms || Object.values(show.timings).reduce((a, b) => a + b, 0)
  const fair = target > 0 && slides.length ? target * 60000 / slides.length : 0
  const max = Math.max(1, ...slides.map(s => Math.max(show.timings[String(s.id)] || 0, (s.seconds || 0) * 1000, fair)))
  return (
    <Card title={`${show.kind === 'talk' ? 'The talk' : 'The last rehearsal'}: ${fmtMs(total)}`} sub={target ? `Planned ${target} minutes · ${total > target * 60000 ? `${fmtMs(total - target * 60000)} over` : `${fmtMs(target * 60000 - total)} to spare`}` : 'Set the minutes on the Present tab to see where you run over.'} flush>
      <div className="timing-list">{slides.map((s, i) => {
        const ms = show.timings[String(s.id)] || 0
        const plan = s.seconds ? s.seconds * 1000 : fair
        return (
          <div key={s.id} className="timing-row">
            <span className="muted small tnum">{i + 1}</span>
            <span className="ellipsis small">{slideName(s as never)}</span>
            <span className={`small tnum ${plan && ms > plan * 1.25 ? 'bad' : ''}`}>{ms ? fmtMs(ms) : '—'}</span>
            <span className="timing-bar" title={plan ? `Planned ${fmtMs(plan)}` : undefined}><i className={plan && ms > plan * 1.25 ? 'over' : ''} style={{ width: `${ms * 100 / max}%` }} />{plan > 0 && <b style={{ left: `${plan * 100 / max}%` }} />}</span>
          </div>
        )
      })}</div>
    </Card>
  )
}

function ShareTab({ p, save, reload }: { p: DeckPayload; save: Save; reload: () => void }) {
  const d = p.deck
  const { toast } = useToast()
  const appState = useAppState()
  const [sheet, setSheet] = useState<'' | 'email' | 'chat'>('')
  const [lang, setLang] = useState('')
  const [busy, setBusy] = useState('')
  const navigate = useNavigate()
  const share = async (patch: Record<string, unknown>) => { try { await api.put(`${V1}/decks/${d.id}/share`, patch); reload(); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const post = async (key: string, url: string, body: Record<string, unknown>) => {
    setBusy(key)
    try { const r = await api.post<{ text: string; url?: string; deck?: { id: number } }>(url, body); toast(r.text, 'ok', openAction(r.url)); if (r.deck) { announceChange(); navigate(`/d/${r.deck.id}`) } } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const html = async (toFiles: boolean) => {
    setBusy(toFiles ? 'files-html' : 'html')
    try {
      const { exportHTML } = await import('../lib/exporthtml')
      const page = await exportHTML(p)
      if (toFiles) { const r = await api.post<{ text: string; url?: string }>(`${V1}/apps/files`, { deck_id: d.id, kind: 'html', html: page }); toast(r.text, 'ok', openAction(r.url)) }
      else { const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([page], { type: 'text/html' })); a.download = `${d.title.replace(/[/\\:*?"<>|]/g, '-')}.html`; a.click(); URL.revokeObjectURL(a.href) }
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const app = (slug: Parameters<typeof appState>[0], scope: string) => { const s = appState(slug, scope); return { disabled: !s.ok, title: s.ok ? undefined : `Connect ${s.app?.name || slug} in Settings → Connected apps` } }
  const spin = (k: string, icon: string) => busy === k ? <span className="spinner" /> : <Icon name={icon} size={15} />
  return (
    <div className="stack loose">
      <Card title="Share link" sub="Anyone with the link can look through the slides (never your notes or the skipped ones). Switch it off and the link stops working.">
        <div className="pad stack">
          <SettingRow title="Share the slides"><Switch checked={d.shared} onChange={v => share({ shared: v })} label="Share" /></SettingRow>
          {d.shared && <div className="qr-pair"><QRCode value={abs(p.share_url)} size={110} label="The slides" /><div className="grow stack"><CopyField value={abs(p.share_url)} /><div className="row wrap"><a className="btn sm" href={abs(p.share_url)} target="_blank" rel="noopener"><Icon name="external" size={14} />Open</a><button type="button" className="btn sm ghost" onClick={() => share({ new_keys: true })}>New links (the old ones stop)</button></div></div></div>}
        </div>
      </Card>
      <Card title="Send it" sub="To the people who came, a chat, or your other apps.">
        <div className="pad app-picks">
          <button type="button" className="app-pick" {...app('email', 'mail:send')} onClick={() => setSheet('email')}><Icon name="mail" size={18} /><span className="grow"><b>Email the slides</b><span className="muted small block-line">From your own account, to people from Contacts</span></span></button>
          <button type="button" className="app-pick" {...app('chat', 'chat:write')} onClick={() => setSheet('chat')}><Icon name="message" size={18} /><span className="grow"><b>Send to a chat</b><span className="muted small block-line">The link, as a card in Pocket Chat</span></span></button>
          <button type="button" className="app-pick" {...app('notes', 'notes:write')} onClick={() => post('notes', `${V1}/apps/notes`, { deck_id: d.id })}>{spin('notes', 'note')}<span className="grow"><b>Keep as a note</b><span className="muted small block-line">The outline with your speaker notes</span></span></button>
          <button type="button" className="app-pick" {...app('blog', 'blog:write')} onClick={() => post('blog', `${V1}/apps/blog`, { deck_id: d.id })}>{spin('blog', 'feather')}<span className="grow"><b>Write it up as a post</b><span className="muted small block-line">A draft in Pocket Blog, your notes as the words</span></span></button>
          <button type="button" className="app-pick" {...app('learn', 'learn:write')} onClick={() => post('learn', `${V1}/apps/learn`, { deck_id: d.id })}>{spin('learn', 'brain')}<span className="grow"><b>Make flashcards</b><span className="muted small block-line">Headings and points, quiz answers, into Pocket Learn</span></span></button>
          <button type="button" className="app-pick" {...app('files', 'files:write')} onClick={() => html(true)}>{spin('files-html', 'archive')}<span className="grow"><b>Save in Pocket Files</b><span className="muted small block-line">One web page that opens anywhere, offline</span></span></button>
        </div>
      </Card>
      <Card title="Download" sub="To keep, to print, or to present somewhere else.">
        <div className="pad row wrap">
          <a className="btn" href={`/d/${d.id}/print`} target="_blank" rel="noopener"><Icon name="printer" size={15} />Print or PDF</a>
          <a className="btn" href={`/d/${d.id}/print?notes=1`} target="_blank" rel="noopener"><Icon name="note" size={15} />With speaker notes</a>
          <button type="button" className="btn" onClick={() => html(false)} disabled={!!busy}>{spin('html', 'download')}Web page (.html)</button>
          <a className="btn" href={`${V1}/decks/${d.id}/export.md`}><Icon name="download" size={15} />Outline (.md)</a>
        </div>
      </Card>
      <Card title="In another language" sub="A copy of the deck, its words and speaker notes translated by Pocket Translate.">
        <div className="pad row wrap">
          <input value={lang} onChange={e => setLang(e.target.value)} placeholder="Language: de, Serbian, fr…" style={{ maxWidth: 240 }} />
          <button type="button" className="btn" {...app('translate', 'translate:run')} disabled={!lang.trim() || !!busy || !appState('translate', 'translate:run').ok} onClick={() => post('tr', `${V1}/decks/${d.id}/translate`, { to: lang.trim(), notes: true })}>{spin('tr', 'globe')}Make the copy</button>
        </div>
      </Card>
      {sheet === 'email' && <EmailSheet deckId={d.id} onClose={() => { setSheet(''); reload() }} />}
      {sheet === 'chat' && <ChatSheet deckId={d.id} what="share" onClose={() => { setSheet(''); reload() }} />}
    </div>
  )
}

function SettingsTab({ p, save }: { p: DeckPayload; save: Save }) {
  const d = p.deck
  const { data: themes } = useAsync(() => api.get<{ themes: Theme[] }>(`${V1}/themes`).then(r => r.themes), [])
  const [about, setAbout] = useState(d.about)
  const [footer, setFooter] = useState(d.footer)
  const [tags, setTags] = useState(d.tags.join(', '))
  const look = useMemo(() => lookOf(d, p.theme), [d, p.theme])
  const first = shown(p.slides)[0]
  return (
    <div className="stack loose">
      <Card title="Look">
        <div className="pad stack">
          <ThemePick value={d.theme} own={d.theme_id || undefined} themes={themes || []} onChange={t => save(t.own ? { theme: t.key, theme_id: t.own } : { theme: t.key, theme_id: 0 })} />
          <p className="muted small"><Link to="/library">Make your own theme</Link>: colours, fonts and a logo.</p>
          {first && <div style={{ maxWidth: 520 }}><SlideView slide={first} deck={d} look={look} imageUrl={ownImage} index={0} total={1} /></div>}
          <SettingRow title="Shape"><Segmented size="sm" value={d.aspect} onChange={v => save({ aspect: v })} options={[{ value: '16:9', label: 'Wide 16:9' }, { value: '4:3', label: 'Square 4:3' }]} /></SettingRow>
          <SettingRow title="Between slides"><Segmented size="sm" value={d.transition} onChange={v => save({ transition: v })} options={[{ value: 'fade', label: 'Fade' }, { value: 'slide', label: 'Slide' }, { value: 'none', label: 'Cut' }]} /></SettingRow>
          <SettingRow title="Slide numbers"><Switch checked={d.numbers} onChange={v => save({ numbers: v })} label="Numbers" /></SettingRow>
          <SettingRow title="Footer" help="On every slide but the first."><input value={footer} onChange={e => setFooter(e.target.value)} onBlur={() => footer !== d.footer && save({ footer })} placeholder="Your name · the event" /></SettingRow>
        </div>
      </Card>
      <Card title="About this deck">
        <div className="pad stack">
          <Field label="What it is about" help="For you, search and the share page."><textarea rows={3} value={about} onChange={e => setAbout(e.target.value)} onBlur={() => about !== d.about && save({ about })} /></Field>
          <Field label="Tags" help="Separated by commas."><input value={tags} onChange={e => setTags(e.target.value)} onBlur={() => tags !== d.tags.join(', ') && save({ tags: tags.split(',').map(t => t.trim()).filter(Boolean) })} placeholder="work, meetup" /></Field>
          <SettingRow title="A template" help="New decks can start from it (New deck → Start from)."><Switch checked={d.template} onChange={v => save({ template: v })} label="Template" /></SettingRow>
          <SettingRow title="The audience may ask questions"><Switch checked={d.questions} onChange={v => save({ questions: v })} label="Questions" /></SettingRow>
          <p className="muted small">Made {fmtDateTime(d.created_at)}{d.source ? ` by ${d.source}` : ''} · changed {ago(d.updated_at)}</p>
        </div>
      </Card>
    </div>
  )
}
