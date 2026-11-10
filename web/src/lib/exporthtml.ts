// A deck as one web page that opens anywhere, offline: every slide drawn by
// the same component as in the app, the pictures inside the file, arrow keys
// to move. No speaker notes, no skipped slides.
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import css from '../styles/slides.css?raw'
import type { DeckPayload } from './types'
import { canvasOf, lookOf } from './slides'
import { agendaOf, ownImage, shown } from './deck'
import { SlideView } from '../components/SlideView'

async function dataURL(url: string): Promise<string> {
  const r = await fetch(url, { credentials: 'same-origin' })
  if (!r.ok) return ''
  const b = await r.blob()
  return await new Promise(res => { const fr = new FileReader(); fr.onload = () => res(String(fr.result)); fr.onerror = () => res(''); fr.readAsDataURL(b) })
}

const esc = (s: string) => s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)

export async function exportHTML(p: DeckPayload): Promise<string> {
  const vis = shown(p.slides)
  const look = lookOf(p.deck, p.theme)
  const ids = new Set<number>()
  vis.forEach(s => { if (s.image_id) ids.add(s.image_id) })
  if (look.logo) ids.add(look.logo)
  const pics = new Map<number, string>()
  for (const id of ids) pics.set(id, await dataURL(ownImage(id)))
  const { w, h } = canvasOf(p.deck.aspect)
  const agenda = agendaOf(p.slides)
  const slides = vis.map((s, i) => renderToStaticMarkup(createElement(SlideView, {
    slide: s, deck: p.deck, look, index: i, total: vis.length, imageUrl: (id: number) => pics.get(id) || '', agenda, qr: {}, width: w,
  })))
  const svCSS = css.split('\n').filter(l => /^\s*(\.sv|\.qr|@keyframes sv|\.is-thumb)/.test(l)).join('\n')
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>${esc(p.deck.title)}</title>
<meta name="generator" content="Pocket Slides">
<style>
html,body{margin:0;height:100%;background:#000;overflow:hidden;font-family:system-ui,sans-serif}
.deck{position:fixed;inset:0;display:flex;align-items:center;justify-content:center}
.frame{position:relative;width:${w}px;height:${h}px;transform-origin:center}
.frame>section{position:absolute;inset:0;display:none}.frame>section.on{display:block;animation:sv-fade .35s ease}
.bar{position:fixed;left:50%;bottom:14px;transform:translateX(-50%);display:flex;gap:6px;align-items:center;background:rgba(20,20,22,.8);color:#fff;padding:6px 10px;border-radius:10px;font:13px system-ui;opacity:.0;transition:opacity .2s}
body:hover .bar{opacity:1}.bar button{background:none;border:0;color:#fff;font-size:18px;cursor:pointer;padding:2px 8px}
@media print{html,body{overflow:visible;background:#fff}.deck{position:static;display:block}.frame{transform:none!important;width:auto;height:auto}.frame>section{position:relative;display:block;width:${w}px;height:${h}px;break-after:page}.bar{display:none}@page{size:${w}px ${h}px;margin:0}}
${svCSS}
.sv-box{width:${w}px!important;border-radius:0}
</style></head><body>
<div class="deck"><div class="frame">${slides.map((s, i) => `<section${i === 0 ? ' class="on"' : ''}>${s}</section>`).join('')}</div></div>
<div class="bar"><button aria-label="Back" onclick="go(-1)">‹</button><span id="n">1 / ${vis.length}</span><button aria-label="Next" onclick="go(1)">›</button></div>
<script>
var i=0,s=document.querySelectorAll('.frame>section'),f=document.querySelector('.frame');
function fit(){var k=Math.min(innerWidth/${w},innerHeight/${h});f.style.transform='scale('+k+')'}
function show(){s.forEach(function(x,j){x.classList.toggle('on',j===i)});document.getElementById('n').textContent=(i+1)+' / '+s.length;location.hash=i+1}
function go(d){i=Math.max(0,Math.min(s.length-1,i+d));show()}
addEventListener('resize',fit);fit();
addEventListener('keydown',function(e){if(['ArrowRight','ArrowDown',' ','PageDown','Enter'].indexOf(e.key)>=0){e.preventDefault();go(1)}if(['ArrowLeft','ArrowUp','PageUp','Backspace'].indexOf(e.key)>=0){e.preventDefault();go(-1)}if(e.key==='Home'){i=0;show()}if(e.key==='End'){i=s.length-1;show()}if(e.key==='f'&&document.documentElement.requestFullscreen)document.documentElement.requestFullscreen()});
var x0=null;addEventListener('touchstart',function(e){x0=e.touches[0].clientX});addEventListener('touchend',function(e){if(x0===null)return;var dx=e.changedTouches[0].clientX-x0;if(Math.abs(dx)>40)go(dx<0?1:-1);x0=null});
var h=parseInt(location.hash.slice(1));if(h>0){i=Math.min(s.length-1,h-1);show()}
</script></body></html>`
}
