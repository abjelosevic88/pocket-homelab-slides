# Configuration

Environment variables; every one has a default. Personal settings (how new decks start, presenting, notifications, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings.

| Variable | Default | What it does |
|---|---|---|
| `BASE_URL` | – | Where the app is (links in pushes, events and other apps, the OIDC callback) |
| `HOST` / `PORT` | `0.0.0.0` / `8080` | Where it listens |
| `DATA_DIR` | `./data` | The database (the pictures are inside it) and the Pocket Core token |
| `DB_PATH` | `$DATA_DIR/pocket-slides.db` | The SQLite file |
| `AUTH_MODE` | `local` | `local` (passwords), `proxy` (a header from your reverse proxy), `none` (one user, for trying it out) |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | – | The first account, made on an empty database |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL` / `NOTIFY_FORMAT` | – / `apprise` | Pushes: an Apprise URL, an ntfy topic or a JSON webhook |
| `NOTIFY_HOUR` | `9` | Default hour of the reminder the day before a talk |
| `OPENAI_API_KEY` / `OPENAI_MODEL` / `OPENAI_BASE_URL` | – | AI without Pocket Core |
| `CORE_URL` / `CORE_REGISTRATION_KEY` / `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY` | `false` | Take the client address from `X-Forwarded-For` (the audience's answers, questions and feedback are limited per address) |
| `METRICS_ENABLED` | `false` | Prometheus `/metrics` |
| `MAX_BODY_BYTES` | `67108864` | Largest request; one picture up to 25 MB, a PowerPoint to bring in up to 100 MB |
| `DEMO_DATA` | `false` | Seed four decks (a meetup talk with every kind of slide, a lesson with a quiz, a project update with a chart, a book club poll), rehearsals with timings, a past talk with questions, poll answers and feedback, a theme and a layout of your own |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | |

Connections to Pocket Notes, Photos, Files, Calendar, Tasks, Chat, Email, Contacts, Voice, Tables, Translate, Blog, Learn and Alerts are made per person under Settings → Connected apps (through Pocket Core with nothing to paste, or directly with a URL and a token).

