package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"strings"
)

const deckCols = `d.id, d.user_id, d.title, d.about, d.theme, d.theme_id, d.aspect, d.footer, d.numbers, d.transition, d.target_minutes,
	d.talk_title, d.talk_at, d.talk_place, d.calendar_url, d.tags, d.template, d.pinned, d.archived, d.shared, d.share_key, d.live_key,
	d.questions, d.source, d.source_ref, d.created_at, d.updated_at`

const deckStats = `,
	(SELECT COUNT(1) FROM slides s WHERE s.deck_id = d.id),
	COALESCE((SELECT s.id FROM slides s WHERE s.deck_id = d.id AND s.hidden = 0 ORDER BY s.position, s.id LIMIT 1), 0),
	(SELECT COUNT(1) FROM shows h WHERE h.deck_id = d.id AND h.kind = 'rehearsal'),
	(SELECT COUNT(1) FROM shows h WHERE h.deck_id = d.id AND h.kind = 'talk'),
	(SELECT MAX(h.started_at) FROM shows h WHERE h.deck_id = d.id),
	EXISTS (SELECT 1 FROM shows h WHERE h.deck_id = d.id AND h.ended_at IS NULL)`

func scanDeck(row interface{ Scan(...any) error }, stats bool) (*Deck, error) {
	var d Deck
	var tags string
	dst := []any{&d.ID, &d.UserID, &d.Title, &d.About, &d.Theme, &d.ThemeID, &d.Aspect, &d.Footer, &d.Numbers, &d.Transition, &d.TargetMinutes,
		&d.TalkTitle, &d.TalkAt, &d.TalkPlace, &d.CalendarURL, &tags, &d.Template, &d.Pinned, &d.Archived, &d.Shared, &d.ShareKey, &d.LiveKey,
		&d.Questions, &d.Source, &d.SourceRef, &d.CreatedAt, &d.UpdatedAt}
	if stats {
		dst = append(dst, &d.Slides, &d.CoverID, &d.Rehearsals, &d.Talks, &d.LastShownAt, &d.Live)
	}
	if err := row.Scan(dst...); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(tags), &d.Tags)
	if d.Tags == nil {
		d.Tags = []string{}
	}
	return &d, nil
}

// Decks lists one person's decks, pinned and most recently changed first.
func (s *Store) Decks(ctx context.Context, uid int64, archived bool) ([]Deck, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.user_id = ? AND d.archived = ?
		ORDER BY d.pinned DESC, d.updated_at DESC, d.id DESC`, uid, boolInt(archived))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deck{}
	for rows.Next() {
		d, err := scanDeck(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// GetDeck loads one of a person's decks (another person's → ErrNotFound).
func (s *Store) GetDeck(ctx context.Context, uid, id int64) (*Deck, error) {
	return scanDeck(s.DB.QueryRowContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.id = ? AND d.user_id = ?`, id, uid), true)
}

// DeckByID loads a deck whoever owns it (for the live screens and links).
func (s *Store) DeckByID(ctx context.Context, id int64) (*Deck, error) {
	return scanDeck(s.DB.QueryRowContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.id = ?`, id), true)
}

// DeckByKey finds a deck by its share or audience key (kind "share" | "live").
func (s *Store) DeckByKey(ctx context.Context, kind, key string) (*Deck, error) {
	col := "share_key"
	if kind == "live" {
		col = "live_key"
	}
	if strings.TrimSpace(key) == "" {
		return nil, ErrNotFound
	}
	return scanDeck(s.DB.QueryRowContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.`+col+` = ?`, key), true)
}

// CreateDeck stores a new deck (keys made here when empty).
func (s *Store) CreateDeck(ctx context.Context, d *Deck) error {
	if d.ShareKey == "" {
		d.ShareKey = RandomToken(9)
	}
	if d.LiveKey == "" {
		d.LiveKey = ShortKey()
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO decks (user_id, title, about, theme, theme_id, aspect, footer, numbers, transition, target_minutes,
		talk_title, talk_at, talk_place, calendar_url, tags, template, pinned, archived, shared, share_key, live_key, questions, source, source_ref, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.UserID, d.Title, d.About, d.Theme, nullI(d.ThemeID), d.Aspect, d.Footer, boolInt(d.Numbers), d.Transition, d.TargetMinutes,
		d.TalkTitle, d.TalkAt, d.TalkPlace, d.CalendarURL, toJSON(d.Tags), boolInt(d.Template), boolInt(d.Pinned), boolInt(d.Archived), boolInt(d.Shared),
		d.ShareKey, d.LiveKey, boolInt(d.Questions), d.Source, d.SourceRef, firstSet(d.CreatedAt, now), firstSet(d.UpdatedAt, now))
	if err != nil {
		return conflict(err)
	}
	d.ID, _ = res.LastInsertId()
	d.CreatedAt, d.UpdatedAt = firstSet(d.CreatedAt, now), firstSet(d.UpdatedAt, now)
	return nil
}

// UpdateDeck saves a deck's fields.
func (s *Store) UpdateDeck(ctx context.Context, d *Deck) error {
	d.UpdatedAt = Now()
	_, err := s.DB.ExecContext(ctx, `UPDATE decks SET title=?, about=?, theme=?, theme_id=?, aspect=?, footer=?, numbers=?, transition=?, target_minutes=?,
		talk_title=?, talk_at=?, talk_place=?, calendar_url=?, tags=?, template=?, pinned=?, archived=?, shared=?, share_key=?, live_key=?, questions=?, updated_at=?
		WHERE id = ? AND user_id = ?`,
		d.Title, d.About, d.Theme, nullI(d.ThemeID), d.Aspect, d.Footer, boolInt(d.Numbers), d.Transition, d.TargetMinutes,
		d.TalkTitle, d.TalkAt, d.TalkPlace, d.CalendarURL, toJSON(d.Tags), boolInt(d.Template), boolInt(d.Pinned), boolInt(d.Archived), boolInt(d.Shared),
		d.ShareKey, d.LiveKey, boolInt(d.Questions), d.UpdatedAt, d.ID, d.UserID)
	return conflict(err)
}

// TouchDeck marks a deck as changed now.
func (s *Store) TouchDeck(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE decks SET updated_at = ? WHERE id = ?`, Now(), id)
}

// DeleteDeck removes a deck with its slides, pictures and shows.
func (s *Store) DeleteDeck(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM decks WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeckBySource finds a deck an app made before (for idempotent imports).
func (s *Store) DeckBySource(ctx context.Context, uid int64, source, ref string) (*Deck, error) {
	if ref == "" {
		return nil, ErrNotFound
	}
	return scanDeck(s.DB.QueryRowContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.user_id = ? AND d.source = ? AND d.source_ref = ?`, uid, source, ref), true)
}

// UpcomingTalks lists decks with a talk on or after from (local YYYY-MM-DDTHH:MM).
func (s *Store) UpcomingTalks(ctx context.Context, uid int64, from string) ([]Deck, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.user_id = ? AND d.archived = 0 AND d.talk_at <> '' AND d.talk_at >= ?
		ORDER BY d.talk_at`, uid, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deck{}
	for rows.Next() {
		d, err := scanDeck(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// TalksBetween lists every person's decks with a talk in [from, to) — for the reminders.
func (s *Store) TalksBetween(ctx context.Context, from, to string) ([]Deck, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.archived = 0 AND d.talk_at >= ? AND d.talk_at < ? ORDER BY d.talk_at`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deck{}
	for rows.Next() {
		d, err := scanDeck(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// SearchDecks finds a person's decks by words in their titles, slides and notes.
func (s *Store) SearchDecks(ctx context.Context, uid int64, q string, limit int) ([]Deck, map[int64]Slide, error) {
	like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	rows, err := s.DB.QueryContext(ctx, `SELECT `+deckCols+deckStats+` FROM decks d WHERE d.user_id = ? AND (lower(d.title) LIKE ? OR lower(d.about) LIKE ? OR lower(d.tags) LIKE ?
		OR EXISTS (SELECT 1 FROM slides s WHERE s.deck_id = d.id AND (lower(s.title) LIKE ? OR lower(s.body) LIKE ? OR lower(s.body2) LIKE ? OR lower(s.notes) LIKE ? OR lower(s.subtitle) LIKE ?)))
		ORDER BY d.archived, d.updated_at DESC LIMIT ?`, uid, like, like, like, like, like, like, like, like, limit)
	if err != nil {
		return nil, nil, err
	}
	out := []Deck{}
	for rows.Next() {
		d, err := scanDeck(rows, true)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		out = append(out, *d)
	}
	rows.Close()
	hits := map[int64]Slide{}
	for _, d := range out {
		sl, err := scanSlide(s.DB.QueryRowContext(ctx, `SELECT `+slideCols+` FROM slides WHERE deck_id = ? AND (lower(title) LIKE ? OR lower(body) LIKE ? OR lower(body2) LIKE ? OR lower(notes) LIKE ? OR lower(subtitle) LIKE ?)
			ORDER BY position LIMIT 1`, d.ID, like, like, like, like, like))
		if err == nil {
			hits[d.ID] = *sl
		}
	}
	return out, hits, nil
}

// ---- themes ----

const themeCols = `id, user_id, name, base, bg, bg2, fg, muted, accent, head_font, body_font, logo_id, created_at, updated_at`

func scanTheme(row interface{ Scan(...any) error }) (*Theme, error) {
	var t Theme
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Base, &t.BG, &t.BG2, &t.FG, &t.Muted, &t.Accent, &t.HeadFont, &t.BodyFont, &t.LogoID, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

// Themes lists a person's own themes.
func (s *Store) Themes(ctx context.Context, uid int64) ([]Theme, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+themeCols+` FROM themes WHERE user_id = ? ORDER BY name COLLATE NOCASE, id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Theme{}
	for rows.Next() {
		t, err := scanTheme(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTheme loads one of a person's themes.
func (s *Store) GetTheme(ctx context.Context, uid, id int64) (*Theme, error) {
	return scanTheme(s.DB.QueryRowContext(ctx, `SELECT `+themeCols+` FROM themes WHERE id = ? AND user_id = ?`, id, uid))
}

// ThemeByID loads a theme whoever owns it (for a deck seen through its link).
func (s *Store) ThemeByID(ctx context.Context, id int64) (*Theme, error) {
	return scanTheme(s.DB.QueryRowContext(ctx, `SELECT `+themeCols+` FROM themes WHERE id = ?`, id))
}

// SaveTheme creates or updates a theme.
func (s *Store) SaveTheme(ctx context.Context, t *Theme) error {
	now := Now()
	if t.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO themes (user_id, name, base, bg, bg2, fg, muted, accent, head_font, body_font, logo_id, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, t.UserID, t.Name, t.Base, t.BG, t.BG2, t.FG, t.Muted, t.Accent, t.HeadFont, t.BodyFont, nullI(t.LogoID), now, now)
		if err != nil {
			return err
		}
		t.ID, _ = res.LastInsertId()
		t.CreatedAt, t.UpdatedAt = now, now
		return nil
	}
	t.UpdatedAt = now
	_, err := s.DB.ExecContext(ctx, `UPDATE themes SET name=?, base=?, bg=?, bg2=?, fg=?, muted=?, accent=?, head_font=?, body_font=?, logo_id=?, updated_at=? WHERE id=? AND user_id=?`,
		t.Name, t.Base, t.BG, t.BG2, t.FG, t.Muted, t.Accent, t.HeadFont, t.BodyFont, nullI(t.LogoID), now, t.ID, t.UserID)
	return err
}

// DeleteTheme removes a theme (its decks go back to their built-in theme).
func (s *Store) DeleteTheme(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM themes WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- my layouts ----

// Layouts lists a person's kept slides.
func (s *Store) Layouts(ctx context.Context, uid int64) ([]Layout, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, name, layout, title, subtitle, body, body2, data, notes, style, created_at FROM layouts WHERE user_id = ? ORDER BY name COLLATE NOCASE, id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Layout{}
	for rows.Next() {
		var l Layout
		var data string
		if err := rows.Scan(&l.ID, &l.UserID, &l.Name, &l.Layout, &l.Title, &l.Subtitle, &l.Body, &l.Body2, &data, &l.Notes, &l.Style, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Data = rawOr(data)
		out = append(out, l)
	}
	return out, rows.Err()
}

// CreateLayout keeps a slide as a layout.
func (s *Store) CreateLayout(ctx context.Context, l *Layout) error {
	l.CreatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO layouts (user_id, name, layout, title, subtitle, body, body2, data, notes, style, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		l.UserID, l.Name, l.Layout, l.Title, l.Subtitle, l.Body, l.Body2, string(rawOr(string(l.Data))), l.Notes, l.Style, l.CreatedAt)
	if err != nil {
		return err
	}
	l.ID, _ = res.LastInsertId()
	return nil
}

// GetLayout loads one of a person's layouts.
func (s *Store) GetLayout(ctx context.Context, uid, id int64) (*Layout, error) {
	var l Layout
	var data string
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, name, layout, title, subtitle, body, body2, data, notes, style, created_at FROM layouts WHERE id = ? AND user_id = ?`, id, uid).
		Scan(&l.ID, &l.UserID, &l.Name, &l.Layout, &l.Title, &l.Subtitle, &l.Body, &l.Body2, &data, &l.Notes, &l.Style, &l.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	l.Data = rawOr(data)
	return &l, nil
}

// RenameLayout changes a layout's name.
func (s *Store) RenameLayout(ctx context.Context, uid, id int64, name string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE layouts SET name = ? WHERE id = ? AND user_id = ?`, name, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteLayout removes a layout.
func (s *Store) DeleteLayout(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM layouts WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func rawOr(v string) json.RawMessage {
	if v == "" || !json.Valid([]byte(v)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(v)
}

func firstSet(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

// ShortKey is an audience key people can type: 6 letters and digits that
// are easy to tell apart.
func ShortKey() string {
	const abc = "abcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	out := make([]byte, 6)
	for i := range out {
		out[i] = abc[int(b[i])%len(abc)]
	}
	return string(out)
}

var _ = sql.ErrNoRows
