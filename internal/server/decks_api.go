package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// ---- what is built in ----

func (s *Server) handleBuiltins(w http.ResponseWriter, r *http.Request) {
	tm := []map[string]any{}
	for _, t := range deckTemplates {
		tm = append(tm, map[string]any{"key": t.Key, "name": t.Name, "about": t.About, "theme": t.Theme, "minutes": t.Minute, "slides": len(t.slides)})
	}
	th := []map[string]any{}
	for _, t := range builtinThemes {
		th = append(th, map[string]any{"key": t.Key, "name": t.Name, "dark": t.Dark})
	}
	writeJSON(w, http.StatusOK, map[string]any{"layouts": builtinLayouts, "themes": th, "templates": tm})
}

// ---- decks ----

func (s *Server) handleDecks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.Decks(ctx, actorID(ctx), queryBool(r, "archived"))
	if err != nil {
		s.fail(w, err, "decks")
		return
	}
	s.withCovers(ctx, list)
	writeJSON(w, http.StatusOK, map[string]any{"decks": list})
}

// withCovers adds each deck's first slide and own theme, for thumbnails.
func (s *Server) withCovers(ctx context.Context, list []store.Deck) {
	themes := map[int64]*store.Theme{}
	for i := range list {
		d := &list[i]
		if d.CoverID > 0 {
			if sl, err := s.store.GetSlide(ctx, d.UserID, d.CoverID); err == nil {
				sl.Notes = ""
				d.Cover = sl
			}
		}
		if d.ThemeID != nil {
			t, ok := themes[*d.ThemeID]
			if !ok {
				t, _ = s.store.GetTheme(ctx, d.UserID, *d.ThemeID)
				themes[*d.ThemeID] = t
			}
			d.OwnTheme = t
		}
	}
}

// deckOf loads one of the caller's decks, writing 404 when it is not theirs.
func (s *Server) deckOf(w http.ResponseWriter, r *http.Request, id int64) (*store.Deck, bool) {
	ctx := r.Context()
	d, err := s.store.GetDeck(ctx, actorID(ctx), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return d, true
}

type deckIn struct {
	Title         *string   `json:"title"`
	About         *string   `json:"about"`
	Theme         *string   `json:"theme"`
	ThemeID       *int64    `json:"theme_id"`
	Aspect        *string   `json:"aspect"`
	Footer        *string   `json:"footer"`
	Numbers       *bool     `json:"numbers"`
	Transition    *string   `json:"transition"`
	TargetMinutes *int      `json:"target_minutes"`
	TalkTitle     *string   `json:"talk_title"`
	TalkAt        *string   `json:"talk_at"`
	TalkPlace     *string   `json:"talk_place"`
	Tags          *[]string `json:"tags"`
	Template      *bool     `json:"template"`
	Pinned        *bool     `json:"pinned"`
	Archived      *bool     `json:"archived"`
	Questions     *bool     `json:"questions"`
}

var reTalkAt = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2})?$`)

func (s *Server) applyDeck(ctx context.Context, uid int64, d *store.Deck, in deckIn, keys map[string]bool) error {
	if in.Title != nil {
		d.Title = cleanText(*in.Title, 200)
		if d.Title == "" {
			return &userErr{"a deck needs a title"}
		}
	}
	if in.About != nil {
		d.About = cleanText(*in.About, 2000)
	}
	if in.Theme != nil {
		if !validTheme(*in.Theme) {
			return &userErr{"unknown theme"}
		}
		d.Theme = *in.Theme
	}
	if keys["theme_id"] {
		if in.ThemeID == nil || *in.ThemeID == 0 {
			d.ThemeID = nil
		} else if _, err := s.store.GetTheme(ctx, uid, *in.ThemeID); err != nil {
			return &userErr{"that theme is not yours"}
		} else {
			d.ThemeID = in.ThemeID
		}
	}
	if in.Aspect != nil {
		if !oneOf(*in.Aspect, aspects) {
			return &userErr{"the shape is 16:9 or 4:3"}
		}
		d.Aspect = *in.Aspect
	}
	if in.Footer != nil {
		d.Footer = cleanText(*in.Footer, 120)
	}
	if in.Numbers != nil {
		d.Numbers = *in.Numbers
	}
	if in.Transition != nil {
		if !oneOf(*in.Transition, transitions) {
			return &userErr{"the change between slides is fade, slide or none"}
		}
		d.Transition = *in.Transition
	}
	if in.TargetMinutes != nil {
		d.TargetMinutes = clampInt(*in.TargetMinutes, 0, 600)
	}
	if in.TalkTitle != nil {
		d.TalkTitle = cleanText(*in.TalkTitle, 200)
	}
	if in.TalkAt != nil {
		v := strings.TrimSpace(*in.TalkAt)
		if v != "" && !reTalkAt.MatchString(v) {
			return &userErr{"the talk's time is YYYY-MM-DDTHH:MM"}
		}
		d.TalkAt = v
	}
	if in.TalkPlace != nil {
		d.TalkPlace = cleanText(*in.TalkPlace, 200)
	}
	if in.Tags != nil {
		d.Tags = cleanTags(*in.Tags)
	}
	if in.Template != nil {
		d.Template = *in.Template
	}
	if in.Pinned != nil {
		d.Pinned = *in.Pinned
	}
	if in.Archived != nil {
		d.Archived = *in.Archived
	}
	if in.Questions != nil {
		d.Questions = *in.Questions
	}
	return nil
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.ToLower(cleanText(strings.TrimPrefix(strings.TrimSpace(t), "#"), 30))
		if t != "" && !seen[t] && len(out) < 12 {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// newDeck is a deck with the person's defaults.
func (s *Server) newDeck(ctx context.Context, uid int64, title string) *store.Deck {
	st := s.settingsOf(ctx, uid)
	return &store.Deck{UserID: uid, Title: title, Theme: firstNonEmpty(st.Theme, "paper"), Aspect: firstNonEmpty(st.Aspect, "16:9"), Footer: st.Footer,
		Numbers: st.Numbers, Transition: firstNonEmpty(st.Transition, "fade"), Questions: st.Questions, Source: sourceOf(ctx)}
}

func (s *Server) handleCreateDeck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		deckIn
		Template  string            `json:"template"`  // a built-in template
		FromDeck  int64             `json:"from_deck"` // or a copy of one of your decks
		Slides    []json.RawMessage `json:"slides"`    // or these slides (for apps)
		SourceRef string            `json:"source_ref"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.FromDeck > 0 {
		from, err := s.store.GetDeck(ctx, u.ID, in.FromDeck)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		d, err := s.copyDeck(ctx, u, from, firstNonEmpty(strPtr(in.Title), from.Title))
		if err != nil {
			s.userFail(w, err, "copy")
			return
		}
		writeJSON(w, http.StatusCreated, s.deckPayload(ctx, d))
		return
	}
	if src := sourceOf(ctx); src != "" && in.SourceRef != "" {
		if d, err := s.store.DeckBySource(ctx, u.ID, src, clip(in.SourceRef, 200)); err == nil {
			writeJSON(w, http.StatusOK, s.deckPayload(ctx, d))
			return
		}
	}
	title := cleanText(strPtr(in.Title), 200)
	if title == "" {
		title = "Untitled deck"
	}
	d := s.newDeck(ctx, u.ID, title)
	d.SourceRef = clip(in.SourceRef, 200)
	tm, hasTm := templateByKey(firstNonEmpty(in.Template, "blank"))
	if hasTm && tm.Theme != "" && in.Theme == nil {
		d.Theme = tm.Theme
	}
	if hasTm && tm.Minute > 0 && in.TargetMinutes == nil {
		d.TargetMinutes = tm.Minute
	}
	in.deckIn.Title = &title
	if err := s.applyDeck(ctx, u.ID, d, in.deckIn, keys); err != nil {
		s.userFail(w, err, "deck")
		return
	}
	if err := s.store.CreateDeck(ctx, d); err != nil {
		s.fail(w, err, "deck")
		return
	}
	if len(in.Slides) > 0 {
		for i, raw := range in.Slides {
			if i >= 200 {
				break
			}
			var sin slideIn
			if json.Unmarshal(raw, &sin) != nil {
				continue
			}
			sl := &store.Slide{DeckID: d.ID, UserID: u.ID, Position: -1, Layout: "bullets", Source: sourceOf(ctx)}
			if err := s.applySlide(ctx, u.ID, sl, sin); err == nil {
				_ = s.store.CreateSlide(ctx, sl)
			}
		}
	} else if hasTm {
		s.fillTemplate(ctx, u, d, tm)
	}
	s.publish(ctx, u.ID, "slides.deck_created", fmt.Sprintf("deck:%d", d.ID), map[string]any{"deck_id": d.ID, "title": d.Title, "url": s.deckURL(d)})
	d, _ = s.store.GetDeck(ctx, u.ID, d.ID)
	writeJSON(w, http.StatusCreated, s.deckPayload(ctx, d))
}

func strPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Server) fillTemplate(ctx context.Context, u *store.User, d *store.Deck, tm deckTemplate) {
	rep := strings.NewReplacer("{title}", d.Title, "{name}", s.actorName(u), "{date}", time.Now().Format("2 January 2006"))
	for _, t := range tm.slides {
		sl := &store.Slide{DeckID: d.ID, UserID: u.ID, Position: -1, Layout: t.layout, Title: rep.Replace(t.title), Subtitle: rep.Replace(t.subtitle),
			Body: t.body, Body2: t.body2, Notes: t.notes, Style: t.style, Data: cleanData(json.RawMessage(firstNonEmpty(t.data, "{}")))}
		_ = s.store.CreateSlide(ctx, sl)
	}
}

// copyDeck makes a copy of a deck with its slides (pictures shared by id are copied too).
func (s *Server) copyDeck(ctx context.Context, u *store.User, from *store.Deck, title string) (*store.Deck, error) {
	d := *from
	d.ID, d.ShareKey, d.LiveKey, d.Shared, d.Template, d.Pinned, d.Archived = 0, "", "", false, false, false, false
	d.Title, d.CalendarURL, d.TalkAt, d.CreatedAt, d.UpdatedAt, d.Source, d.SourceRef = cleanText(title, 200), "", "", "", "", sourceOf(ctx), ""
	if err := s.store.CreateDeck(ctx, &d); err != nil {
		return nil, err
	}
	slides, err := s.store.Slides(ctx, from.ID)
	if err != nil {
		return nil, err
	}
	copied := map[int64]int64{}
	for _, sl := range slides {
		n := sl
		n.ID, n.DeckID, n.Position, n.Source = 0, d.ID, -1, sourceOf(ctx)
		if sl.ImageID != nil {
			if nid, ok := copied[*sl.ImageID]; ok {
				n.ImageID = &nid
			} else if up, err := s.store.GetUpload(ctx, *sl.ImageID); err == nil && up.UserID == u.ID {
				up.ID, up.DeckID = 0, d.ID
				if s.store.SaveUpload(ctx, up) == nil {
					copied[*sl.ImageID] = up.ID
					n.ImageID = &up.ID
				}
			}
		}
		if err := s.store.CreateSlide(ctx, &n); err != nil {
			return nil, err
		}
	}
	return s.store.GetDeck(ctx, u.ID, d.ID)
}

func (s *Server) deckURL(d *store.Deck) string { return s.link(fmt.Sprintf("/d/%d", d.ID)) }

// deckPayload is a deck with its slides, its own theme and its running show.
func (s *Server) deckPayload(ctx context.Context, d *store.Deck) map[string]any {
	slides, _ := s.store.Slides(ctx, d.ID)
	out := map[string]any{"deck": d, "slides": slides, "share_url": s.link("/s/" + d.ShareKey), "live_url": s.link("/w/" + d.LiveKey), "url": s.deckURL(d)}
	if d.ThemeID != nil {
		if t, err := s.store.GetTheme(ctx, d.UserID, *d.ThemeID); err == nil {
			out["theme"] = t
		}
	}
	if h, err := s.store.LiveShow(ctx, d.ID); err == nil {
		out["show"] = h
		out["remote_url"] = s.link("/r/" + h.RemoteKey)
	}
	return out
}

func (s *Server) handleGetDeck(w http.ResponseWriter, r *http.Request) {
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.deckPayload(r.Context(), d))
}

func (s *Server) handleUpdateDeck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	if !mayChange(ctx, d.Source) {
		writeErr(w, http.StatusForbidden, "this app may change only the decks it made")
		return
	}
	var in deckIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	before := d.TalkAt
	if err := s.applyDeck(ctx, d.UserID, d, in, keys); err != nil {
		s.userFail(w, err, "deck")
		return
	}
	if err := s.store.UpdateDeck(ctx, d); err != nil {
		s.fail(w, err, "deck")
		return
	}
	out := map[string]any{}
	if d.TalkAt != "" && d.TalkAt != before {
		out["talk"] = s.afterTalkSet(ctx, userFrom(ctx), d)
	}
	d, _ = s.store.GetDeck(ctx, d.UserID, d.ID)
	out["deck"] = d
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteDeck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	if !mayChange(ctx, d.Source) {
		writeErr(w, http.StatusForbidden, "this app may remove only the decks it made")
		return
	}
	s.live.endDeck(d.ID)
	if err := s.store.DeleteDeck(ctx, d.UserID, d.ID); err != nil {
		s.fail(w, err, "deck")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCopyDeck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	_ = decode(r, &in)
	n, err := s.copyDeck(ctx, userFrom(ctx), d, firstNonEmpty(cleanText(in.Title, 200), d.Title+" (copy)"))
	if err != nil {
		s.fail(w, err, "copy")
		return
	}
	writeJSON(w, http.StatusCreated, s.deckPayload(ctx, n))
}

// handleShare switches the read-only link on or off, or makes new links.
func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	var in struct {
		Shared  *bool `json:"shared"`
		NewKeys bool  `json:"new_keys"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Shared != nil {
		d.Shared = *in.Shared
	}
	if in.NewKeys {
		d.ShareKey, d.LiveKey = store.RandomToken(9), ""
		for i := 0; i < 5 && d.LiveKey == ""; i++ {
			if k := newLiveKey(); k != "" {
				if _, err := s.store.DeckByKey(ctx, "live", k); err != nil {
					d.LiveKey = k
				}
			}
		}
		if d.LiveKey == "" {
			d.LiveKey = store.RandomToken(6)
		}
	}
	if err := s.store.UpdateDeck(ctx, d); err != nil {
		s.fail(w, err, "share")
		return
	}
	if in.Shared != nil && *in.Shared {
		s.publish(ctx, d.UserID, "slides.shared", fmt.Sprintf("deck:%d", d.ID), map[string]any{"deck_id": d.ID, "title": d.Title, "share_url": s.link("/s/" + d.ShareKey)})
	}
	writeJSON(w, http.StatusOK, s.deckPayload(ctx, d))
}

func newLiveKey() string { return store.ShortKey() }

// ---- slides ----

type slideIn struct {
	Layout   *string         `json:"layout"`
	Title    *string         `json:"title"`
	Subtitle *string         `json:"subtitle"`
	Body     *string         `json:"body"`
	Body2    *string         `json:"body2"`
	ImageID  *int64          `json:"image_id"`
	NoImage  bool            `json:"no_image"`
	Data     json.RawMessage `json:"data"`
	Notes    *string         `json:"notes"`
	Style    *string         `json:"style"`
	Hidden   *bool           `json:"hidden"`
	Seconds  *int            `json:"seconds"`
	Position *int            `json:"position"`
	LayoutID int64           `json:"layout_id"` // start from one of my layouts
	After    int64           `json:"after"`     // put it after this slide
}

func (s *Server) applySlide(ctx context.Context, uid int64, sl *store.Slide, in slideIn) error {
	if in.Layout != nil {
		if !validLayout(*in.Layout) {
			return &userErr{"unknown layout"}
		}
		sl.Layout = *in.Layout
	}
	set := func(dst *string, v *string, n int) {
		if v != nil {
			*dst = clip(strings.TrimRight(*v, " \t\n"), n)
		}
	}
	set(&sl.Title, in.Title, 300)
	set(&sl.Subtitle, in.Subtitle, 300)
	set(&sl.Body, in.Body, 8000)
	set(&sl.Body2, in.Body2, 8000)
	set(&sl.Notes, in.Notes, maxNotes)
	if in.NoImage {
		sl.ImageID = nil
	} else if in.ImageID != nil && *in.ImageID > 0 {
		up, err := s.store.GetUpload(ctx, *in.ImageID)
		if err != nil || up.UserID != uid || (up.DeckID != 0 && up.DeckID != sl.DeckID) {
			return &userErr{"that picture is not in this deck"}
		}
		sl.ImageID = in.ImageID
	}
	if len(in.Data) > 0 && string(in.Data) != "null" {
		sl.Data = cleanData(in.Data)
	}
	if in.Style != nil {
		if !oneOf(*in.Style, styles) {
			return &userErr{"unknown style"}
		}
		sl.Style = *in.Style
	}
	if in.Hidden != nil {
		sl.Hidden = *in.Hidden
	}
	if in.Seconds != nil {
		sl.Seconds = clampInt(*in.Seconds, 0, 3600)
	}
	return nil
}

func (s *Server) handleCreateSlide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	var in slideIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	sl := &store.Slide{DeckID: d.ID, UserID: d.UserID, Position: -1, Layout: "bullets", Source: sourceOf(ctx), Data: json.RawMessage(`{}`)}
	if in.LayoutID > 0 {
		l, err := s.store.GetLayout(ctx, d.UserID, in.LayoutID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		sl.Layout, sl.Title, sl.Subtitle, sl.Body, sl.Body2, sl.Data, sl.Notes, sl.Style = l.Layout, l.Title, l.Subtitle, l.Body, l.Body2, l.Data, l.Notes, l.Style
	}
	if in.Position != nil {
		sl.Position = *in.Position
	}
	if in.After > 0 {
		if a, err := s.store.GetSlide(ctx, d.UserID, in.After); err == nil && a.DeckID == d.ID {
			sl.Position = a.Position + 1
		}
	}
	if err := s.applySlide(ctx, d.UserID, sl, in); err != nil {
		s.userFail(w, err, "slide")
		return
	}
	if d.Slides >= 500 {
		writeErr(w, http.StatusBadRequest, "a deck can have up to 500 slides")
		return
	}
	if err := s.store.CreateSlide(ctx, sl); err != nil {
		s.fail(w, err, "slide")
		return
	}
	s.live.deckChanged(ctx, s, d.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"slide": sl})
}

// slideOf loads one of the caller's slides.
func (s *Server) slideOf(w http.ResponseWriter, r *http.Request) (*store.Slide, bool) {
	ctx := r.Context()
	sl, err := s.store.GetSlide(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return sl, true
}

func (s *Server) handleUpdateSlide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sl, ok := s.slideOf(w, r)
	if !ok {
		return
	}
	if !mayChange(ctx, sl.Source) {
		writeErr(w, http.StatusForbidden, "this app may change only the slides it made")
		return
	}
	var in slideIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.applySlide(ctx, sl.UserID, sl, in); err != nil {
		s.userFail(w, err, "slide")
		return
	}
	if err := s.store.UpdateSlide(ctx, sl); err != nil {
		s.fail(w, err, "slide")
		return
	}
	s.live.deckChanged(ctx, s, sl.DeckID)
	writeJSON(w, http.StatusOK, map[string]any{"slide": sl})
}

func (s *Server) handleDeleteSlide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sl, ok := s.slideOf(w, r)
	if !ok {
		return
	}
	if !mayChange(ctx, sl.Source) {
		writeErr(w, http.StatusForbidden, "this app may remove only the slides it made")
		return
	}
	if err := s.store.DeleteSlide(ctx, sl.UserID, sl.ID); err != nil {
		s.fail(w, err, "slide")
		return
	}
	s.live.deckChanged(ctx, s, sl.DeckID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCopySlide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sl, ok := s.slideOf(w, r)
	if !ok {
		return
	}
	n := *sl
	n.ID, n.Position, n.Source = 0, sl.Position+1, sourceOf(ctx)
	if err := s.store.CreateSlide(ctx, &n); err != nil {
		s.fail(w, err, "slide")
		return
	}
	s.live.deckChanged(ctx, s, sl.DeckID)
	writeJSON(w, http.StatusCreated, map[string]any{"slide": n})
}

func (s *Server) handleReorder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderSlides(ctx, d.ID, in.IDs); err != nil {
		s.fail(w, err, "order")
		return
	}
	s.live.deckChanged(ctx, s, d.ID)
	slides, _ := s.store.Slides(ctx, d.ID)
	writeJSON(w, http.StatusOK, map[string]any{"slides": slides})
}

// ---- my layouts ----

func (s *Server) handleLayouts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.Layouts(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "layouts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"layouts": list})
}

// handleKeepLayout keeps a slide as one of my layouts (its words stay as a starting point).
func (s *Server) handleKeepLayout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sl, ok := s.slideOf(w, r)
	if !ok {
		return
	}
	var in struct {
		Name      string `json:"name"`
		KeepWords *bool  `json:"keep_words"`
	}
	_ = decode(r, &in)
	l := &store.Layout{UserID: sl.UserID, Name: firstNonEmpty(cleanText(in.Name, 80), sl.Title, "My layout"), Layout: sl.Layout, Data: sl.Data, Style: sl.Style}
	if in.KeepWords == nil || *in.KeepWords {
		l.Title, l.Subtitle, l.Body, l.Body2, l.Notes = sl.Title, sl.Subtitle, sl.Body, sl.Body2, sl.Notes
	}
	if err := s.store.CreateLayout(ctx, l); err != nil {
		s.fail(w, err, "layout")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"layout": l})
}

func (s *Server) handleRenameLayout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil || cleanText(in.Name, 80) == "" {
		writeErr(w, http.StatusBadRequest, "a layout needs a name")
		return
	}
	if err := s.store.RenameLayout(ctx, actorID(ctx), idParam(r, "id"), cleanText(in.Name, 80)); err != nil {
		s.fail(w, err, "layout")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteLayout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteLayout(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "layout")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- my themes ----

func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.Themes(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "themes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"themes": list})
}

func (s *Server) handleSaveTheme(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t := &store.Theme{UserID: uid, Base: "paper"}
	if id := idParam(r, "id"); id > 0 {
		cur, err := s.store.GetTheme(ctx, uid, id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		t = cur
	}
	var in struct {
		Name     *string `json:"name"`
		Base     *string `json:"base"`
		BG       *string `json:"bg"`
		BG2      *string `json:"bg2"`
		FG       *string `json:"fg"`
		Muted    *string `json:"muted"`
		Accent   *string `json:"accent"`
		HeadFont *string `json:"head_font"`
		BodyFont *string `json:"body_font"`
		LogoID   *int64  `json:"logo_id"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil {
		t.Name = cleanText(*in.Name, 60)
	}
	if t.Name == "" {
		t.Name = "My theme"
	}
	if in.Base != nil {
		if !validTheme(*in.Base) {
			writeErr(w, http.StatusBadRequest, "unknown theme")
			return
		}
		t.Base = *in.Base
	}
	for _, c := range []struct {
		dst *string
		v   *string
	}{{&t.BG, in.BG}, {&t.BG2, in.BG2}, {&t.FG, in.FG}, {&t.Muted, in.Muted}, {&t.Accent, in.Accent}} {
		if c.v == nil {
			continue
		}
		v := strings.TrimSpace(*c.v)
		if v != "" && !reColor.MatchString(v) {
			writeErr(w, http.StatusBadRequest, "colours are written like #1a2b3c")
			return
		}
		*c.dst = strings.ToLower(v)
	}
	for _, f := range []struct {
		dst *string
		v   *string
	}{{&t.HeadFont, in.HeadFont}, {&t.BodyFont, in.BodyFont}} {
		if f.v != nil {
			if !oneOf(*f.v, fonts) {
				writeErr(w, http.StatusBadRequest, "unknown font")
				return
			}
			*f.dst = *f.v
		}
	}
	if keys["logo_id"] {
		if in.LogoID == nil || *in.LogoID == 0 {
			t.LogoID = nil
		} else if up, err := s.store.GetUpload(ctx, *in.LogoID); err != nil || up.UserID != uid || !strings.HasPrefix(up.Mime, "image/") {
			writeErr(w, http.StatusBadRequest, "that picture is not yours")
			return
		} else {
			t.LogoID = in.LogoID
		}
	}
	if err := s.store.SaveTheme(ctx, t); err != nil {
		s.fail(w, err, "theme")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"theme": t})
}

func (s *Server) handleDeleteTheme(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteTheme(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "theme")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- shows: talks and rehearsals ----

func (s *Server) handleShows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	deckID := int64(queryInt(r, "deck"))
	if deckID > 0 {
		if _, err := s.store.GetDeck(ctx, uid, deckID); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
	}
	list, err := s.store.Shows(ctx, uid, deckID, clampInt(firstPos(queryInt(r, "limit"), 50), 1, 500))
	if err != nil {
		s.fail(w, err, "shows")
		return
	}
	for i := range list {
		list[i].RemoteKey = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"shows": list})
}

func (s *Server) handleGetShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h, err := s.store.GetShow(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	qs, _ := s.store.Questions(ctx, h.ID)
	fb, _ := s.store.FeedbackOf(ctx, h.ID)
	tally, _ := s.store.Tally(ctx, h.ID)
	slides, _ := s.store.Slides(ctx, h.DeckID)
	if h.EndedAt != nil {
		h.RemoteKey = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"show": h, "questions": qs, "feedback": fb, "tally": tallyJSON(tally), "slides": slides})
}

func (s *Server) handleUpdateShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h, err := s.store.GetShow(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var in struct {
		Note *string `json:"note"`
		Kind *string `json:"kind"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Note != nil {
		h.Note = cleanText(*in.Note, 4000)
	}
	if in.Kind != nil && oneOf(*in.Kind, []string{store.KindTalk, store.KindRehearsal}) {
		h.Kind = *in.Kind
	}
	if err := s.store.SaveShow(ctx, h); err != nil {
		s.fail(w, err, "show")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"show": h})
}

func (s *Server) handleDeleteShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h, err := s.store.GetShow(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if h.EndedAt == nil {
		s.live.endDeck(h.DeckID)
	}
	if err := s.store.DeleteShow(ctx, h.UserID, h.ID); err != nil {
		s.fail(w, err, "show")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMarkQuestion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, err := s.store.GetQuestion(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var in struct {
		Answered *bool `json:"answered"`
		Hidden   *bool `json:"hidden"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Answered != nil {
		q.Answered = *in.Answered
	}
	if in.Hidden != nil {
		q.Hidden = *in.Hidden
	}
	if err := s.store.MarkQuestion(ctx, q); err != nil {
		s.fail(w, err, "question")
		return
	}
	if h, err := s.store.GetShow(ctx, q.UserID, q.ShowID); err == nil && h.EndedAt == nil {
		s.live.broadcast(ctx, s, h.DeckID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"question": q})
}

func tallyJSON(t map[int64]map[int]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for sid, m := range t {
		mm := map[string]int{}
		for c, n := range m {
			mm[fmt.Sprint(c)] = n
		}
		out[fmt.Sprint(sid)] = mm
	}
	return out
}
