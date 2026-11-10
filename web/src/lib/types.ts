// The shapes the API sends.

export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = 'core' | 'direct' | ''
export type AppSlug = 'notes' | 'photos' | 'files' | 'calendar' | 'tasks' | 'chat' | 'email' | 'contacts' | 'voice' | 'tables' | 'translate' | 'blog' | 'learn' | 'alerts'
type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  theme: string; aspect: string; transition: string; numbers: boolean; footer: string
  questions: boolean; feedback: boolean; warn_minutes: number; notes_size: number; keep_awake: boolean; audience_link: boolean
  calendar_on_talk: boolean; rehearse_tasks: boolean; alerts_day_before: boolean; notes_folder: string; files_folder: string; chat_to: string; learn_deck: string; ai_help: boolean
  notify_question: boolean; notify_talk: boolean; notify_feedback: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export type LayoutKey = 'title' | 'section' | 'bullets' | 'two' | 'image_right' | 'image_left' | 'image' | 'quote' | 'number' | 'chart' | 'table' | 'code' | 'poll' | 'agenda' | 'end' | 'blank'
export type Style = '' | 'accent' | 'dark' | 'light'

export interface ChartData { kind: 'bar' | 'hbar' | 'line' | 'pie' | 'donut'; labels: string[]; values: number[]; unit?: string }
export interface SlideData {
  build?: boolean; fit?: '' | 'cover' | 'contain'; lang?: string; chart?: ChartData; rows?: string[][]; options?: string[]; correct?: number
  qr?: '' | 'share' | 'live' | 'none'; left?: string; right?: string; from?: string
}

export interface Slide {
  id: number; deck_id: number; position: number; layout: LayoutKey; title: string; subtitle: string; body: string; body2: string
  image_id: number | null; data: SlideData; notes: string; style: Style; hidden: boolean; seconds: number; source: string; created_at: string; updated_at: string
}

export interface Theme {
  id: number; name: string; base: string; bg: string; bg2: string; fg: string; muted: string; accent: string; head_font: string; body_font: string; logo_id: number | null
  created_at: string; updated_at: string
}

export interface Deck {
  id: number; title: string; about: string; theme: string; theme_id: number | null; aspect: '16:9' | '4:3'; footer: string; numbers: boolean; transition: 'fade' | 'slide' | 'none'
  target_minutes: number; talk_title: string; talk_at: string; talk_place: string; calendar_url: string; tags: string[]; template: boolean; pinned: boolean; archived: boolean
  shared: boolean; share_key: string; live_key: string; questions: boolean; source: string; source_ref: string; created_at: string; updated_at: string
  slides: number; cover_slide_id?: number; rehearsals: number; talks: number; last_shown_at: string | null; live: boolean
  cover?: Slide; own_theme?: Theme
}

export interface Show {
  id: number; deck_id: number; kind: 'talk' | 'rehearsal'; remote_key?: string; pos: number; step: number; black: boolean; started_at: string; ended_at: string | null
  timings: Record<string, number>; total_ms: number; viewers: number; reactions: number; note: string
  deck_title?: string; questions: number; votes: number; feedback: number; rating: number
}

export interface Question { id: number; show_id: number; slide_id: number | null; name: string; body: string; votes: number; answered: boolean; hidden: boolean; created_at: string }
export interface Feedback { id: number; show_id: number; rating: number; comment: string; name: string; created_at: string }

export type Tally = Record<string, Record<string, number>>

export interface LiveState {
  type: 'state' | 'ended' | 'deck' | 'reaction'
  live: boolean; show_id?: number; kind?: 'talk' | 'rehearsal'; pos: number; step: number; steps: number; black: boolean; total: number; slide_id?: number
  started_at?: string; slide_at?: string; viewers: number; reactions: number; tally: Tally; questions: Question[]; asking: boolean; slide?: Slide; feedback?: boolean
  emoji?: string
}

export interface DeckPayload { deck: Deck; slides: Slide[]; theme?: Theme; show?: Show; share_url: string; live_url: string; remote_url?: string; url: string }

export interface LayoutInfo { key: LayoutKey; name: string; about: string; uses: string[] }
export interface TemplateInfo { key: string; name: string; about: string; theme: string; minutes: number; slides: number }
export interface Builtins { layouts: LayoutInfo[]; themes: { key: string; name: string; dark: boolean }[]; templates: TemplateInfo[] }

export interface MyLayout { id: number; name: string; layout: LayoutKey; title: string; subtitle: string; body: string; body2: string; data: SlideData; notes: string; style: Style; created_at: string }

export interface Home { decks: Deck[]; upcoming: Deck[]; shows: Show[]; live: Deck[]; archived: number }
