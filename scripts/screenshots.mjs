// README screenshots and a browser smoke test against a demo instance:
//
//   make build && DATA_DIR=/tmp/slides-demo DEMO_DATA=true AUTH_MODE=none HOST=127.0.0.1 PORT=18269 ./pocket-slides &
//   CDP=http://127.0.0.1:9339 BASE=http://127.0.0.1:18269 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18269'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text()) })
  return { ctx, page }
}
async function go(page, path, wait = 1400) {
  await page.goto(base + path, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(wait)
}
async function shot(page, name) {
  if (!shots || (only && !only.split(',').includes(name))) return
  for (let i = 0; ; i++) {
    try { await page.bringToFront(); await page.screenshot({ path: dir + name + '.png', animations: 'disabled' }); return } catch (e) { if (i >= 3) throw e; await page.waitForTimeout(700) }
  }
}
const api = (page, path, method = 'GET', body) => page.evaluate(([p, m, b]) => fetch('/api/v1' + p, { method: m, headers: { 'Content-Type': 'application/json' }, body: b ? JSON.stringify(b) : undefined }).then(r => r.status === 204 ? {} : r.json()), [path, method, body])
const deckId = async (page, title) => (await api(page, '/decks')).decks.find(d => d.title.startsWith(title)).id

const { page } = await session()
await go(page, '/', 2000)
assert.ok((await page.content()).includes('Running a home server'), 'the demo decks show on the home page')
await shot(page, 'desktop-home')
const talk = await deckId(page, 'Running a home server')
const deck = await api(page, `/decks/${talk}`)
const slide = n => deck.slides.filter(s => !s.hidden)[n].id
await go(page, `/d/${talk}?slide=${slide(3)}`)
assert.ok((await page.content()).includes('Speaker notes'), 'the editor shows the speaker notes')
await shot(page, 'desktop-editor')
await go(page, `/d/${talk}?slide=${slide(7)}`)
await shot(page, 'desktop-chart')
await go(page, `/d/${talk}/practice`)
assert.ok((await page.content()).includes('The last rehearsal'), 'practice shows the last rehearsal')
await shot(page, 'desktop-practice')

// edit: a new slide, typed into, saved by itself
await go(page, `/d/${talk}?slide=${slide(12)}`)
await page.keyboard.press('Control+Enter')
await page.waitForTimeout(900)
await page.fill('.ed-side input >> nth=0', 'Smoke test slide')
await page.waitForTimeout(1300)
const made = (await api(page, `/decks/${talk}`)).slides.find(s => s.title === 'Smoke test slide')
assert.ok(made, 'a new slide saves by itself')
await api(page, `/slides/${made.id}`, 'DELETE')

// present: the pairing codes, then the slides
await go(page, `/present/${talk}`, 2200)
assert.ok((await page.content()).includes('Your phone'), 'presenting starts with the phone code')
await shot(page, 'desktop-pair')
for (let i = 0; i < 6; i++) { await page.keyboard.press('ArrowRight'); await page.waitForTimeout(350) }
await page.waitForTimeout(800)
await shot(page, 'desktop-present')
const live = await api(page, `/decks/${talk}`)
assert.ok(live.show && live.remote_url, 'a running show has a phone link')
const remote = live.remote_url.replace(/^https?:\/\/[^/]+/, '')

// the phone: presenter screen; the audience
const phone = await session({ phone: true, dark: true })
await go(phone.page, remote, 1800)
assert.ok((await phone.page.content()).includes('Notes'), 'the phone shows the presenter screen')
const before = (await api(page, `/decks/${talk}/live`)).pos
await phone.page.click('.pres-btn.next')
await phone.page.waitForTimeout(900)
const after = await api(page, `/decks/${talk}/live`)
assert.ok(after.pos > before || after.step > 0, 'the phone moves the show')
await shot(phone.page, 'phone-presenter')
const aud = await session({ phone: true })
await go(aud.page, `/w/${live.deck.live_key}`, 1800)
assert.ok(!(await aud.page.content()).includes('Tell the story of the photo app'), 'the audience never sees the notes')
await aud.page.fill('textarea', 'Which box would you buy today?')
await aud.page.click('button.btn.primary:text-is("Ask")')
await aud.page.waitForTimeout(900)
assert.ok((await api(page, `/decks/${talk}/live`)).questions.length >= 1, 'a question reaches the speaker')
await shot(aud.page, 'phone-audience')
await api(page, `/decks/${talk}/live`, 'POST', { action: 'end' })
await phone.ctx.close(); await aud.ctx.close()

const ph = await session({ phone: true })
await go(ph.page, `/d/${talk}?slide=${slide(1)}`, 1600)
await shot(ph.page, 'phone-editor')

const lesson = await deckId(page, 'The water cycle')
const shows = (await api(page, `/shows?deck=${lesson}`)).shows
await go(page, `/d/${lesson}/shows/${shows[0].id}`, 1600)
await shot(page, 'desktop-show')

// the share link
await go(page, `/s/${live.deck.share_key}`, 1500)
assert.ok(!(await page.content()).includes('Tell the story of the photo app'), 'the share link has no notes')

if (errors.length) { console.error(errors.join('\n')); process.exitCode = 1 } else console.log('ok: every check passed')
await browser.close()
