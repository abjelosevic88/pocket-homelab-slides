// Your own slide layouts (slides kept to start new ones from) and themes
// (colours, fonts and a logo), for every deck.
import { useEffect, useMemo, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { MyLayout, Slide, Theme } from '../lib/types'
import { FONTS, THEMES, builtinLook, lookOf } from '../lib/slides'
import { ownImage } from '../lib/deck'
import { uploadFile } from '../lib/upload'
import { Card, Confirm, Empty, Field, Loading, Modal, errMsg, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { SlideView } from '../components/SlideView'

const SAMPLE: Slide[] = [
  { id: -1, deck_id: 0, position: 0, layout: 'title', title: 'The title of your talk', subtitle: 'Your name · the event', body: '', body2: '', image_id: null, data: {}, notes: '', style: '', hidden: false, seconds: 0, source: '', created_at: '', updated_at: '' },
  { id: -2, deck_id: 0, position: 1, layout: 'bullets', title: 'Three things to know', subtitle: '', body: '- The first point\n- The second, with **a bold word**\n  - and a point under it\n- The third', body2: '', image_id: null, data: {}, notes: '', style: '', hidden: false, seconds: 0, source: '', created_at: '', updated_at: '' },
  { id: -3, deck_id: 0, position: 2, layout: 'chart', title: 'Growing', subtitle: '', body: '', body2: '', image_id: null, data: { chart: { kind: 'bar', labels: ['2023', '2024', '2025', '2026'], values: [12, 30, 52, 70] } }, notes: '', style: '', hidden: false, seconds: 0, source: '', created_at: '', updated_at: '' },
]
const sampleDeck = { aspect: '16:9' as const, footer: 'Your footer', numbers: true, title: 'Sample', theme: 'paper' }

export default function Library() {
  const { toast } = useToast()
  const [layouts, setLayouts] = useState<MyLayout[] | null>(null)
  const [themes, setThemes] = useState<Theme[] | null>(null)
  const [edit, setEdit] = useState<Partial<Theme> | null>(null)
  const [del, setDel] = useState<{ kind: 'layout' | 'theme'; id: number; name: string } | null>(null)
  const load = () => {
    api.get<{ layouts: MyLayout[] }>(`${V1}/layouts`).then(r => setLayouts(r.layouts)).catch(() => setLayouts([]))
    api.get<{ themes: Theme[] }>(`${V1}/themes`).then(r => setThemes(r.themes)).catch(() => setThemes([]))
  }
  useEffect(load, [])
  const rename = async (l: MyLayout) => {
    const name = window.prompt('A new name', l.name)
    if (!name?.trim()) return
    try { await api.put(`${V1}/layouts/${l.id}`, { name }); load() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <div className="page">
      <div className="page-head"><div className="grow"><h2 className="page-title">Layouts and themes</h2><p className="muted small">Yours, for every deck. The built-in layouts and themes are always there too.</p></div></div>
      <Card title="My themes" sub="Your colours, fonts and logo, on top of a built-in theme." actions={<button type="button" className="btn sm primary" onClick={() => setEdit({ base: 'paper', name: 'My theme' })}><Icon name="plus" size={14} />New theme</button>}>
        <div className="pad">
          {!themes ? <Loading /> : themes.length === 0 ? <p className="muted small">None yet. Make one with your brand's colours, or your school's.</p> : (
            <div className="deck-grid">{themes.map(t => (
              <div key={t.id} className="deck-card" role="group">
                <button type="button" className="deck-card" onClick={() => setEdit(t)}><SlideView slide={SAMPLE[0]} deck={sampleDeck} look={lookOf({ theme: t.base }, t)} imageUrl={ownImage} thumb /></button>
                <div className="deck-card-meta"><div className="grow deck-card-title">{t.name}</div><button type="button" className="btn sm ghost icon-only" aria-label="Delete" onClick={() => setDel({ kind: 'theme', id: t.id, name: t.name })}><Icon name="trash" size={14} /></button></div>
              </div>
            ))}</div>
          )}
        </div>
      </Card>
      <Card title="My layouts" sub="Slides you kept (in the editor: ⋯ → Keep as my layout). New slides can start from them, in any deck.">
        <div className="pad">
          {!layouts ? <Loading /> : layouts.length === 0 ? <Empty title="No layouts of your own yet" icon="bookmark">Make a slide just the way you like it, then keep it as a layout from its ⋯ menu.</Empty> : (
            <div className="deck-grid">{layouts.map(l => (
              <div key={l.id} className="deck-card" role="group">
                <SlideView slide={{ ...SAMPLE[1], ...l, id: -10 - l.id, image_id: null, hidden: false, seconds: 0, source: '', deck_id: 0, position: 0, updated_at: '' }} deck={sampleDeck} look={builtinLook('paper')} imageUrl={ownImage} thumb />
                <div className="deck-card-meta"><div className="grow"><div className="deck-card-title">{l.name}</div><div className="deck-card-sub">{l.layout.replace('_', ' ')}</div></div>
                  <button type="button" className="btn sm ghost icon-only" aria-label="Rename" onClick={() => rename(l)}><Icon name="edit" size={14} /></button>
                  <button type="button" className="btn sm ghost icon-only" aria-label="Delete" onClick={() => setDel({ kind: 'layout', id: l.id, name: l.name })}><Icon name="trash" size={14} /></button></div>
              </div>
            ))}</div>
          )}
        </div>
      </Card>
      {edit && <ThemeEditor t={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); toast('Theme saved') }} />}
      {del && <Confirm title={`Delete “${del.name}”?`} message={del.kind === 'theme' ? 'Decks using it go back to its built-in theme.' : 'Slides made from it stay as they are.'} confirmLabel="Delete"
        onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/${del.kind === 'theme' ? 'themes' : 'layouts'}/${del.id}`); setDel(null); load() }} />}
    </div>
  )
}

function ThemeEditor({ t: start, onClose, onSaved }: { t: Partial<Theme>; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [t, setT] = useState<Partial<Theme>>(start)
  const [busy, setBusy] = useState(false)
  const file = useRef<HTMLInputElement>(null)
  const b = builtinLook(t.base || 'paper')
  const look = useMemo(() => lookOf({ theme: t.base || 'paper' }, { ...({} as Theme), ...t, logo_id: t.logo_id ?? null } as Theme), [t])
  const color = (k: 'bg' | 'bg2' | 'fg' | 'muted' | 'accent', label: string, fallback: string) => (
    <label className="color-field"><input type="color" value={t[k] || fallback} onChange={e => setT({ ...t, [k]: e.target.value })} /><span>{label}</span>{t[k] && <button type="button" className="linkish small" onClick={() => setT({ ...t, [k]: '' })}>reset</button>}</label>
  )
  const save = async () => {
    setBusy(true)
    try { await (t.id ? api.put(`${V1}/themes/${t.id}`, t) : api.post(`${V1}/themes`, t)); onSaved() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={t.id ? t.name : 'New theme'} onClose={onClose} size="lg" footer={<button type="button" className="btn primary" onClick={save} disabled={busy}>{busy && <span className="spinner" />}Save</button>}>
      <div className="theme-editor">
        <div className="stack">
          <Field label="Name"><input value={t.name || ''} onChange={e => setT({ ...t, name: e.target.value })} /></Field>
          <Field group label="Starts from">
            <div className="chips">{THEMES.map(x => <button key={x.key} type="button" className={`chip ${t.base === x.key ? 'on' : ''}`} onClick={() => setT({ ...t, base: x.key })}>{x.name}</button>)}</div>
          </Field>
          <Field group label="Colours">
            <div className="color-grid">{color('bg', 'Background', b.bg)}{color('bg2', 'Second background', b.bg2)}{color('fg', 'Text', b.fg)}{color('muted', 'Quiet text', b.muted)}{color('accent', 'Accent', b.accent)}</div>
          </Field>
          <Field group label="Headings">
            <div className="chips">{Object.entries(FONTS).map(([k, f]) => <button key={k} type="button" className={`chip ${(t.head_font || b.head) === k ? 'on' : ''}`} style={{ fontFamily: f.css }} onClick={() => setT({ ...t, head_font: k })}>{f.label}</button>)}</div>
          </Field>
          <Field group label="Text">
            <div className="chips">{Object.entries(FONTS).map(([k, f]) => <button key={k} type="button" className={`chip ${(t.body_font || b.body) === k ? 'on' : ''}`} style={{ fontFamily: f.css }} onClick={() => setT({ ...t, body_font: k })}>{f.label}</button>)}</div>
          </Field>
          <Field group label="Logo" help="In the top corner of every slide but the full pictures.">
            <div className="row wrap">
              {t.logo_id ? <img src={ownImage(t.logo_id)} alt="" style={{ height: 32, maxWidth: 120, objectFit: 'contain' }} /> : null}
              <button type="button" className="btn sm" onClick={() => file.current?.click()}><Icon name="upload" size={14} />{t.logo_id ? 'Another' : 'Upload'}</button>
              {t.logo_id && <button type="button" className="btn sm ghost" onClick={() => setT({ ...t, logo_id: null })}>Remove</button>}
            </div>
            <input ref={file} type="file" accept="image/png,image/jpeg,image/webp" hidden onChange={async e => { const f = e.target.files?.[0]; e.target.value = ''; if (!f) return; try { const r = await uploadFile(f, 0); setT(x => ({ ...x, logo_id: r.upload.id })) } catch (er) { toast(errMsg(er), 'err') } }} />
          </Field>
        </div>
        <div className="stack">{SAMPLE.map(s => <SlideView key={s.id} slide={s} deck={sampleDeck} look={look} imageUrl={ownImage} index={s.position} total={3} />)}</div>
      </div>
    </Modal>
  )
}
