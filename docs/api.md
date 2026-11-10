# API

Everything is under `/api/v1`, JSON in and out, with a session cookie, `Authorization: Bearer psld_…` (Settings → API & shortcuts) or a Pocket Core link token. Errors are `{"error": "…"}`. Everything is per person: another person's deck, slide, picture or show is a 404.

Times are ISO 8601 UTC; a talk's `talk_at` is local `YYYY-MM-DDTHH:MM`.

## Decks and slides

| | |
|---|---|
| `GET /decks` (`?archived=1`) | Your decks, each with its first slide (`cover`) and own theme |
| `POST /decks` | `{title, template: blank\|talk\|lesson\|update\|pitch\|photos\|workshop, theme, theme_id}`, or `{from_deck: ID}` (a copy), or `{title, slides: [{layout, title, body, notes, data}…], source_ref}` (apps) |
| `GET /decks/ID` | `{deck, slides, theme, show (if running), share_url, live_url, remote_url}` |
| `PUT /decks/ID` | Any of `title, about, theme, theme_id, aspect (16:9\|4:3), footer, numbers, transition (fade\|slide\|none), target_minutes, talk_title, talk_at, talk_place, tags, template, pinned, archived, questions`. Setting `talk_at` puts the talk in Pocket Calendar and Tasks when switched on |
| `DELETE /decks/ID` | With its slides, pictures and shows |
| `POST /decks/ID/copy` | `{title}` |
| `PUT /decks/ID/share` | `{shared}` switches the read-only link; `{new_keys: true}` makes new share and audience links |
| `GET /decks/ID/export.md` | The outline with speaker notes (`?no_notes=1` without) |
| `POST /decks/ID/slides` | `{layout, title, subtitle, body, body2, image_id, data, notes, style, hidden, seconds, after: SLIDE_ID \| position, layout_id: MY_LAYOUT}` |
| `PUT /slides/ID` | The same fields (`no_image: true` takes the picture off) |
| `DELETE /slides/ID`, `POST /slides/ID/copy` | |
| `PUT /decks/ID/order` | `{ids: […]}` |
| `POST /slides/ID/keep` | `{name, keep_words}` — keep a slide as one of my layouts |
| `GET /layouts`, `PUT /layouts/ID {name}`, `DELETE /layouts/ID` | My layouts |
| `GET/POST /themes`, `PUT/DELETE /themes/ID` | My themes `{name, base, bg, bg2, fg, muted, accent, head_font, body_font, logo_id}` |
| `GET /builtins` | The built-in layouts (with the fields each uses), themes and templates |
| `POST /uploads` | multipart `file` (JPEG/PNG/WebP/GIF, 25 MB), `deck`, optional `thumb`, `width`, `height` |
| `GET /uploads/ID` | A picture (`?thumb=1` small) |
| `POST /import/deck` | `{text, title}` (Markdown) or multipart `file` (.pptx, .md) → a new deck |

Layouts: `title, section, bullets, two, image_right, image_left, image, quote, number, chart, table, code, poll, agenda, end, blank`. `data` holds `build` (points one at a time), `fit`, `lang`, `chart {kind: bar|hbar|line|pie|donut, labels, values, unit}`, `rows` (table, first row the headings), `options` and `correct` (poll / quiz), `qr` (end: share|live|none), `left`/`right` (column headings).

Slide text is short Markdown: `- ` points, `1. ` numbers, two spaces in front for a point under a point, `**bold**`, `*italic*`, `` `code` ``.

## Presenting

| | |
|---|---|
| `POST /decks/ID/live` | `{action: start, kind: talk\|rehearsal, pos}`, `{action: next\|prev}`, `{action: go, pos}`, `{action: black, black}`, `{action: end, note}` |
| `GET /decks/ID/live` | The live state `{live, show_id, kind, pos, step, steps, black, total, slide_id, started_at, slide_at, viewers, reactions, tally, questions}` |
| `GET /decks/ID/live/stream` | The same as server-sent events after every change (`type: state\|ended\|deck\|reaction`) |
| `GET /shows?deck=ID`, `GET /shows/ID` | Talks and rehearsals: timings per slide id (ms), questions, feedback, poll tally |
| `PUT /shows/ID` | `{note, kind}`; `DELETE /shows/ID` |
| `PUT /questions/ID` | `{answered, hidden}` |

## Links without an account

| | |
|---|---|
| `GET /public/s/KEY` (+ `/uploads/ID`) | The share link: shown slides, no notes (only while sharing is on) |
| `GET /public/live/KEY` (+ `/stream`, `/uploads/ID`) | The audience: the slides shown so far of a running talk (never a rehearsal), no notes, no quiz answer before it is shown |
| `POST /public/live/KEY/vote` | `{slide_id, choice, voter}` — only the poll on the screen |
| `POST /public/live/KEY/questions` | `{body, name, voter}`; `POST …/questions/QID/like {voter}` |
| `POST /public/live/KEY/react` | `{emoji: 👏 ❤️ 😂 🤯 👍 🎉, voter}` |
| `POST /public/live/KEY/feedback` | `{rating 1–5, comment, name, voter}` — during or up to two days after a talk |
| `GET /public/remote/KEY` (+ `/stream`, `/uploads/ID`) | A show's phone link: the presenter screen, notes included, while that show runs |
| `POST /public/remote/KEY` | Move the show (not start it); `PUT /public/remote/KEY/questions/QID` |

## For the rest of the box

| | |
|---|---|
| `GET /home` | Decks, upcoming talks, recent shows, live decks |
| `GET /summary` | Counts and the next talk (Pocket Day, AI, Homepage) |
| `GET /day/YYYY-MM-DD` | Talks given and rehearsed that day, and a talk planned on it |
| `GET /search?q=` | Decks by the words on their slides and in their notes |

## Other Pocket apps

Through Pocket Core app links (each switched on per person), Slides reaches Notes (`notes:read`, `notes:write`), Photos (`photos:read`), Files (`files:read`, `files:write`), Calendar (`calendar:read`, `calendar:write`), Tasks (`tasks:write`), Chat (`chat:read`, `chat:write`), Email (`mail:send`), Contacts (`contacts:read`), Voice (`voice:read`), Tables (`tables:read`), Translate (`translate:run`), Blog (`blog:write`), Learn (`learn:write`) and Alerts (`alerts:write`); and Pocket AI for drafting and speaker notes.

Slides offers `slides:read` (decks, slides, notes, shows, summary, day, search) and `slides:write` (make a deck with slides, add slides; an app changes only what it made). It publishes `slides.deck_created`, `slides.presented`, `slides.rehearsed` and `slides.shared`.
