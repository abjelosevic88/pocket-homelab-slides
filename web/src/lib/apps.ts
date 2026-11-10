// The other Pocket apps Slides works with.
import type { AppSlug, AppStateName } from './types'

export const APP_INFO: Record<AppSlug, { name: string; icon: string }> = {
  notes: { name: 'Pocket Notes', icon: 'note' }, photos: { name: 'Pocket Photos', icon: 'image' }, files: { name: 'Pocket Files', icon: 'archive' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar' }, tasks: { name: 'Pocket Tasks', icon: 'check-square' }, chat: { name: 'Pocket Chat', icon: 'message' },
  email: { name: 'Pocket Email', icon: 'mail' }, contacts: { name: 'Pocket Contacts', icon: 'users' }, voice: { name: 'Pocket Voice', icon: 'mic' },
  tables: { name: 'Pocket Tables', icon: 'table' }, translate: { name: 'Pocket Translate', icon: 'globe' }, blog: { name: 'Pocket Blog', icon: 'feather' },
  learn: { name: 'Pocket Learn', icon: 'brain' }, alerts: { name: 'Pocket Alerts', icon: 'bell' },
}

export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Off', link_off: 'Link is off in Pocket Core', missing: 'Not on this box', old: 'Needs an update',
  no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not working',
}
