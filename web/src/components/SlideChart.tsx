// Charts on a slide, drawn in the slide's own pixels (the whole slide is
// scaled as one, so nothing is ever stretched).
import type { ChartData } from '../lib/types'

export function mix(a: string, b: string, t: number): string {
  const p = (h: string) => { const m = /^#?([0-9a-f]{6})$/i.exec(h); const n = m ? parseInt(m[1], 16) : 0; return [(n >> 16) & 255, (n >> 8) & 255, n & 255] }
  if (!/^#?[0-9a-f]{6}$/i.test(a) || !/^#?[0-9a-f]{6}$/i.test(b)) return a
  const x = p(a), y = p(b)
  return '#' + x.map((v, i) => Math.round(v * (1 - t) + y[i] * t).toString(16).padStart(2, '0')).join('')
}

function nice(v: number, unit = ''): string {
  const a = Math.abs(v)
  const s = a >= 1e6 ? (v / 1e6).toFixed(a >= 1e7 ? 0 : 1) + 'm' : a >= 1e4 ? (v / 1e3).toFixed(a >= 1e5 ? 0 : 1) + 'k' : Number.isInteger(v) ? v.toLocaleString() : v.toLocaleString(undefined, { maximumFractionDigits: 1 })
  if (!unit) return s
  return ['€', '$', '£'].includes(unit) ? unit + s : unit === '%' ? s + '%' : s
}

function niceMax(v: number) {
  if (v <= 0) return 1
  const e = Math.pow(10, Math.floor(Math.log10(v)))
  for (const m of [1, 1.2, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10]) if (m * e >= v) return m * e
  return 10 * e
}

export function SlideChart({ chart, colors, w, h }: { chart: ChartData; colors: { fg: string; muted: string; accent: string; bg: string; dark: boolean }; w: number; h: number }) {
  const { labels, values, unit = '' } = chart
  const n = Math.min(labels.length, values.length)
  const pal = [colors.accent, mix(colors.accent, colors.fg, 0.45), mix(colors.accent, colors.bg, 0.45), colors.muted, mix(colors.fg, colors.bg, 0.3), mix(colors.accent, '#3b82f6', 0.5),
    mix(colors.accent, '#10b981', 0.5), mix(colors.accent, '#ef4444', 0.5)]
  const grid = mix(colors.fg, colors.bg, 0.85)
  if (n === 0) return null
  if (chart.kind === 'pie' || chart.kind === 'donut') {
    const total = values.slice(0, n).reduce((a, b) => a + Math.max(0, b), 0) || 1
    const r = Math.min(h / 2 - 10, w * 0.22)
    const cx = r + 20, cy = h / 2
    let a0 = -Math.PI / 2
    const arcs = values.slice(0, n).map((v, i) => {
      const a1 = a0 + Math.max(0, v) / total * Math.PI * 2
      const large = a1 - a0 > Math.PI ? 1 : 0
      const d = n === 1 || v >= total ? `M${cx} ${cy - r}A${r} ${r} 0 1 1 ${cx - 0.01} ${cy - r}Z` : `M${cx} ${cy}L${cx + r * Math.cos(a0)} ${cy + r * Math.sin(a0)}A${r} ${r} 0 ${large} 1 ${cx + r * Math.cos(a1)} ${cy + r * Math.sin(a1)}Z`
      a0 = a1
      return <path key={i} d={d} fill={pal[i % pal.length]} stroke={colors.bg} strokeWidth={4} />
    })
    const lx = cx + r + 80
    const rowH = Math.min(84, (h - 20) / n)
    return (
      <svg viewBox={`0 0 ${w} ${h}`} width={w} height={h} className="sv-chart">
        {arcs}
        {chart.kind === 'donut' && <circle cx={cx} cy={cy} r={r * 0.58} fill={colors.bg} />}
        {chart.kind === 'donut' && unit !== '%' && <text x={cx} y={cy + 22} textAnchor="middle" fontSize={64} fontWeight={700} fill={colors.fg}>{nice(total, unit === '%' ? '' : unit)}</text>}
        {labels.slice(0, n).map((l, i) => {
          const y = cy - (n * rowH) / 2 + i * rowH + rowH / 2
          return <g key={i}>
            <rect x={lx} y={y - 18} width={36} height={36} rx={9} fill={pal[i % pal.length]} />
            <text x={lx + 56} y={y + 14} fontSize={40} fill={colors.fg}>{l}</text>
            <text x={w - 10} y={y + 14} fontSize={40} fontWeight={700} textAnchor="end" fill={colors.fg}>{unit === '%' ? nice(values[i], '%') : `${Math.round(Math.max(0, values[i]) * 100 / total)}%`}</text>
          </g>
        })}
      </svg>
    )
  }
  const min = Math.min(0, ...values.slice(0, n))
  const max = niceMax(Math.max(...values.slice(0, n), 0))
  if (chart.kind === 'hbar') {
    const L = Math.min(420, Math.max(...labels.slice(0, n).map(l => l.length)) * 22 + 30)
    const R = 170
    const rowH = Math.min(110, h / n)
    const bh = Math.min(64, rowH * 0.62)
    return (
      <svg viewBox={`0 0 ${w} ${h}`} width={w} height={h} className="sv-chart">
        {labels.slice(0, n).map((l, i) => {
          const y = (h - n * rowH) / 2 + i * rowH + (rowH - bh) / 2
          const bw = Math.max(4, (w - L - R) * Math.max(0, values[i]) / max)
          return <g key={i}>
            <text x={L - 24} y={y + bh / 2 + 14} textAnchor="end" fontSize={38} fill={colors.fg}>{l}</text>
            <rect x={L} y={y} width={bw} height={bh} rx={12} fill={i === 0 ? colors.accent : mix(colors.accent, colors.bg, 0.25)} />
            <text x={L + bw + 18} y={y + bh / 2 + 14} fontSize={38} fontWeight={700} fill={colors.fg}>{nice(values[i], unit)}</text>
          </g>
        })}
      </svg>
    )
  }
  const L = 120, R = 30, T = 60, B = 90
  const iw = w - L - R, ih = h - T - B
  const y = (v: number) => T + ih - (v - min) / (max - min || 1) * ih
  const ticks = [0, 0.25, 0.5, 0.75, 1].map(t => min + (max - min) * t)
  const step = iw / n
  const every = Math.max(1, Math.ceil(n / Math.floor(iw / 150)))
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width={w} height={h} className="sv-chart">
      {ticks.map((t, i) => <g key={i}>
        <line x1={L} x2={w - R} y1={y(t)} y2={y(t)} stroke={grid} strokeWidth={2} />
        <text x={L - 18} y={y(t) + 12} textAnchor="end" fontSize={32} fill={colors.muted}>{nice(t, unit)}</text>
      </g>)}
      {chart.kind === 'line' ? <>
        <path d={values.slice(0, n).map((v, i) => `${i ? 'L' : 'M'}${L + step * i + step / 2} ${y(v)}`).join('')} fill="none" stroke={colors.accent} strokeWidth={8} strokeLinejoin="round" strokeLinecap="round" />
        {values.slice(0, n).map((v, i) => <g key={i}>
          <circle cx={L + step * i + step / 2} cy={y(v)} r={14} fill={colors.bg} stroke={colors.accent} strokeWidth={7} />
          <text x={L + step * i + step / 2} y={y(v) - 30} textAnchor="middle" fontSize={34} fontWeight={700} fill={colors.fg}>{nice(v, unit)}</text>
        </g>)}
      </> : values.slice(0, n).map((v, i) => {
        const bw = Math.min(160, step * 0.62)
        const x = L + step * i + (step - bw) / 2
        const top = Math.min(y(v), y(0))
        const hh = Math.max(4, Math.abs(y(v) - y(0)))
        return <g key={i}>
          <rect x={x} y={top} width={bw} height={hh} rx={Math.min(14, bw / 4)} fill={i === values.slice(0, n).indexOf(Math.max(...values.slice(0, n))) ? colors.accent : mix(colors.accent, colors.bg, 0.3)} />
          <text x={x + bw / 2} y={top - 18} textAnchor="middle" fontSize={36} fontWeight={700} fill={colors.fg}>{nice(v, unit)}</text>
        </g>
      })}
      {labels.slice(0, n).map((l, i) => i % every === 0 && <text key={i} x={L + step * i + step / 2} y={h - B + 56} textAnchor="middle" fontSize={34} fill={colors.muted}>{l.length > 14 ? l.slice(0, 13) + '…' : l}</text>)}
    </svg>
  )
}
