// One slide, drawn on a fixed canvas (1920×1080, or 1440×1080 for 4:3) and
// scaled as a whole to the width of its box — so a slide looks the same on
// a phone, in the editor, on the projector and in the PDF.
import React, { useCallback, useRef, useState } from 'react'
import type { Deck, Slide } from '../lib/types'
import { FONTS, canvasOf, inline, parseBody, slideColors, type Line, type Look } from '../lib/slides'
import { SlideChart, mix } from './SlideChart'
import { QRCode } from './QRCode'

export function useWidth(fallback: number) {
  const [w, setW] = useState(fallback)
  const ro = useRef<ResizeObserver | null>(null)
  const ref = useCallback((el: Element | null) => {
    ro.current?.disconnect(); ro.current = null
    if (!el) return
    const measure = () => { const cw = Math.round(el.getBoundingClientRect().width); if (cw > 0) setW(cw) }
    measure(); ro.current = new ResizeObserver(measure); ro.current.observe(el)
  }, [])
  return [ref, w] as const
}

export interface SlideViewProps {
  slide: Slide | null
  deck: Pick<Deck, 'aspect' | 'footer' | 'numbers' | 'title'>
  look: Look
  index?: number
  total?: number
  /** top-level points shown so far on a step-by-step slide (undefined: all) */
  step?: number
  imageUrl: (id: number, thumb?: boolean) => string
  agenda?: string[]
  tally?: Record<string, number>
  showAnswer?: boolean
  qr?: { share?: string; live?: string }
  follow?: { url: string; code: string }
  thumb?: boolean
  placeholders?: boolean
  className?: string
  /** draw at this width without measuring (print) */
  width?: number
}

export function SlideView(p: SlideViewProps) {
  const [ref, measured] = useWidth(p.width || 640)
  const W = p.width || measured
  const { w, h } = canvasOf(p.deck.aspect)
  const k = W / w
  return (
    <div ref={p.width ? undefined : ref} className={`sv-box ${p.className || ''}`} style={{ height: Math.round(W * h / w) }}>
      <div className="sv-scale" style={{ width: w, height: h, transform: `scale(${k})` }}>
        <Canvas {...p} />
      </div>
    </div>
  )
}

function Canvas(p: SlideViewProps) {
  const s = p.slide
  const c = slideColors(p.look, s?.style || '')
  const vars = {
    '--sv-bg': c.bg, '--sv-bg2': c.bg2, '--sv-fg': c.fg, '--sv-muted': c.muted, '--sv-accent': c.accent,
    '--sv-soft': mix(c.bg, c.accent, 0.16), '--sv-line': mix(c.bg, c.fg, 0.16), '--sv-panel': mix(c.bg, c.fg, c.dark ? 0.09 : 0.05),
    '--sv-head': (FONTS[p.look.head] || FONTS.sans).css, '--sv-body': (FONTS[p.look.body] || FONTS.sans).css,
  } as React.CSSProperties
  if (!s) return <div className="sv" style={vars} />
  const ph = (v: string, word: string) => v || (p.placeholders ? <span className="sv-ph">{word}</span> : null)
  const img = s.image_id ? p.imageUrl(s.image_id, p.thumb) : ''
  const fit = s.data?.fit === 'contain' ? 'contain' : 'cover'
  const step = s.data?.build ? p.step : undefined
  const n1 = topCount(s.body)
  const body = (text: string, offset = 0, big = false) => <Points text={text} step={step === undefined ? undefined : step - offset} big={big} />
  let inner: React.ReactNode
  switch (s.layout) {
    case 'title':
      inner = <>
        {img && <div className="sv-bgimg" style={{ backgroundImage: `url("${img}")` }}><div className="sv-shade" /></div>}
        <div className={`sv-center sv-title-wrap ${img ? 'on-img' : ''}`}>
          <h1 className="sv-h-title">{ph(s.title, 'Your title')}</h1>
          {(s.subtitle || p.placeholders) && <p className="sv-sub">{ph(s.subtitle, 'A line under it')}</p>}
        </div>
      </>
      break
    case 'section':
      inner = <div className="sv-section">
        {s.subtitle && <p className="sv-kicker">{s.subtitle}</p>}
        <h1 className="sv-h-section">{ph(s.title, 'A new part')}</h1>
        <span className="sv-bar" />
      </div>
      break
    case 'two':
      inner = <div className="sv-pad sv-col">
        <h2 className="sv-h">{ph(s.title, 'Heading')}</h2>
        {s.subtitle && <p className="sv-sub sm">{s.subtitle}</p>}
        <div className="sv-two">
          <div className="sv-cell">{s.data?.left && <h3 className="sv-colhead">{s.data.left}</h3>}{body(s.body)}{!s.body && p.placeholders && <span className="sv-ph sv-text">Points on the left</span>}</div>
          <div className="sv-cell">{s.data?.right && <h3 className="sv-colhead">{s.data.right}</h3>}{body(s.body2, n1)}{!s.body2 && p.placeholders && <span className="sv-ph sv-text">Points on the right</span>}</div>
        </div>
      </div>
      break
    case 'image_left':
    case 'image_right':
      inner = <div className={`sv-split ${s.layout === 'image_left' ? 'rev' : ''}`}>
        <div className="sv-pad sv-col sv-half">
          <h2 className="sv-h">{ph(s.title, 'Heading')}</h2>
          {body(s.body)}{!s.body && p.placeholders && <span className="sv-ph sv-text">A few points</span>}
        </div>
        <div className="sv-pic" style={img ? { backgroundImage: `url("${img}")`, backgroundSize: fit } : undefined}>{!img && <span className="sv-ph sv-ph-pic">Picture</span>}</div>
      </div>
      break
    case 'image':
      inner = <>
        <div className="sv-full" style={img ? { backgroundImage: `url("${img}")`, backgroundSize: fit } : undefined}>{!img && <span className="sv-ph sv-ph-pic">Picture</span>}</div>
        {(s.title || s.subtitle) && <div className="sv-caption"><h2>{s.title}</h2>{s.subtitle && <p>{s.subtitle}</p>}</div>}
      </>
      break
    case 'quote':
      inner = <div className="sv-center sv-quote">
        <span className="sv-qmark" aria-hidden>“</span>
        <blockquote>{s.body ? s.body.split('\n').map((l, i) => <span key={i}>{spans(l)}<br /></span>) : ph('', 'Somebody\'s words')}</blockquote>
        {(s.subtitle || p.placeholders) && <p className="sv-who">— {ph(s.subtitle, 'who said it')}</p>}
      </div>
      break
    case 'number':
      inner = <div className="sv-center sv-number">
        <div className="sv-big">{ph(s.title, '42')}</div>
        {(s.subtitle || p.placeholders) && <p className="sv-big-sub">{ph(s.subtitle, 'what it means')}</p>}
        {s.body && <p className="sv-big-note">{spans(s.body)}</p>}
      </div>
      break
    case 'chart':
      inner = <div className="sv-pad sv-col">
        <h2 className="sv-h">{ph(s.title, 'Chart')}</h2>
        {s.subtitle && <p className="sv-sub sm">{s.subtitle}</p>}
        <div className="sv-grow">{s.data?.chart && s.data.chart.labels.length ? <SlideChart chart={s.data.chart} colors={c} w={canvasOf(p.deck.aspect).w - 220} h={s.subtitle ? 640 : 700} /> : <span className="sv-ph sv-text">Add a few numbers</span>}</div>
        {s.data?.from && <p className="sv-source">{s.data.from}</p>}
      </div>
      break
    case 'table':
      inner = <div className="sv-pad sv-col">
        <h2 className="sv-h">{ph(s.title, 'Table')}</h2>
        <div className="sv-grow sv-table-wrap">{s.data?.rows?.length ? <SlideTable rows={s.data.rows} /> : <span className="sv-ph sv-text">Add rows</span>}</div>
        {s.data?.from && <p className="sv-source">{s.data.from}</p>}
      </div>
      break
    case 'code':
      inner = <div className="sv-pad sv-col">
        {(s.title || p.placeholders) && <h2 className="sv-h">{ph(s.title, 'Code')}</h2>}
        <pre className={`sv-code ${codeSize(s.body)}`}>{s.body || (p.placeholders ? '// your code' : '')}</pre>
        {s.data?.lang && <p className="sv-source">{s.data.lang}</p>}
      </div>
      break
    case 'poll':
      inner = <Poll s={s} tally={p.tally} showAnswer={!!p.showAnswer} placeholders={p.placeholders} />
      break
    case 'agenda':
      inner = <div className="sv-pad sv-col">
        <h2 className="sv-h">{ph(s.title, 'Agenda')}</h2>
        <ol className="sv-agenda">{(p.agenda?.length ? p.agenda : p.placeholders ? ['The section slides', 'show up here'] : []).map((a, i) => <li key={i}><span className="sv-agenda-n">{i + 1}</span>{a}</li>)}</ol>
      </div>
      break
    case 'end': {
      const q = s.data?.qr === 'live' ? p.qr?.live : s.data?.qr === 'none' ? '' : p.qr?.share
      inner = <div className={`sv-center sv-end ${q ? 'with-qr' : ''}`}>
        <div className="sv-end-words">
          <h1 className="sv-h-title">{ph(s.title, 'Thank you!')}</h1>
          {(s.subtitle || p.placeholders) && <p className="sv-sub">{ph(s.subtitle, 'Questions?')}</p>}
        </div>
        {q && <div className="sv-qr"><QRCode value={q} size={300} label="Scan for the slides" /><span>{s.data?.qr === 'live' ? 'Follow along' : 'The slides'}</span></div>}
      </div>
      break
    }
    case 'blank':
      inner = <div className="sv-center sv-blank">{s.body ? <Points text={s.body} big step={step} /> : ph('', 'A few words')}</div>
      break
    default:
      inner = <div className="sv-pad sv-col">
        <h2 className="sv-h">{ph(s.title, 'Heading')}</h2>
        {s.subtitle && <p className="sv-sub sm">{s.subtitle}</p>}
        {body(s.body)}{!s.body && p.placeholders && <span className="sv-ph sv-text">A few points</span>}
      </div>
  }
  const full = s.layout === 'image' || (s.layout === 'title' && !!img)
  return (
    <div className={`sv sv-${s.layout} ${c.dark ? 'is-dark' : ''} ${p.thumb ? 'is-thumb' : ''}`} style={vars}>
      {inner}
      {p.look.logo && !full && <img className="sv-logo" src={p.imageUrl(p.look.logo)} alt="" />}
      {(p.deck.footer || (p.deck.numbers && p.index !== undefined && p.index > 0)) && !full && s.layout !== 'title' && (
        <div className="sv-foot"><span>{p.deck.footer}</span>{p.deck.numbers && p.index !== undefined && <span className="sv-num">{p.index + 1}{p.total ? ` / ${p.total}` : ''}</span>}</div>
      )}
      {p.follow && <div className="sv-follow"><b>Follow along</b><span>{p.follow.url}</span><span className="sv-code-word">{p.follow.code.toUpperCase()}</span></div>}
    </div>
  )
}

function topCount(text: string) { return parseBody(text).filter(l => l.level === 0 && (l.kind === 'point' || l.kind === 'num')).length }

function spans(text: string): React.ReactNode {
  return inline(text).map((x, i) => x.b ? <b key={i}>{x.t}</b> : x.i ? <i key={i}>{x.t}</i> : x.c ? <code key={i}>{x.t}</code> : x.a ? <u key={i}>{x.t}</u> : <React.Fragment key={i}>{x.t}</React.Fragment>)
}

/** A text size that fits: fewer, shorter lines are bigger. */
function sizeOf(lines: Line[], big: boolean): string {
  const n = lines.filter(l => l.kind !== 'gap').length
  const chars = lines.reduce((a, l) => a + l.text.length, 0)
  if (big) return chars > 160 ? 'fs-3' : chars > 70 ? 'fs-2' : 'fs-big'
  if (n > 9 || chars > 520) return 'fs-4'
  if (n > 6 || chars > 340) return 'fs-3'
  if (n > 4 || chars > 200) return 'fs-2'
  return 'fs-1'
}

function Points({ text, step, big }: { text: string; step?: number; big?: boolean }) {
  const lines = parseBody(text)
  if (!lines.length) return null
  return (
    <div className={`sv-points ${sizeOf(lines, !!big)} ${big ? 'big' : ''}`}>
      {lines.map((l, i) => {
        const hidden = step !== undefined && l.top >= step && (l.kind !== 'text' || l.top >= 0) && hasPointsBefore(lines, i)
        if (l.kind === 'gap') return <div key={i} className="sv-gap" />
        return (
          <div key={i} className={`sv-line lvl-${l.level} k-${l.kind} ${hidden ? 'is-hidden' : ''}`}>
            {l.kind === 'point' && <span className="sv-dot" />}
            {l.kind === 'num' && <span className="sv-n">{l.n}.</span>}
            <span className="sv-t">{spans(l.text)}</span>
          </div>
        )
      })}
    </div>
  )
}

/** Text before the first point is always shown; after it, it goes with its point. */
function hasPointsBefore(lines: Line[], i: number) {
  for (let j = 0; j <= i; j++) if (lines[j].kind === 'point' || lines[j].kind === 'num') return true
  return false
}

function codeSize(code: string) {
  const lines = code.split('\n')
  const longest = Math.max(0, ...lines.map(l => l.length))
  if (lines.length > 18 || longest > 80) return 'cs-4'
  if (lines.length > 12 || longest > 60) return 'cs-3'
  if (lines.length > 7 || longest > 44) return 'cs-2'
  return 'cs-1'
}

function SlideTable({ rows }: { rows: string[][] }) {
  const [head, ...rest] = rows
  const small = rows.length > 8 || (head?.length || 0) > 5
  return (
    <table className={`sv-table ${small ? 'small' : ''}`}>
      <thead><tr>{head?.map((c, i) => <th key={i}>{c}</th>)}</tr></thead>
      <tbody>{rest.map((r, i) => <tr key={i}>{r.map((c, j) => <td key={j} className={/^[-+€$]?[\d.,]+\s?%?$/.test(c.trim()) ? 'num' : ''}>{c}</td>)}</tr>)}</tbody>
    </table>
  )
}

function Poll({ s, tally, showAnswer, placeholders }: { s: Slide; tally?: Record<string, number>; showAnswer: boolean; placeholders?: boolean }) {
  const opts = s.data?.options?.length ? s.data.options : placeholders ? ['An answer', 'Another answer'] : []
  const counts = opts.map((_, i) => tally?.[String(i)] || 0)
  const total = counts.reduce((a, b) => a + b, 0)
  const correct = showAnswer ? (s.data?.correct || 0) : 0
  return (
    <div className="sv-pad sv-col">
      <h2 className="sv-h">{s.title || (placeholders ? <span className="sv-ph">Your question</span> : '')}</h2>
      <div className="sv-poll">
        {opts.map((o, i) => {
          const pct = total ? Math.round(counts[i] * 100 / total) : 0
          return (
            <div key={i} className={`sv-opt ${correct === i + 1 ? 'is-right' : correct ? 'is-wrong' : ''}`}>
              <span className="sv-opt-fill" style={{ width: tally ? `${pct}%` : 0 }} />
              <span className="sv-opt-letter">{String.fromCharCode(65 + i)}</span>
              <span className="sv-opt-text">{o}</span>
              {tally && <span className="sv-opt-n">{pct}%</span>}
            </div>
          )
        })}
      </div>
      <p className="sv-source">{tally ? `${total} ${total === 1 ? 'answer' : 'answers'}` : 'Answer on your phone'}</p>
    </div>
  )
}
