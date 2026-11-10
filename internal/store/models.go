package store

import (
	"encoding/json"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Theme is a person's own look for decks: colours, fonts and a logo.
type Theme struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Name      string `json:"name"`
	Base      string `json:"base"`
	BG        string `json:"bg"`
	BG2       string `json:"bg2"`
	FG        string `json:"fg"`
	Muted     string `json:"muted"`
	Accent    string `json:"accent"`
	HeadFont  string `json:"head_font"`
	BodyFont  string `json:"body_font"`
	LogoID    *int64 `json:"logo_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Deck is a presentation: its slides, its look and where it is given.
type Deck struct {
	ID            int64    `json:"id"`
	UserID        int64    `json:"-"`
	Title         string   `json:"title"`
	About         string   `json:"about"`
	Theme         string   `json:"theme"`
	ThemeID       *int64   `json:"theme_id"`
	Aspect        string   `json:"aspect"`
	Footer        string   `json:"footer"`
	Numbers       bool     `json:"numbers"`
	Transition    string   `json:"transition"`
	TargetMinutes int      `json:"target_minutes"`
	TalkTitle     string   `json:"talk_title"`
	TalkAt        string   `json:"talk_at"`
	TalkPlace     string   `json:"talk_place"`
	CalendarURL   string   `json:"calendar_url"`
	Tags          []string `json:"tags"`
	Template      bool     `json:"template"`
	Pinned        bool     `json:"pinned"`
	Archived      bool     `json:"archived"`
	Shared        bool     `json:"shared"`
	ShareKey      string   `json:"share_key"`
	LiveKey       string   `json:"live_key"`
	Questions     bool     `json:"questions"`
	Source        string   `json:"source"`
	SourceRef     string   `json:"source_ref"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`

	// Worked out when listed.
	Slides      int     `json:"slides"`
	CoverID     int64   `json:"cover_slide_id,omitempty"`
	Rehearsals  int     `json:"rehearsals"`
	Talks       int     `json:"talks"`
	LastShownAt *string `json:"last_shown_at"`
	Live        bool    `json:"live"`
	Cover       *Slide  `json:"cover,omitempty"`     // the first shown slide (lists)
	OwnTheme    *Theme  `json:"own_theme,omitempty"` // its own theme (lists)
}

// Slide is one slide of a deck.
type Slide struct {
	ID        int64           `json:"id"`
	DeckID    int64           `json:"deck_id"`
	UserID    int64           `json:"-"`
	Position  int             `json:"position"`
	Layout    string          `json:"layout"`
	Title     string          `json:"title"`
	Subtitle  string          `json:"subtitle"`
	Body      string          `json:"body"`
	Body2     string          `json:"body2"`
	ImageID   *int64          `json:"image_id"`
	Data      json.RawMessage `json:"data"`
	Notes     string          `json:"notes"`
	Style     string          `json:"style"`
	Hidden    bool            `json:"hidden"`
	Seconds   int             `json:"seconds"`
	Source    string          `json:"source"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// Layout is a slide a person kept to start new slides from.
type Layout struct {
	ID        int64           `json:"id"`
	UserID    int64           `json:"-"`
	Name      string          `json:"name"`
	Layout    string          `json:"layout"`
	Title     string          `json:"title"`
	Subtitle  string          `json:"subtitle"`
	Body      string          `json:"body"`
	Body2     string          `json:"body2"`
	Data      json.RawMessage `json:"data"`
	Notes     string          `json:"notes"`
	Style     string          `json:"style"`
	CreatedAt string          `json:"created_at"`
}

// Upload is a picture kept for a deck (or a theme's logo).
type Upload struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	DeckID    int64  `json:"deck_id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Data      []byte `json:"-"`
	Thumb     []byte `json:"-"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Size      int    `json:"size"`
	CreatedAt string `json:"created_at"`
}

// Show kinds.
const (
	KindTalk      = "talk"
	KindRehearsal = "rehearsal"
)

// Show is one time a deck was presented or practised.
type Show struct {
	ID        int64            `json:"id"`
	UserID    int64            `json:"-"`
	DeckID    int64            `json:"deck_id"`
	Kind      string           `json:"kind"`
	RemoteKey string           `json:"remote_key,omitempty"`
	Pos       int              `json:"pos"`
	Step      int              `json:"step"`
	Black     bool             `json:"black"`
	StartedAt string           `json:"started_at"`
	EndedAt   *string          `json:"ended_at"`
	Timings   map[string]int64 `json:"timings"`
	TotalMs   int64            `json:"total_ms"`
	Viewers   int              `json:"viewers"`
	Reactions int              `json:"reactions"`
	Note      string           `json:"note"`

	DeckTitle string  `json:"deck_title,omitempty"`
	Questions int     `json:"questions"`
	Votes     int     `json:"votes"`
	Feedback  int     `json:"feedback"`
	Rating    float64 `json:"rating"`
}

// Question is something the audience asked during a show.
type Question struct {
	ID        int64  `json:"id"`
	ShowID    int64  `json:"show_id"`
	UserID    int64  `json:"-"`
	SlideID   *int64 `json:"slide_id"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	Votes     int    `json:"votes"`
	Answered  bool   `json:"answered"`
	Hidden    bool   `json:"hidden"`
	CreatedAt string `json:"created_at"`
}

// Feedback is what someone in the audience thought of a talk.
type Feedback struct {
	ID        int64  `json:"id"`
	ShowID    int64  `json:"show_id"`
	UserID    int64  `json:"-"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// New decks.
	Theme      string `json:"theme"`      // built-in theme of a new deck
	Aspect     string `json:"aspect"`     // 16:9 | 4:3
	Transition string `json:"transition"` // fade | slide | none
	Numbers    bool   `json:"numbers"`    // slide numbers in the corner
	Footer     string `json:"footer"`     // e.g. your name, on every slide

	// Presenting.
	Questions    bool `json:"questions"`     // the audience may ask questions
	Feedback     bool `json:"feedback"`      // ask the audience how it was at the end
	WarnMinutes  int  `json:"warn_minutes"`  // the phone warns this many minutes before the end
	NotesSize    int  `json:"notes_size"`    // speaker notes text size on the phone (px)
	KeepAwake    bool `json:"keep_awake"`    // keep the phone screen on while presenting
	AudienceLink bool `json:"audience_link"` // show the audience link on the first slide

	// Other apps.
	CalendarOnTalk  bool   `json:"calendar_on_talk"`  // put a talk in Pocket Calendar when its day is set
	RehearseTasks   bool   `json:"rehearse_tasks"`    // add rehearse tasks to Pocket Tasks with a talk
	AlertsDayBefore bool   `json:"alerts_day_before"` // remind through Pocket Alerts the day before a talk
	NotesFolder     string `json:"notes_folder"`      // where decks go in Pocket Notes
	FilesFolder     string `json:"files_folder"`      // where decks go in Pocket Files
	ChatTo          string `json:"chat_to"`           // the Pocket Chat conversation slides are shared in
	LearnDeck       string `json:"learn_deck"`        // the Pocket Learn deck cards go to
	AIHelp          bool   `json:"ai_help"`

	// Notifications.
	NotifyQuestion bool   `json:"notify_question"` // someone asked a question (when you are not presenting)
	NotifyTalk     bool   `json:"notify_talk"`     // a talk is tomorrow
	NotifyFeedback bool   `json:"notify_feedback"` // the audience said how it was
	NotifyHour     int    `json:"notify_hour"`     // reminders go at this hour
	NotifyURL      string `json:"notify_url"`
	NotifyFormat   string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
