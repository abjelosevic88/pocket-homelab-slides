// The deck editor: thumbnails on the left, the slide in the middle with
// its speaker notes under it, its fields on the right. Every change saves by
// itself a moment after typing stops.
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useAppState } from '../lib/data'
import type { DeckPayload, LayoutKey, MyLayout, Slide, SlideData } from '../lib/types'
import { LAYOUT_ICONS, LAYOUT_NAMES, lookOf, slideName, stepsOf, type Look } from '../lib/slides'
import { abs, agendaOf, origin, ownImage, shown } from '../lib/deck'
import { uploadFile } from '../lib/upload'
import { Field, MenuButton, Modal, Segmented, Switch, errMsg, useIsMobile, useToast, type MenuItem } from './ui'
import { Icon } from './icons'
import { SlideView } from './SlideView'
import { FilePicker, PhotoPicker, TablePicker } from './pickers'
import { useBuiltins } from './NewDeck'

const ALL: LayoutKey[] = ['title', 'section', 'bullets', 'two', 'image_right', 'image_left', 'image', 'quote', 'number', 'chart', 'table', 'code', 'poll', 'agenda', 'end', 'blank']

export function Editor({ p, setP }: { p: DeckPayload; setP: (fn: (p: DeckPayload) => DeckPayload) => void }) {
  const loc = useLocation()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const { toast } = useToast()
  const deck = p.deck
  const look = useMemo(() => lookOf(deck, p.theme), [deck, p.theme])
  const want = Number(new URLSearchParams(loc.search).get('slide')) || 0
  const [sel, setSel] = useState<number>(() => want || p.slides[0]?.id || 0)
  useEffect(() => { if (want) setSel(want) }, [want])
  const slide = p.slides.find(s => s.id === sel) || p.slides[0]
  const [layouts, setLayouts] = useState<MyLayout[]>([])
  useEffect(() => { api.get<{ layouts: MyLayout[] }>(`${V1}/layouts`).then(r => setLayouts(r.layouts)).catch(() => {}) }, [])
  const vis = shown(p.slides)
  const agenda = agendaOf(p.slides)
  const qr = { share: abs(p.share_url), live: p.live_url }

  // ---- saving: a change shows at once, and goes to the server a moment later ----
  const [saving, setSaving] = useState<'' | 'saving' | 'saved' | 'error'>('')
  const pending = useRef<Map<number, Partial<Slide>>>(new Map())
  const timer = useRef(0)
  const flush = useCallback(async () => {
    const all = [...pending.current.entries()]
    pending.current.clear()
    if (!all.length) return
    setSaving('saving')
    try {
      for (const [id, patch] of all) await api.put(`${V1}/slides/${id}`, patch)
      setSaving('saved')
    } catch (e) { setSaving('error'); toast(errMsg(e), 'err') }
  }, [toast])
  useEffect(() => () => { window.clearTimeout(timer.current); void flush() }, [flush])
  const change = (id: number, patch: Partial<Slide>) => {
    setP(x => ({ ...x, slides: x.slides.map(s => s.id === id ? { ...s, ...patch, data: patch.data ? patch.data : s.data } : s) }))
    const cur = pending.current.get(id) || {}
    pending.current.set(id, { ...cur, ...patch })
    setSaving('saving')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(flush, 600)
  }
  const setData = (s: Slide, d: Partial<SlideData>) => change(s.id, { data: { ...s.data, ...d } })

  const reload = useCallback(async () => {
    const r = await api.get<DeckPayload>(`${V1}/decks/${deck.id}`)
    setP(() => r)
  }, [deck.id, setP])

  const add = async (body: Record<string, unknown>) => {
    await flush()
    try {
      const r = await api.post<{ slide: Slide }>(`${V1}/decks/${deck.id}/slides`, { after: slide?.id, ...body })
      await reload()
      setSel(r.slide.id)
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const copy = async (s: Slide) => { await flush(); const r = await api.post<{ slide: Slide }>(`${V1}/slides/${s.id}/copy`); await reload(); setSel(r.slide.id) }
  const remove = async (s: Slide) => {
    const i = p.slides.findIndex(x => x.id === s.id)
    await flush()
    await api.del(`${V1}/slides/${s.id}`)
    await reload()
    setSel(p.slides[i + 1]?.id || p.slides[i - 1]?.id || 0)
    toast('Slide removed')
  }
  const order = async (ids: number[]) => {
    setP(x => ({ ...x, slides: ids.map(id => x.slides.find(s => s.id === id)!).filter(Boolean).map((s, i) => ({ ...s, position: i })) }))
    try { await api.put(`${V1}/decks/${deck.id}/order`, { ids }) } catch (e) { toast(errMsg(e), 'err'); reload() }
  }
  const move = (s: Slide, by: number) => {
    const ids = p.slides.map(x => x.id)
    const i = ids.indexOf(s.id), j = i + by
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    order(ids)
  }
  const keep = async (s: Slide) => {
    const name = window.prompt('Name of the layout (it stays in your layouts for every deck)', s.title || LAYOUT_NAMES[s.layout])
    if (name === null) return
    try { const r = await api.post<{ layout: MyLayout }>(`${V1}/slides/${s.id}/keep`, { name }); setLayouts(l => [...l, r.layout]); toast('Kept in your layouts') } catch (e) { toast(errMsg(e), 'err') }
  }

  // ---- keys ----
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (!slide) return
      const typing = (e.target as HTMLElement).closest('input, textarea, [contenteditable="true"]')
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); add({ layout: slide.layout === 'title' ? 'bullets' : slide.layout }) }
      else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'd') { e.preventDefault(); copy(slide) }
      else if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) { e.preventDefault(); move(slide, e.key === 'ArrowUp' ? -1 : 1) }
      else if (!typing && !document.querySelector('.modal-backdrop, .menu') && (e.key === 'ArrowUp' || e.key === 'ArrowDown' || e.key === 'j' || e.key === 'k')) {
        const i = p.slides.findIndex(x => x.id === slide.id) + (e.key === 'ArrowUp' || e.key === 'k' ? -1 : 1)
        if (p.slides[i]) { e.preventDefault(); setSel(p.slides[i].id) }
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })

  // ---- drag to reorder ----
  const [drag, setDrag] = useState<number | null>(null)
  const [over, setOver] = useState<number | null>(null)
  const drop = (target: number) => {
    if (drag === null || drag === target) return
    const ids = p.slides.map(x => x.id).filter(id => id !== drag)
    ids.splice(ids.indexOf(target), 0, drag)
    order(ids)
  }

  const addMenu = (): MenuItem[] => [
    ...ALL.map(k => ({ label: LAYOUT_NAMES[k], icon: LAYOUT_ICONS[k], onSelect: () => add({ layout: k }) })),
    ...(layouts.length ? ['sep' as const, ...layouts.map(l => ({ label: l.name, icon: 'bookmark', hint: 'yours', onSelect: () => add({ layout_id: l.id }) }))] : []),
  ]
  const index = slide ? vis.findIndex(v => v.id === slide.id) : -1

  if (!slide) return (
    <div className="ed-page"><div className="empty"><h3>No slides yet</h3><MenuButton items={addMenu} className="btn primary" label="Add a slide"><Icon name="plus" size={15} />Add a slide</MenuButton></div></div>
  )
  return (
    <div className="ed-page">
      <div className="ed">
        <div className="ed-rail" aria-label="Slides">
          {p.slides.map(s => {
            const vi = vis.findIndex(v => v.id === s.id)
            return (
              <button key={s.id} type="button" className={`ed-thumb ${s.id === slide.id ? 'on' : ''} ${s.hidden ? 'off' : ''} ${over === s.id && drag !== s.id ? 'drag-over' : ''}`}
                onClick={() => setSel(s.id)} draggable onDragStart={() => setDrag(s.id)} onDragEnd={() => { setDrag(null); setOver(null) }}
                onDragOver={e => { e.preventDefault(); setOver(s.id) }} onDrop={e => { e.preventDefault(); drop(s.id); setDrag(null); setOver(null) }} title={slideName(s)}>
                <span className="ed-thumb-n">{s.hidden ? '–' : vi + 1}</span>
                <span className="grow" style={{ position: 'relative' }}>
                  <SlideView slide={s} deck={deck} look={look} imageUrl={ownImage} thumb index={vi} total={vis.length} agenda={agenda} qr={qr} />
                  <span className="ed-thumb-flags">
                    {s.notes.trim() && <span title="Has speaker notes"><Icon name="note" size={10} /></span>}
                    {s.hidden && <span title="Skipped when presenting"><Icon name="eye-off" size={10} /></span>}
                    {stepsOf(s) > 0 && <span title="Step by step"><Icon name="list" size={10} /></span>}
                  </span>
                </span>
              </button>
            )
          })}
          <MenuButton items={addMenu} className="btn sm ed-add" label="Add a slide" width={250}><Icon name="plus" size={14} />Slide</MenuButton>
        </div>

        <div className="ed-stage">
          <div className="ed-stage-bar">
            <span className="muted small grow">{slide.hidden ? 'Skipped when presenting' : `Slide ${index + 1} of ${vis.length}`} · {LAYOUT_NAMES[slide.layout]}</span>
            <span className={`save-state ${saving === 'error' ? 'error' : ''}`}>{saving === 'saving' ? 'Saving…' : saving === 'saved' ? 'Saved' : saving === 'error' ? 'Not saved' : ''}</span>
            <button type="button" className="btn sm ghost icon-only" title="Move up (alt ↑)" aria-label="Move up" disabled={p.slides[0]?.id === slide.id} onClick={() => move(slide, -1)}><Icon name="arrow-up" size={15} /></button>
            <button type="button" className="btn sm ghost icon-only" title="Move down (alt ↓)" aria-label="Move down" disabled={p.slides[p.slides.length - 1]?.id === slide.id} onClick={() => move(slide, 1)}><Icon name="arrow-down" size={15} /></button>
            <AIHelp slide={slide} look={look} p={p} onUse={patch => change(slide.id, patch)} />
            <MenuButton label="More" width={240} align="right" items={[
              { label: 'Copy the slide', icon: 'copy', hint: '⌘D', onSelect: () => copy(slide) },
              { label: 'Keep as my layout', icon: 'bookmark', onSelect: () => keep(slide) },
              { label: slide.hidden ? 'Show it when presenting' : 'Skip it when presenting', icon: slide.hidden ? 'eye' : 'eye-off', onSelect: () => change(slide.id, { hidden: !slide.hidden }) },
              { label: 'Present from here', icon: 'play', disabled: slide.hidden, onSelect: () => navigate(`/present/${deck.id}?from=${Math.max(0, index)}`) },
              'sep',
              { label: 'Remove the slide', icon: 'trash', danger: true, disabled: p.slides.length < 2, onSelect: () => remove(slide) },
            ]}><Icon name="more" size={16} /></MenuButton>
          </div>
          <SlideView slide={slide} deck={deck} look={look} imageUrl={ownImage} index={index} total={vis.length} agenda={agenda} qr={qr} placeholders />
          <div className="ed-notes">
            <div className="row"><h4 className="muted small grow" style={{ margin: 0 }}><Icon name="note" size={13} className="inline-ico" /> Speaker notes — only you see them, on your phone while presenting</h4>
              <label className="row small muted" title="How long this slide should take">
                <Icon name="timer" size={13} /><input className="secs" type="number" min={0} max={3600} value={slide.seconds || ''} placeholder="sec" onChange={e => change(slide.id, { seconds: Math.max(0, Number(e.target.value) || 0) })} aria-label="Seconds for this slide" />
              </label>
            </div>
            <textarea value={slide.notes} onChange={e => change(slide.id, { notes: e.target.value })} placeholder="What to say here: a story, a number to mention, a question to ask the room…" />
          </div>
        </div>

        <aside className="ed-side" aria-label="This slide">
          <Inspector key={slide.id} s={slide} deckId={deck.id} change={patch => change(slide.id, patch)} setData={d => setData(slide, d)} reload={reload} mobile={mobile} />
        </aside>
      </div>
    </div>
  )
}

function Inspector({ s, deckId, change, setData, reload, mobile }: { s: Slide; deckId: number; change: (p: Partial<Slide>) => void; setData: (d: Partial<SlideData>) => void; reload: () => Promise<void>; mobile: boolean }) {
  const { data: b } = useBuiltins()
  const uses = b?.layouts.find(l => l.key === s.layout)?.uses || []
  const has = (k: string) => uses.includes(k)
  const labels: Record<string, [string, string]> = {
    number: ['The number', 'What it means'], quote: ['', 'Who said it'], image: ['Caption', 'A line under the caption'], title: ['Title', 'A line under it'], section: ['Title', 'Above it (e.g. “Part two”)'],
    end: ['Big words', 'A line under them'],
  }
  const [tl, sl] = labels[s.layout] || ['Heading', 'A line under it']
  return (
    <>
      <div>
        <h4>Layout</h4>
        <div className="layout-grid" style={{ marginTop: 8 }}>
          {ALL.map(k => (
            <button key={k} type="button" className={`layout-btn ${s.layout === k ? 'on' : ''}`} onClick={() => change({ layout: k })} title={b?.layouts.find(l => l.key === k)?.about}>
              <Icon name={LAYOUT_ICONS[k]} size={17} />{LAYOUT_NAMES[k]}
            </button>
          ))}
        </div>
      </div>
      {has('title') && <Field label={tl}><input value={s.title} onChange={e => change({ title: e.target.value })} autoFocus={!mobile && !s.title} /></Field>}
      {has('subtitle') && <Field label={sl}><input value={s.subtitle} onChange={e => change({ subtitle: e.target.value })} /></Field>}
      {s.layout === 'two' && <div className="two-col tight">
        <Field label="Left heading"><input value={s.data.left || ''} onChange={e => setData({ left: e.target.value })} placeholder="Before" /></Field>
        <Field label="Right heading"><input value={s.data.right || ''} onChange={e => setData({ right: e.target.value })} placeholder="After" /></Field>
      </div>}
      {has('body') && <Field label={s.layout === 'quote' ? 'The quote' : s.layout === 'code' ? 'The code' : s.layout === 'number' ? 'Where it comes from' : s.layout === 'two' ? 'Left column' : 'Text'}
        help={['bullets', 'two', 'image_left', 'image_right', 'blank'].includes(s.layout) ? <>Start lines with <code>-</code> for points, <code>1.</code> for numbers, two spaces in front for a point under a point. <code>**bold**</code>, <code>*italic*</code>.</> : undefined}>
        <textarea className={s.layout === 'code' ? 'mono' : ''} value={s.body} onChange={e => change({ body: e.target.value })} rows={s.layout === 'code' ? 10 : 6} spellCheck={s.layout !== 'code'} />
      </Field>}
      {has('body2') && <Field label="Right column"><textarea value={s.body2} onChange={e => change({ body2: e.target.value })} rows={6} /></Field>}
      {['bullets', 'two', 'image_left', 'image_right'].includes(s.layout) && (
        <label className="check-row"><Switch checked={!!s.data.build} onChange={v => setData({ build: v })} size="sm" /><span><b>Points one at a time</b><span className="hint-line block-line">Each press shows the next point.</span></span></label>
      )}
      {has('image') && <ImageField s={s} deckId={deckId} change={change} setData={setData} reload={reload} />}
      {has('chart') && <ChartField s={s} setData={setData} deckId={deckId} reload={reload} />}
      {has('table') && <TableField s={s} setData={setData} deckId={deckId} reload={reload} />}
      {has('code') && <Field label="Language" help="Shown small under the code."><input value={s.data.lang || ''} onChange={e => setData({ lang: e.target.value })} placeholder="python" /></Field>}
      {has('poll') && <PollField s={s} setData={setData} />}
      {has('qr') && <Field group label="Code to scan"><Segmented size="sm" value={s.data.qr || 'share'} onChange={v => setData({ qr: v })} options={[{ value: 'share', label: 'The slides' }, { value: 'live', label: 'Follow along' }, { value: 'none', label: 'None' }]} /></Field>}
      {s.layout === 'agenda' && <p className="hint-line">The agenda lists the titles of your section slides by itself.</p>}
      <Field group label="Colours"><Segmented size="sm" value={s.style} onChange={v => change({ style: v })} options={[{ value: '', label: 'Theme' }, { value: 'accent', label: 'Bright' }, { value: 'dark', label: 'Dark' }, { value: 'light', label: 'Light' }]} /></Field>
      <label className="check-row"><Switch checked={s.hidden} onChange={v => change({ hidden: v })} size="sm" /><span><b>Skip when presenting</b><span className="hint-line block-line">A backup slide: still in the deck, not shown.</span></span></label>
    </>
  )
}

function ImageField({ s, deckId, change, setData, reload }: { s: Slide; deckId: number; change: (p: Partial<Slide>) => void; setData: (d: Partial<SlideData>) => void; reload: () => Promise<void> }) {
  const { toast } = useToast()
  const appState = useAppState()
  const file = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [pick, setPick] = useState<'' | 'photos' | 'files'>('')
  const photos = appState('photos', 'photos:read'), files = appState('files', 'files:read')
  const up = async (f?: File) => {
    if (!f) return
    setBusy(true)
    try { const r = await uploadFile(f, deckId); change({ image_id: r.upload.id }) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Field group label="Picture">
      <div className="stack" style={{ gap: 8 }}>
        <div className="pic-pick">
          {s.image_id ? <img src={ownImage(s.image_id, true)} alt="" /> : <span className="muted small">No picture yet</span>}
          <span className="grow" />
          {busy && <span className="spinner" />}
        </div>
        <div className="row wrap">
          <button type="button" className="btn sm" onClick={() => file.current?.click()}><Icon name="upload" size={14} />Upload</button>
          <button type="button" className="btn sm" disabled={!photos.ok} title={photos.ok ? undefined : 'Connect Pocket Photos in Settings → Connected apps'} onClick={() => setPick('photos')}><Icon name="image" size={14} />Photos</button>
          <button type="button" className="btn sm" disabled={!files.ok} title={files.ok ? undefined : 'Connect Pocket Files in Settings → Connected apps'} onClick={() => setPick('files')}><Icon name="archive" size={14} />Files</button>
          {s.image_id && <button type="button" className="btn sm ghost" onClick={() => change({ image_id: null, no_image: true } as Partial<Slide>)}>Remove</button>}
        </div>
        {s.image_id && s.layout !== 'title' && <Segmented size="sm" value={s.data.fit || 'cover'} onChange={v => setData({ fit: v })} options={[{ value: 'cover', label: 'Fill the space' }, { value: 'contain', label: 'Show it whole' }]} />}
      </div>
      <input ref={file} type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden onChange={e => { up(e.target.files?.[0]); e.target.value = '' }} />
      {pick === 'photos' && <PhotoPicker multi={false} onClose={() => setPick('')} onPick={async ids => {
        try { await api.post(`${V1}/apps/photos/copy`, { deck_id: deckId, slide_id: s.id, ids }); await reload(); setPick('') } catch (e) { toast(errMsg(e), 'err') }
      }} />}
      {pick === 'files' && <FilePicker want="picture" title="Pick a picture" onClose={() => setPick('')} onPick={async (space, path) => {
        try { await api.post(`${V1}/apps/files/picture`, { deck_id: deckId, slide_id: s.id, space, path }); await reload(); setPick('') } catch (e) { toast(errMsg(e), 'err') }
      }} />}
    </Field>
  )
}

function ChartField({ s, setData, deckId, reload }: { s: Slide; setData: (d: Partial<SlideData>) => void; deckId: number; reload: () => Promise<void> }) {
  const appState = useAppState()
  const tables = appState('tables', 'tables:read')
  const [pick, setPick] = useState(false)
  const c = s.data.chart || { kind: 'bar' as const, labels: [], values: [], unit: '' }
  const set = (patch: Partial<typeof c>) => setData({ chart: { ...c, ...patch } })
  const rows = c.labels.map((l, i) => [l, c.values[i]] as const)
  const setRow = (i: number, l: string, v: number) => { const L = [...c.labels], V = [...c.values]; L[i] = l; V[i] = v; set({ labels: L, values: V }) }
  return (
    <Field group label="The numbers">
      <div className="stack" style={{ gap: 8 }}>
        <div className="chips">{(['bar', 'hbar', 'line', 'pie', 'donut'] as const).map(k => <button key={k} type="button" className={`chip ${c.kind === k ? 'on' : ''}`} onClick={() => set({ kind: k })}>{({ bar: 'Bars', hbar: 'Bars across', line: 'Line', pie: 'Pie', donut: 'Ring' })[k]}</button>)}</div>
        <div className="rows-edit">
          {rows.map(([l, v], i) => (
            <div key={i} className="row">
              <input value={l} onChange={e => setRow(i, e.target.value, v)} placeholder="Name" aria-label="Name" />
              <input type="number" value={Number.isFinite(v) ? v : ''} onChange={e => setRow(i, l, Number(e.target.value))} placeholder="0" aria-label="Number" style={{ maxWidth: 96 }} />
              <button type="button" className="btn sm ghost icon-only" aria-label="Remove" onClick={() => set({ labels: c.labels.filter((_, j) => j !== i), values: c.values.filter((_, j) => j !== i) })}><Icon name="x" size={14} /></button>
            </div>
          ))}
          <div className="row wrap">
            <button type="button" className="btn sm" disabled={c.labels.length >= 24} onClick={() => set({ labels: [...c.labels, ''], values: [...c.values, 0] })}><Icon name="plus" size={14} />A row</button>
            <button type="button" className="btn sm" disabled={!tables.ok} title={tables.ok ? undefined : 'Connect Pocket Tables in Settings → Connected apps'} onClick={() => setPick(true)}><Icon name="table" size={14} />From Pocket Tables</button>
          </div>
        </div>
        <Field label="Unit" help="€, $, %, kg… shown with the numbers."><input value={c.unit || ''} onChange={e => set({ unit: e.target.value })} style={{ maxWidth: 120 }} /></Field>
      </div>
      {pick && <TablePicker deckId={deckId} slideId={s.id} onDone={reload} onClose={() => setPick(false)} />}
    </Field>
  )
}

function TableField({ s, setData, deckId, reload }: { s: Slide; setData: (d: Partial<SlideData>) => void; deckId: number; reload: () => Promise<void> }) {
  const appState = useAppState()
  const tables = appState('tables', 'tables:read')
  const [pick, setPick] = useState(false)
  const rows = s.data.rows?.length ? s.data.rows : [['', ''], ['', '']]
  const cols = Math.max(...rows.map(r => r.length))
  const set = (r: string[][]) => setData({ rows: r })
  return (
    <Field group label="Rows" help="The first row is the headings.">
      <div className="stack" style={{ gap: 8 }}>
        <div className="cell-grid" style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}>
          {rows.map((r, i) => Array.from({ length: cols }, (_, j) => (
            <input key={`${i}-${j}`} value={r[j] || ''} style={i === 0 ? { fontWeight: 650 } : undefined} aria-label={`Row ${i + 1}, column ${j + 1}`}
              onChange={e => set(rows.map((rr, ii) => ii === i ? Array.from({ length: cols }, (_, jj) => jj === j ? e.target.value : rr[jj] || '') : rr))} />
          )))}
        </div>
        <div className="row wrap">
          <button type="button" className="btn sm" disabled={rows.length >= 16} onClick={() => set([...rows, Array(cols).fill('')])}><Icon name="plus" size={14} />Row</button>
          <button type="button" className="btn sm" disabled={cols >= 8} onClick={() => set(rows.map(r => [...r, '']))}><Icon name="plus" size={14} />Column</button>
          {rows.length > 1 && <button type="button" className="btn sm ghost" onClick={() => set(rows.slice(0, -1))}>Last row off</button>}
          {cols > 1 && <button type="button" className="btn sm ghost" onClick={() => set(rows.map(r => r.slice(0, cols - 1)))}>Last column off</button>}
        </div>
        <button type="button" className="btn sm" disabled={!tables.ok} title={tables.ok ? undefined : 'Connect Pocket Tables in Settings → Connected apps'} onClick={() => setPick(true)}><Icon name="table" size={14} />From Pocket Tables</button>
      </div>
      {pick && <TablePicker deckId={deckId} slideId={s.id} onDone={reload} onClose={() => setPick(false)} />}
    </Field>
  )
}

function PollField({ s, setData }: { s: Slide; setData: (d: Partial<SlideData>) => void }) {
  const opts = s.data.options?.length ? s.data.options : ['', '']
  return (
    <Field group label="Answers" help="People answer on their phones with the follow-along code; the results fill in live.">
      <div className="rows-edit">
        {opts.map((o, i) => (
          <div key={i} className="row">
            <span className="muted small" style={{ width: 16 }}>{String.fromCharCode(65 + i)}</span>
            <input value={o} onChange={e => setData({ options: opts.map((x, j) => j === i ? e.target.value : x) })} placeholder="An answer" />
            <button type="button" className="btn sm ghost icon-only" aria-label="Remove" disabled={opts.length < 3} onClick={() => setData({ options: opts.filter((_, j) => j !== i), correct: 0 })}><Icon name="x" size={14} /></button>
          </div>
        ))}
        <button type="button" className="btn sm" disabled={opts.length >= 8} onClick={() => setData({ options: [...opts, ''] })}><Icon name="plus" size={14} />An answer</button>
        <Field group label="A quiz? The right answer" help="Shown with the next press after people answered.">
          <Segmented size="sm" value={s.data.correct || 0} onChange={v => setData({ correct: v })} options={[{ value: 0, label: 'None' }, ...opts.map((_, i) => ({ value: i + 1, label: String.fromCharCode(65 + i) }))]} />
        </Field>
      </div>
    </Field>
  )
}

function AIHelp({ slide, look, p, onUse }: { slide: Slide; look: Look; p: DeckPayload; onUse: (patch: Partial<Slide>) => void }) {
  const { toast } = useToast()
  const [busy, setBusy] = useState('')
  const [sugg, setSugg] = useState<Partial<Slide> | null>(null)
  const ask = async (want: string) => {
    setBusy(want)
    try { const r = await api.post<{ suggestion: Partial<Slide> }>(`${V1}/ai/slide`, { slide_id: slide.id, want }); setSugg(r.suggestion) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const merged = sugg ? { ...slide, ...sugg, data: { ...slide.data, ...(sugg.data || {}) } } as Slide : null
  return (
    <>
      <MenuButton label="AI help" className="btn sm" width={250} align="right" items={[
        { label: 'Make it shorter', icon: 'minus', onSelect: () => ask('shorter') },
        { label: 'Make it clearer', icon: 'sparkles', onSelect: () => ask('clearer') },
        { label: 'Write the speaker notes', icon: 'note', onSelect: () => ask('notes') },
        { label: 'Turn the text into points', icon: 'list', onSelect: () => ask('points') },
        { label: 'A better heading', icon: 'heading', onSelect: () => ask('title') },
        { label: 'A better layout', icon: 'layout', onSelect: () => ask('ideas') },
      ]}>{busy ? <span className="spinner" /> : <Icon name="sparkles" size={14} />}AI</MenuButton>
      {merged && <Modal title="A suggestion" onClose={() => setSugg(null)} size="lg"
        footer={<><button type="button" className="btn" onClick={() => setSugg(null)}>Keep mine</button><button type="button" className="btn primary" onClick={() => { onUse({ layout: merged.layout, title: merged.title, subtitle: merged.subtitle, body: merged.body, body2: merged.body2, notes: merged.notes, data: merged.data }); setSugg(null) }}>Use it</button></>}>
        <div className="stack">
          <SlideView slide={merged} deck={p.deck} look={look} imageUrl={ownImage} qr={{ share: abs(p.share_url), live: origin() + '/w/' + p.deck.live_key }} />
          {merged.notes && <div><h4 className="muted small">Speaker notes</h4><p className="pre small">{merged.notes}</p></div>}
        </div>
      </Modal>}
    </>
  )
}

