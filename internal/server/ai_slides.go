package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// Pocket AI (or the person's own OpenAI-compatible key) drafts decks from a
// topic, a note or a voice memo, writes speaker notes, makes a slide shorter
// and says what to fix before the talk. The person's own words win: AI
// suggests, the page shows it, nothing is changed without a press.

func (s *Server) aiClient(w http.ResponseWriter, r *http.Request) (*ai.Client, bool) {
	ctx := r.Context()
	cl, err := s.aiReady(ctx, actorID(ctx))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	select {
	case s.aiSem <- struct{}{}:
	default:
		writeErr(w, http.StatusTooManyRequests, "the AI is busy: try again in a moment")
		return nil, false
	}
	return cl, true
}

func (s *Server) aiReady(ctx context.Context, uid int64) (*ai.Client, error) {
	st := s.settingsOf(ctx, uid)
	if !st.AIHelp {
		return nil, errors.New("AI help is switched off in Settings → AI")
	}
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		return nil, errors.New(firstNonEmpty(why, "connect Pocket AI or an AI key in Settings → AI"))
	}
	return cl, nil
}

func (s *Server) aiDone() { <-s.aiSem }

const layoutGuide = `Layouts you may use (field "layout"):
- "title": first slide. title + subtitle.
- "section": starts a part. title (+ subtitle).
- "bullets": title + body (3 to 5 short points, each line starting "- ").
- "two": title + body and body2 (two short lists), data.left / data.right are the column headings.
- "number": title is one big number or short figure ("3 in 4", "€2 m"), subtitle what it means, body one line where it comes from.
- "quote": body is the quote, subtitle who said it.
- "chart": title + data.chart {"kind":"bar"|"line"|"pie","labels":[...],"values":[...],"unit":""} only with real numbers you were given.
- "poll": title is a question for the audience, data.options 2 to 5 short answers.
- "agenda": title only; it lists the section slides by itself.
- "end": title (e.g. "Thank you!") + subtitle (e.g. "Questions?").
Points are short (under 9 words), no full sentences on slides. What the speaker says goes in "notes" (2 to 5 plain sentences).`

type aiSlide struct {
	Layout   string          `json:"layout"`
	Title    string          `json:"title"`
	Subtitle string          `json:"subtitle"`
	Body     string          `json:"body"`
	Body2    string          `json:"body2"`
	Notes    string          `json:"notes"`
	Data     json.RawMessage `json:"data"`
}

// aiOutline asks the AI for slides.
func (s *Server) aiOutline(ctx context.Context, u *store.User, task, material string, minutes int) ([]draftSlide, error) {
	cl, err := s.aiReady(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	select {
	case s.aiSem <- struct{}{}:
	default:
		return nil, errors.New("the AI is busy: try again in a moment")
	}
	defer s.aiDone()
	n := "8 to 14"
	if minutes > 0 {
		n = fmt.Sprintf("about %d", clampInt(minutes/2+2, 4, 40))
	}
	system := "You make presentation slides for " + s.actorName(u) + ". " + layoutGuide +
		"\nAnswer with JSON only: {\"title\": \"deck title\", \"slides\": [{\"layout\":..., \"title\":..., \"subtitle\":..., \"body\":..., \"body2\":..., \"notes\":..., \"data\": {...}}]}. Make " + n +
		" slides, start with a title slide and end with an end slide. Write in the language of the material."
	var out struct {
		Title  string    `json:"title"`
		Slides []aiSlide `json:"slides"`
	}
	if err := cl.JSON(ctx, system, task+"\n\n"+material, &out); err != nil {
		return nil, err
	}
	drafts := []draftSlide{}
	for _, a := range out.Slides {
		if !validLayout(a.Layout) {
			a.Layout = "bullets"
		}
		drafts = append(drafts, draftSlide{Layout: a.Layout, Title: a.Title, Subtitle: a.Subtitle, Body: a.Body, Body2: a.Body2, Notes: a.Notes, Data: string(cleanData(a.Data))})
	}
	if len(drafts) == 0 {
		return nil, errors.New("the AI did not make any slides; try again with a few more words")
	}
	if out.Title != "" && drafts[0].Layout == "title" && drafts[0].Title == "" {
		drafts[0].Title = out.Title
	}
	return drafts, nil
}

// handleAIDraft makes a new deck from a topic (and who it is for, and how long).
func (s *Server) handleAIDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Topic    string `json:"topic"`
		Audience string `json:"audience"`
		Minutes  int    `json:"minutes"`
		Points   string `json:"points"`
		Theme    string `json:"theme"`
	}
	if err := decode(r, &in); err != nil || cleanText(in.Topic, 500) == "" {
		writeErr(w, http.StatusBadRequest, "say what the talk is about")
		return
	}
	u := userFrom(ctx)
	task := "Make the slides for a talk about: " + cleanText(in.Topic, 500)
	if a := cleanText(in.Audience, 300); a != "" {
		task += "\nThe audience: " + a
	}
	if in.Minutes > 0 {
		task += fmt.Sprintf("\nIt takes %d minutes.", clampInt(in.Minutes, 1, 300))
	}
	drafts, err := s.aiOutline(ctx, u, task, "What the speaker wants to say:\n"+cleanText(in.Points, 20000), in.Minutes)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	title := cleanText(in.Topic, 120)
	if len(drafts) > 0 && drafts[0].Layout == "title" && drafts[0].Title != "" {
		title = drafts[0].Title
	}
	d, err := s.makeDeck(ctx, u, title, drafts, "", "")
	if err != nil {
		s.fail(w, err, "deck")
		return
	}
	if in.Minutes > 0 || validTheme(in.Theme) {
		d.TargetMinutes = clampInt(in.Minutes, 0, 600)
		if validTheme(in.Theme) {
			d.Theme = in.Theme
		}
		_ = s.store.UpdateDeck(ctx, d)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deck": d, "text": fmt.Sprintf("A first draft of %d %s", d.Slides, plural(d.Slides, "slide", "slides"))})
}

// handleAISlide suggests a better version of one slide: shorter, clearer,
// with speaker notes, or split into points. It changes nothing by itself.
func (s *Server) handleAISlide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		SlideID int64  `json:"slide_id"`
		Want    string `json:"want"` // shorter | clearer | notes | points | title | ideas
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	sl, err := s.store.GetSlide(ctx, actorID(ctx), in.SlideID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	d, _ := s.store.GetDeck(ctx, sl.UserID, sl.DeckID)
	cl, ok := s.aiClient(w, r)
	if !ok {
		return
	}
	defer s.aiDone()
	asks := map[string]string{
		"shorter": "Make the slide shorter: fewer, shorter points (under 8 words each), keep the meaning. Move what is cut into the notes.",
		"clearer": "Make the slide clearer for the audience: plain words, one idea per point, a heading that says the point.",
		"notes":   "Write the speaker notes for this slide: what to say, 2 to 5 plain sentences, in the speaker's voice. Leave the slide itself as it is.",
		"points":  "Turn the slide's text into 3 to 5 short points.",
		"title":   "Suggest a better heading: short, saying the point of the slide. Leave the rest as it is.",
		"ideas":   "Suggest a better layout for what this slide says, and fill it in.",
	}
	ask, ok := asks[in.Want]
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown kind of help")
		return
	}
	cur, _ := json.Marshal(map[string]any{"layout": sl.Layout, "title": sl.Title, "subtitle": sl.Subtitle, "body": sl.Body, "body2": sl.Body2, "notes": sl.Notes, "data": sl.Data})
	system := "You help " + s.actorName(userFrom(ctx)) + " improve one slide of the deck “" + d.Title + "”. " + layoutGuide +
		"\nAnswer with JSON only: the whole slide as {\"layout\",\"title\",\"subtitle\",\"body\",\"body2\",\"notes\",\"data\"}. Keep the language of the slide."
	var out aiSlide
	if err := cl.JSON(ctx, system, ask+"\n\nThe slide:\n"+string(cur), &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if !validLayout(out.Layout) {
		out.Layout = sl.Layout
	}
	out.Data = cleanData(out.Data)
	writeJSON(w, http.StatusOK, map[string]any{"suggestion": out})
}

// handleAINotes writes speaker notes for every slide of a deck that has none.
func (s *Server) handleAINotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	type item struct {
		ID       int64  `json:"id"`
		Layout   string `json:"layout"`
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		Body     string `json:"body"`
		Body2    string `json:"body2"`
		Notes    string `json:"notes"`
	}
	list := []item{}
	missing := 0
	for _, sl := range slides {
		list = append(list, item{sl.ID, sl.Layout, sl.Title, sl.Subtitle, clip(sl.Body, 1500), clip(sl.Body2, 1500), clip(sl.Notes, 1500)})
		if strings.TrimSpace(sl.Notes) == "" {
			missing++
		}
	}
	if missing == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "written": 0, "text": "Every slide already has notes"})
		return
	}
	cl, ok := s.aiClient(w, r)
	if !ok {
		return
	}
	defer s.aiDone()
	b, _ := json.Marshal(list)
	mins := ""
	if d.TargetMinutes > 0 {
		mins = fmt.Sprintf(" The talk takes %d minutes in all, so share the time out.", d.TargetMinutes)
	}
	system := "You write speaker notes for " + s.actorName(userFrom(ctx)) + "'s talk “" + d.Title + "”: for each slide whose notes are empty, what to say (2 to 5 plain sentences, spoken style, not reading the slide out)." + mins +
		" Answer with JSON only: {\"notes\": [{\"id\": slide id, \"notes\": \"...\"}]} for the slides with empty notes. Write in the language of the slides."
	var out struct {
		Notes []struct {
			ID    int64  `json:"id"`
			Notes string `json:"notes"`
		} `json:"notes"`
	}
	if err := cl.JSON(ctx, system, string(b), &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	n := 0
	for _, x := range out.Notes {
		for i := range slides {
			if slides[i].ID == x.ID && strings.TrimSpace(slides[i].Notes) == "" && strings.TrimSpace(x.Notes) != "" {
				slides[i].Notes = clip(strings.TrimSpace(x.Notes), maxNotes)
				if s.store.UpdateSlide(ctx, &slides[i]) == nil {
					n++
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "written": n, "text": fmt.Sprintf("Notes written for %d %s", n, plural(n, "slide", "slides"))})
}

// handleAICheck reads a deck the way a friendly listener would and says what to fix.
func (s *Server) handleAICheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	cl, ok := s.aiClient(w, r)
	if !ok {
		return
	}
	defer s.aiDone()
	md := s.deckMarkdown(ctx, d, true)
	shows, _ := s.store.Shows(ctx, d.UserID, d.ID, 3)
	timing := ""
	for _, h := range shows {
		if h.EndedAt != nil {
			timing += fmt.Sprintf("- a %s took %.1f minutes\n", h.Kind, float64(h.TotalMs)/60000)
		}
	}
	if d.TargetMinutes > 0 {
		timing += fmt.Sprintf("The talk should take %d minutes.\n", d.TargetMinutes)
	}
	system := "You are a kind, practical speaking coach. Read the deck (slides, then the speaker's notes) and say what would make the talk better: at most 6 points, " +
		"each naming the slide, plain words, most useful first. Mention slides with too many words, a missing story or example, a weak start or end, and the timing if it is off. " +
		"Answer with JSON only: {\"summary\": \"one sentence\", \"tips\": [{\"slide\": slide number or 0, \"tip\": \"...\"}]}. Write in the language of the deck."
	var out struct {
		Summary string `json:"summary"`
		Tips    []struct {
			Slide int    `json:"slide"`
			Tip   string `json:"tip"`
		} `json:"tips"`
	}
	if err := cl.JSON(ctx, system, clip(md, 60000)+"\n\nRehearsals so far:\n"+firstNonEmpty(timing, "none yet\n"), &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
