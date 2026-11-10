package server

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// The export is everything a person made: their decks with their slides and
// pictures, their own themes and layouts, and the talks and rehearsals with
// what the audience asked and thought.

type exportFile struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Mime  string `json:"mime"`
	W     int    `json:"width,omitempty"`
	H     int    `json:"height,omitempty"`
	Data  string `json:"data"`
	Thumb string `json:"thumb,omitempty"`
}

type exportShow struct {
	Show      store.Show                `json:"show"`
	Questions []store.Question          `json:"questions"`
	Feedback  []store.Feedback          `json:"feedback"`
	Tally     map[string]map[string]int `json:"tally"`
}

type exportDeck struct {
	Deck   store.Deck    `json:"deck"`
	Slides []store.Slide `json:"slides"`
	Shows  []exportShow  `json:"shows"`
}

type exportDoc struct {
	App        string         `json:"app"`
	Version    string         `json:"version"`
	ExportedAt string         `json:"exported_at"`
	Decks      []exportDeck   `json:"decks"`
	Themes     []store.Theme  `json:"themes"`
	Layouts    []store.Layout `json:"layouts"`
	Files      []exportFile   `json:"files"`
}

func (s *Server) export(ctx context.Context, u *store.User) (*exportDoc, error) {
	doc := &exportDoc{App: "pocket-slides", Version: Version, ExportedAt: store.Now(), Decks: []exportDeck{}, Files: []exportFile{}}
	files := map[int64]bool{}
	addFile := func(id *int64) {
		if id == nil || files[*id] {
			return
		}
		up, err := s.store.GetUpload(ctx, *id)
		if err != nil || up.UserID != u.ID {
			return
		}
		files[*id] = true
		f := exportFile{ID: up.ID, Name: up.Name, Mime: up.Mime, W: up.Width, H: up.Height, Data: base64.StdEncoding.EncodeToString(up.Data)}
		if len(up.Thumb) > 0 {
			f.Thumb = base64.StdEncoding.EncodeToString(up.Thumb)
		}
		doc.Files = append(doc.Files, f)
	}
	for _, archived := range []bool{false, true} {
		list, err := s.store.Decks(ctx, u.ID, archived)
		if err != nil {
			return nil, err
		}
		for _, d := range list {
			ed := exportDeck{Deck: d, Shows: []exportShow{}}
			ed.Slides, _ = s.store.Slides(ctx, d.ID)
			for _, sl := range ed.Slides {
				addFile(sl.ImageID)
			}
			shows, _ := s.store.Shows(ctx, u.ID, d.ID, 10000)
			for _, h := range shows {
				h.RemoteKey = ""
				es := exportShow{Show: h}
				es.Questions, _ = s.store.Questions(ctx, h.ID)
				es.Feedback, _ = s.store.FeedbackOf(ctx, h.ID)
				t, _ := s.store.Tally(ctx, h.ID)
				es.Tally = tallyJSON(t)
				ed.Shows = append(ed.Shows, es)
			}
			doc.Decks = append(doc.Decks, ed)
		}
	}
	doc.Themes, _ = s.store.Themes(ctx, u.ID)
	for _, t := range doc.Themes {
		addFile(t.LogoID)
	}
	doc.Layouts, _ = s.store.Layouts(ctx, u.ID)
	return doc, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	doc, err := s.export(r.Context(), userFrom(r.Context()))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-slides-%s.json"`, time.Now().Format("2006-01-02")))
	_ = json.NewEncoder(w).Encode(doc)
}

// handleExportCSV is one row per talk or rehearsal.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-slides-%s.csv"`, time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"kind", "deck", "started", "minutes", "target_minutes", "slides", "viewers", "reactions", "questions", "votes", "feedback", "rating", "note"})
	decks := map[int64]store.Deck{}
	for _, archived := range []bool{false, true} {
		list, _ := s.store.Decks(ctx, uid, archived)
		for _, d := range list {
			decks[d.ID] = d
		}
	}
	shows, _ := s.store.Shows(ctx, uid, 0, 100000)
	for _, h := range shows {
		d := decks[h.DeckID]
		t, _ := time.Parse(time.RFC3339Nano, h.StartedAt)
		_ = cw.Write([]string{h.Kind, d.Title, t.Local().Format("2006-01-02 15:04"), fmt.Sprintf("%.1f", float64(h.TotalMs)/60000), strconv.Itoa(d.TargetMinutes),
			strconv.Itoa(d.Slides), strconv.Itoa(h.Viewers), strconv.Itoa(h.Reactions), strconv.Itoa(h.Questions), strconv.Itoa(h.Votes), strconv.Itoa(h.Feedback),
			fmt.Sprintf("%.1f", h.Rating), h.Note})
	}
	cw.Flush()
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var doc exportDoc
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<30)).Decode(&doc); err != nil {
		writeErr(w, http.StatusBadRequest, "that is not a Pocket Slides export")
		return
	}
	if doc.App != "pocket-slides" {
		writeErr(w, http.StatusBadRequest, "that is not a Pocket Slides export")
		return
	}
	res, err := s.importDoc(ctx, userFrom(ctx), &doc)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// importDoc adds what is not here yet: a deck imported before (same origin
// id, kept as its source_ref) is skipped, so importing twice changes nothing.
func (s *Server) importDoc(ctx context.Context, u *store.User, doc *exportDoc) (map[string]any, error) {
	made, skipped, slides, shows := 0, 0, 0, 0
	files := map[int64]exportFile{}
	for _, f := range doc.Files {
		files[f.ID] = f
	}
	newFile := map[int64]int64{}
	saveFile := func(old *int64, deck int64) *int64 {
		if old == nil {
			return nil
		}
		if id, ok := newFile[*old]; ok {
			return &id
		}
		f, ok := files[*old]
		if !ok {
			return nil
		}
		data, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil || !allowedMime[f.Mime] {
			return nil
		}
		up := &store.Upload{UserID: u.ID, DeckID: deck, Name: f.Name, Mime: f.Mime, Data: data, Width: f.W, Height: f.H}
		if f.Thumb != "" {
			up.Thumb, _ = base64.StdEncoding.DecodeString(f.Thumb)
		}
		if s.store.SaveUpload(ctx, up) != nil {
			return nil
		}
		newFile[*old] = up.ID
		return &up.ID
	}
	themes := map[int64]int64{}
	have, _ := s.store.Themes(ctx, u.ID)
	for _, t := range doc.Themes {
		found := int64(0)
		for _, h := range have {
			if h.Name == t.Name && h.BG == t.BG && h.Accent == t.Accent && h.FG == t.FG {
				found = h.ID
			}
		}
		if found == 0 {
			old := t.ID
			t.ID, t.UserID = 0, u.ID
			t.LogoID = saveFile(t.LogoID, 0)
			if !validTheme(t.Base) {
				t.Base = "paper"
			}
			if err := s.store.SaveTheme(ctx, &t); err != nil {
				return nil, err
			}
			found = t.ID
			themes[old] = found
		} else {
			themes[t.ID] = found
		}
	}
	lays, _ := s.store.Layouts(ctx, u.ID)
	for _, l := range doc.Layouts {
		dup := false
		for _, h := range lays {
			if h.Name == l.Name && h.Layout == l.Layout && h.Body == l.Body && h.Title == l.Title {
				dup = true
			}
		}
		if dup || !validLayout(l.Layout) {
			continue
		}
		l.ID, l.UserID = 0, u.ID
		l.Data = cleanData(l.Data)
		if err := s.store.CreateLayout(ctx, &l); err != nil {
			return nil, err
		}
	}
	for _, ed := range doc.Decks {
		ref := fmt.Sprintf("import:%d:%s", ed.Deck.ID, ed.Deck.CreatedAt)
		if _, err := s.store.DeckBySource(ctx, u.ID, "import", ref); err == nil {
			skipped++
			continue
		}
		d := ed.Deck
		oldDeck := d.ID
		d.ID, d.UserID, d.ShareKey, d.LiveKey, d.Shared, d.Source, d.SourceRef = 0, u.ID, "", "", false, "import", ref
		if !validTheme(d.Theme) {
			d.Theme = "paper"
		}
		if !oneOf(d.Aspect, aspects) {
			d.Aspect = "16:9"
		}
		if !oneOf(d.Transition, transitions) {
			d.Transition = "fade"
		}
		if d.ThemeID != nil {
			if id, ok := themes[*d.ThemeID]; ok {
				d.ThemeID = &id
			} else {
				d.ThemeID = nil
			}
		}
		if err := s.store.CreateDeck(ctx, &d); err != nil {
			return nil, err
		}
		made++
		slideIDs := map[int64]int64{}
		for _, sl := range ed.Slides {
			if sl.DeckID != oldDeck && sl.DeckID != 0 {
				continue
			}
			old := sl.ID
			sl.ID, sl.DeckID, sl.UserID, sl.Position, sl.Source = 0, d.ID, u.ID, -1, ""
			if !validLayout(sl.Layout) {
				sl.Layout = "bullets"
			}
			if !oneOf(sl.Style, styles) {
				sl.Style = ""
			}
			sl.Data = cleanData(sl.Data)
			sl.ImageID = saveFile(sl.ImageID, d.ID)
			if err := s.store.CreateSlide(ctx, &sl); err != nil {
				return nil, err
			}
			slideIDs[old] = sl.ID
			slides++
		}
		for _, es := range ed.Shows {
			h := es.Show
			h.ID, h.UserID, h.DeckID, h.RemoteKey = 0, u.ID, d.ID, ""
			if h.EndedAt == nil {
				at := h.StartedAt
				h.EndedAt = &at
			}
			if !oneOf(h.Kind, []string{store.KindTalk, store.KindRehearsal}) {
				h.Kind = store.KindTalk
			}
			tm := map[string]int64{}
			for k, v := range h.Timings {
				if old, err := strconv.ParseInt(k, 10, 64); err == nil {
					if nid, ok := slideIDs[old]; ok {
						tm[strconv.FormatInt(nid, 10)] = v
					}
				}
			}
			h.Timings = tm
			if err := s.store.StartShow(ctx, &h); err != nil {
				return nil, err
			}
			shows++
			for _, q := range es.Questions {
				q.ShowID, q.UserID = h.ID, u.ID
				if q.SlideID != nil {
					if nid, ok := slideIDs[*q.SlideID]; ok {
						q.SlideID = &nid
					} else {
						q.SlideID = nil
					}
				}
				_ = s.store.AddQuestion(ctx, &q)
			}
			for i, f := range es.Feedback {
				f.ShowID, f.UserID = h.ID, u.ID
				_ = s.store.AddFeedback(ctx, &f, fmt.Sprintf("import-%d-%d", h.ID, i))
			}
			n := 0
			for sid, m := range es.Tally {
				old, _ := strconv.ParseInt(sid, 10, 64)
				nid, ok := slideIDs[old]
				if !ok {
					continue
				}
				for c, k := range m {
					ci, _ := strconv.Atoi(c)
					for j := 0; j < k && j < 10000; j++ {
						n++
						_ = s.store.Vote(ctx, u.ID, h.ID, nid, ci, fmt.Sprintf("import-%d-%d", h.ID, n))
					}
				}
			}
		}
	}
	return map[string]any{"ok": true, "decks": made, "skipped": skipped, "slides": slides, "shows": shows,
		"text": fmt.Sprintf("%d %s brought in (%d already here)", made, plural(made, "deck", "decks"), skipped)}, nil
}

// ---- one deck as Markdown: the outline with the speaker notes ----

func (s *Server) deckMarkdown(ctx context.Context, d *store.Deck, notes bool) string {
	slides, _ := s.store.Slides(ctx, d.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: %q\n---\n\n", d.Title)
	for i, sl := range slides {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		if sl.Hidden {
			b.WriteString("<!-- hidden slide -->\n")
		}
		switch sl.Layout {
		case "title", "section":
			fmt.Fprintf(&b, "# %s\n", firstNonEmpty(sl.Title, "(no title)"))
			if sl.Subtitle != "" {
				fmt.Fprintf(&b, "\n%s\n", sl.Subtitle)
			}
		case "quote":
			for _, l := range strings.Split(sl.Body, "\n") {
				fmt.Fprintf(&b, "> %s\n", l)
			}
			if sl.Subtitle != "" {
				fmt.Fprintf(&b, "> — %s\n", sl.Subtitle)
			}
		case "code":
			if sl.Title != "" {
				fmt.Fprintf(&b, "## %s\n\n", sl.Title)
			}
			fmt.Fprintf(&b, "```%s\n%s\n```\n", dataOf(&sl).Lang, sl.Body)
		case "poll":
			fmt.Fprintf(&b, "## %s\n\n", sl.Title)
			for _, o := range dataOf(&sl).Options {
				fmt.Fprintf(&b, "- [ ] %s\n", o)
			}
		case "chart":
			fmt.Fprintf(&b, "## %s\n\n", sl.Title)
			if c := dataOf(&sl).Chart; c != nil {
				for i, l := range c.Labels {
					fmt.Fprintf(&b, "- %s: %s %s\n", l, strconv.FormatFloat(c.Values[i], 'f', -1, 64), c.Unit)
				}
			}
		case "table":
			fmt.Fprintf(&b, "## %s\n\n", sl.Title)
			rows := dataOf(&sl).Rows
			for i, r := range rows {
				fmt.Fprintf(&b, "| %s |\n", strings.Join(r, " | "))
				if i == 0 {
					sep := make([]string, len(r))
					for j := range sep {
						sep[j] = "---"
					}
					fmt.Fprintf(&b, "| %s |\n", strings.Join(sep, " | "))
				}
			}
		case "number":
			fmt.Fprintf(&b, "## %s\n\n**%s**", sl.Title, sl.Subtitle)
			if sl.Body != "" {
				fmt.Fprintf(&b, "\n\n%s", sl.Body)
			}
			b.WriteString("\n")
		default:
			if sl.Title != "" {
				fmt.Fprintf(&b, "## %s\n\n", sl.Title)
			}
			if sl.Subtitle != "" {
				fmt.Fprintf(&b, "### %s\n\n", sl.Subtitle)
			}
			if sl.Body != "" {
				fmt.Fprintf(&b, "%s\n", sl.Body)
			}
			if sl.Body2 != "" {
				fmt.Fprintf(&b, "\n%s\n", sl.Body2)
			}
		}
		if sl.ImageID != nil {
			fmt.Fprintf(&b, "\n![picture](%s)\n", s.link(fmt.Sprintf("/api/v1/uploads/%d", *sl.ImageID)))
		}
		if notes && strings.TrimSpace(sl.Notes) != "" {
			fmt.Fprintf(&b, "\nNotes:\n%s\n", strings.TrimSpace(sl.Notes))
		}
	}
	return b.String()
}

func (s *Server) handleDeckMarkdown(w http.ResponseWriter, r *http.Request) {
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, fileSafe(d.Title)))
	_, _ = w.Write([]byte(s.deckMarkdown(r.Context(), d, !queryBool(r, "no_notes"))))
}

func fileSafe(v string) string {
	return firstNonEmpty(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, clip(strings.TrimSpace(v), 90)), "deck")
}
