import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['h'], 'All decks'], [['t'], 'Talks'], [['l'], 'Layouts and themes'], [[','], 'Settings']] },
  { title: 'Make', keys: [[['n'], 'New deck'], [['⌘', '↩'], 'A new slide after this one (editor)'], [['⌘', 'D'], 'Copy the slide (editor)'], [['alt', '↑ ↓'], 'Move the slide (editor)']] },
  { title: 'Present', keys: [[['→', 'space'], 'Next'], [['←'], 'Back'], [['B'], 'Black screen'], [['F'], 'Full screen'], [['P'], 'Phone and audience codes'], [['G'], 'Go to a slide'], [['esc'], 'Stop']] },
  { title: 'Find', keys: [[['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}
