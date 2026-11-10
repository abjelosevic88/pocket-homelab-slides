// Themes, the slide canvas and the short Markdown slides are written in.
import type { Deck, Slide, Style, Theme } from './types'

export const FONTS: Record<string, { label: string; css: string }> = {
  sans: { label: 'Clean', css: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif' },
  serif: { label: 'Classic', css: '"Iowan Old Style", "Palatino Linotype", Palatino, Georgia, "Times New Roman", serif' },
  rounded: { label: 'Friendly', css: 'ui-rounded, "SF Pro Rounded", "Nunito", "Quicksand", "Varela Round", system-ui, sans-serif' },
  mono: { label: 'Typewriter', css: 'ui-monospace, "SF Mono", "JetBrains Mono", Menlo, Consolas, monospace' },
  slab: { label: 'Poster', css: '"Rockwell", "Roboto Slab", "Zilla Slab", "Courier New", Georgia, serif' },
  condensed: { label: 'Tall', css: '"Avenir Next Condensed", "Arial Narrow", "Roboto Condensed", "Helvetica Neue", system-ui, sans-serif' },
}

export interface Look { key: string; name: string; dark: boolean; bg: string; bg2: string; fg: string; muted: string; accent: string; head: string; body: string; logo?: number | null }

export const THEMES: Look[] = [
  { key: 'paper', name: 'Paper', dark: false, bg: '#fbfaf7', bg2: '#f1ece2', fg: '#1f1c18', muted: '#716a5f', accent: '#c27c0e', head: 'serif', body: 'sans' },
  { key: 'night', name: 'Night', dark: true, bg: '#0f172a', bg2: '#1e293b', fg: '#f1f5f9', muted: '#94a3b8', accent: '#f0b341', head: 'sans', body: 'sans' },
  { key: 'chalk', name: 'Chalkboard', dark: true, bg: '#213e30', bg2: '#2c5140', fg: '#f4f1e8', muted: '#b9c9b8', accent: '#f5d76e', head: 'rounded', body: 'rounded' },
  { key: 'sunrise', name: 'Sunrise', dark: false, bg: '#fff5ec', bg2: '#ffe0cf', fg: '#3b1f14', muted: '#8a5a48', accent: '#d9482f', head: 'slab', body: 'sans' },
  { key: 'studio', name: 'Studio', dark: true, bg: '#0b0b0c', bg2: '#1c1c20', fg: '#ffffff', muted: '#a1a1aa', accent: '#ff5a4f', head: 'condensed', body: 'sans' },
  { key: 'ocean', name: 'Ocean', dark: true, bg: '#0b3b4a', bg2: '#11606f', fg: '#f0fbfc', muted: '#a7d3da', accent: '#5ee0d0', head: 'sans', body: 'sans' },
  { key: 'forest', name: 'Forest', dark: false, bg: '#f4f7f2', bg2: '#e1ebdc', fg: '#1b2a1f', muted: '#5b6e5f', accent: '#2f7d4f', head: 'serif', body: 'serif' },
  { key: 'mono', name: 'Plain', dark: false, bg: '#ffffff', bg2: '#ffffff', fg: '#111111', muted: '#666666', accent: '#111111', head: 'sans', body: 'sans' },
  { key: 'candy', name: 'Candy', dark: false, bg: '#fff5fb', bg2: '#efe6ff', fg: '#2b1736', muted: '#7a5f86', accent: '#c2378a', head: 'rounded', body: 'rounded' },
  { key: 'slate', name: 'Slate', dark: true, bg: '#1c2430', bg2: '#263241', fg: '#e8edf3', muted: '#9aa8b8', accent: '#8ab4ff', head: 'mono', body: 'sans' },
]

export function builtinLook(key: string): Look { return THEMES.find(t => t.key === key) || THEMES[0] }

/** The look of a deck: its built-in theme, or one of the person's own on top of one. */
export function lookOf(deck: Pick<Deck, 'theme'>, own?: Theme | null): Look {
  const b = builtinLook(own?.base || deck.theme)
  if (!own) return b
  const bg = own.bg || b.bg
  return { key: 'own', name: own.name, dark: luminance(bg) < 0.4, bg, bg2: own.bg2 || own.bg || b.bg2, fg: own.fg || b.fg, muted: own.muted || b.muted, accent: own.accent || b.accent,
    head: own.head_font || b.head, body: own.body_font || b.body, logo: own.logo_id }
}

export function luminance(hex: string): number {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex)
  if (!m) return 1
  const n = parseInt(m[1], 16)
  const c = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map(v => { const x = v / 255; return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4) })
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]
}

/** The colours one slide uses (a slide can be on the accent colour, dark or light). */
export function slideColors(look: Look, style: Style) {
  switch (style) {
    case 'accent': {
      const darkText = luminance(look.accent) > 0.45
      return { bg: look.accent, bg2: look.accent, fg: darkText ? '#141414' : '#ffffff', muted: darkText ? 'rgba(0,0,0,.62)' : 'rgba(255,255,255,.78)', accent: darkText ? '#141414' : '#ffffff', dark: !darkText }
    }
    case 'dark': return { bg: '#111214', bg2: '#1b1c20', fg: '#f6f6f6', muted: '#a3a3a3', accent: look.dark ? look.accent : (luminance(look.accent) < 0.2 ? '#ffffff' : look.accent), dark: true }
    case 'light': return { bg: '#ffffff', bg2: '#f6f6f4', fg: '#141414', muted: '#666666', accent: look.dark && luminance(look.accent) > 0.5 ? '#333333' : look.accent, dark: false }
  }
  return { bg: look.bg, bg2: look.bg2, fg: look.fg, muted: look.muted, accent: look.accent, dark: look.dark }
}

/** The canvas every slide is drawn on, then scaled to the box. */
export function canvasOf(aspect: string): { w: number; h: number } { return aspect === '4:3' ? { w: 1440, h: 1080 } : { w: 1920, h: 1080 } }

// ---- the short Markdown of slide text ----

export interface Line { kind: 'point' | 'num' | 'text' | 'gap'; level: number; text: string; n?: number; top: number }

/** Lines of a slide body. top = which top-level point it belongs to (for step-by-step slides). */
export function parseBody(body: string): Line[] {
  const out: Line[] = []
  let top = -1
  for (const raw of (body || '').split('\n')) {
    const indent = raw.length - raw.trimStart().length
    const t = raw.trim()
    if (!t) { out.push({ kind: 'gap', level: 0, text: '', top }); continue }
    const m = /^([-*•])\s+(.*)$/.exec(t)
    const nm = /^(\d+)\.\s+(.*)$/.exec(t)
    const level = Math.min(3, Math.floor(indent / 2))
    if (m || nm) {
      if (level === 0) top++
      out.push({ kind: m ? 'point' : 'num', level, text: m ? m[2] : nm![2], n: nm ? Number(nm[1]) : undefined, top: Math.max(top, 0) })
    } else {
      out.push({ kind: 'text', level, text: t, top: Math.max(top, 0) })
    }
  }
  while (out.length && out[out.length - 1].kind === 'gap') out.pop()
  return out
}

export interface Span { t: string; b?: boolean; i?: boolean; c?: boolean; a?: string }

/** **bold**, *italic*, `code` and [links](https://…) inside a line. */
export function inline(text: string): Span[] {
  const out: Span[] = []
  const re = /(\*\*[^*]+\*\*|__[^_]+__|\*[^*\s][^*]*\*|_[^_\s][^_]*_|`[^`]+`|\[[^\]]+\]\([^)]+\))/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(text))) {
    if (m.index > last) out.push({ t: text.slice(last, m.index) })
    const s = m[0]
    if (s.startsWith('**') || s.startsWith('__')) out.push({ t: s.slice(2, -2), b: true })
    else if (s.startsWith('`')) out.push({ t: s.slice(1, -1), c: true })
    else if (s.startsWith('[')) { const mm = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(s)!; out.push({ t: mm[1], a: mm[2] }) }
    else out.push({ t: s.slice(1, -1), i: true })
    last = m.index + s.length
  }
  if (last < text.length) out.push({ t: text.slice(last) })
  return out
}

/** How many presses a slide takes before the next one (step-by-step points, a quiz answer). */
export function stepsOf(s: Slide): number {
  if (s.layout === 'poll' && (s.data?.correct || 0) > 0) return 1
  if (!s.data?.build) return 0
  if (!['bullets', 'image_left', 'image_right', 'two'].includes(s.layout)) return 0
  const count = (b: string) => parseBody(b).filter(l => l.level === 0 && (l.kind === 'point' || l.kind === 'num')).length
  return count(s.body) + count(s.body2)
}

export const LAYOUT_ICONS: Record<string, string> = {
  title: 'presentation', section: 'bookmark', bullets: 'list', two: 'columns', image_right: 'image', image_left: 'image', image: 'maximize', quote: 'quote', number: 'hash',
  chart: 'chart', table: 'table', code: 'code', poll: 'poll', agenda: 'list-ordered', end: 'flag', blank: 'text',
}

export const LAYOUT_NAMES: Record<string, string> = {
  title: 'Title', section: 'Section', bullets: 'Title and points', two: 'Two columns', image_right: 'Picture on the right', image_left: 'Picture on the left', image: 'Full picture',
  quote: 'Quote', number: 'Big number', chart: 'Chart', table: 'Table', code: 'Code', poll: 'Live poll', agenda: 'Agenda', end: 'The end', blank: 'Words only',
}

/** A slide's name in lists: its title, else its first words, else its layout. */
export function slideName(s: Slide): string {
  const first = (s.title || s.subtitle || s.body.replace(/^[-*\d.\s]+/, '').split('\n')[0] || '').trim()
  return first || LAYOUT_NAMES[s.layout] || 'Slide'
}

export function fmtMs(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = s % 60
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${String(x).padStart(2, '0')}` : `${m}:${String(x).padStart(2, '0')}`
}

/** A random id this browser keeps, so a vote or a like counts once. */
export function voterId(): string {
  const k = 'psld-voter'
  let v = localStorage.getItem(k)
  if (!v || !/^[A-Za-z0-9_-]{8,64}$/.test(v)) {
    v = Array.from(crypto.getRandomValues(new Uint8Array(12)), b => 'abcdefghijklmnopqrstuvwxyz0123456789'[b % 36]).join('')
    localStorage.setItem(k, v)
  }
  return v
}
