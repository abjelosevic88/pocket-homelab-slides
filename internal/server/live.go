package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// Presenting. A deck being presented has one running show. Every screen
// following it — the big screen, the presenter screen on a laptop or a phone,
// the audience's phones — keeps an event stream open; a press on any
// controlling screen moves the show and every screen follows at once.
//
// The audience sees only talks (never rehearsals), only the slide on the
// screen and the ones before it, never the speaker notes and never a quiz
// answer before the presenter shows it.

type liveSub struct {
	ch       chan []byte
	audience bool
}

type liveHub struct {
	mu     sync.Mutex
	subs   map[int64]map[*liveSub]bool // by deck
	since  map[int64]time.Time         // by show: when the slide on the screen came up
	likes  map[string]bool             // "show:question:voter"
	reacts map[string]time.Time        // "show:voter" → last reaction
}

func newLiveHub() *liveHub {
	return &liveHub{subs: map[int64]map[*liveSub]bool{}, since: map[int64]time.Time{}, likes: map[string]bool{}, reacts: map[string]time.Time{}}
}

func (h *liveHub) sub(deck int64, audience bool) *liveSub {
	h.mu.Lock()
	defer h.mu.Unlock()
	sb := &liveSub{ch: make(chan []byte, 16), audience: audience}
	if h.subs[deck] == nil {
		h.subs[deck] = map[*liveSub]bool{}
	}
	h.subs[deck][sb] = true
	return sb
}

func (h *liveHub) unsub(deck int64, sb *liveSub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[deck], sb)
	if len(h.subs[deck]) == 0 {
		delete(h.subs, deck)
	}
}

func (h *liveHub) viewers(deck int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for sb := range h.subs[deck] {
		if sb.audience {
			n++
		}
	}
	return n
}

func (h *liveHub) send(deck int64, presenter, audience []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sb := range h.subs[deck] {
		b := presenter
		if sb.audience {
			b = audience
		}
		if b == nil {
			continue
		}
		select {
		case sb.ch <- b:
		default:
		}
	}
}

// endDeck tells every screen of a deck that the show is over (the deck went away).
func (h *liveHub) endDeck(deck int64) {
	b := []byte(`{"type":"ended"}`)
	h.send(deck, b, b)
}

// deckChanged tells the screens to load the slides again (they were edited).
func (h *liveHub) deckChanged(ctx context.Context, s *Server, deck int64) {
	h.mu.Lock()
	n := len(h.subs[deck])
	h.mu.Unlock()
	if n == 0 {
		return
	}
	b := []byte(`{"type":"deck"}`)
	h.send(deck, b, nil)
	go s.live.broadcast(context.WithoutCancel(ctx), s, deck)
}

// visible are the slides shown when presenting, in order.
func visible(all []store.Slide) []store.Slide {
	out := []store.Slide{}
	for _, sl := range all {
		if !sl.Hidden {
			out = append(out, sl)
		}
	}
	return out
}

// forAudience is a slide as the audience may see it: no notes, and the
// quiz answer only once it is shown.
func forAudience(sl store.Slide, answerShown bool) store.Slide {
	sl.Notes, sl.Source = "", ""
	if sl.Layout == "poll" && !answerShown {
		d := dataOf(&sl)
		if d.Correct > 0 {
			d.Correct = 0
			b, _ := json.Marshal(d)
			sl.Data = b
		}
	}
	return sl
}

// liveState is what every screen gets after each change.
type liveState struct {
	Type      string                    `json:"type"`
	Live      bool                      `json:"live"`
	ShowID    int64                     `json:"show_id,omitempty"`
	Kind      string                    `json:"kind,omitempty"`
	Pos       int                       `json:"pos"`
	Step      int                       `json:"step"`
	Steps     int                       `json:"steps"`
	Black     bool                      `json:"black"`
	Total     int                       `json:"total"`
	SlideID   int64                     `json:"slide_id,omitempty"`
	StartedAt string                    `json:"started_at,omitempty"`
	SlideAt   string                    `json:"slide_at,omitempty"` // when the slide on the screen came up
	Viewers   int                       `json:"viewers"`
	Reactions int                       `json:"reactions"`
	Tally     map[string]map[string]int `json:"tally"`
	Questions []store.Question          `json:"questions"`
	Asking    bool                      `json:"asking"` // the audience may ask
	Slide     *store.Slide              `json:"slide,omitempty"`
	Feedback  bool                      `json:"feedback,omitempty"`
}

// state works out the live state of a deck for the presenter and for the audience.
func (s *Server) state(ctx context.Context, d *store.Deck) (pres, aud liveState) {
	pres = liveState{Type: "state", Tally: map[string]map[string]int{}, Questions: []store.Question{}, Asking: d.Questions}
	aud = pres
	h, err := s.store.LiveShow(ctx, d.ID)
	if err != nil {
		if last, err := s.store.LatestShow(ctx, d.ID, store.KindTalk); err == nil && last.EndedAt != nil && s.settingsOf(ctx, d.UserID).Feedback {
			if t, err := time.Parse(time.RFC3339Nano, *last.EndedAt); err == nil && time.Since(t) < 48*time.Hour {
				aud.Feedback = true
			}
		}
		return
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	vis := visible(slides)
	pres.Live, pres.ShowID, pres.Kind, pres.Pos, pres.Step, pres.Black, pres.Total = true, h.ID, h.Kind, h.Pos, h.Step, h.Black, len(vis)
	pres.StartedAt, pres.Viewers, pres.Reactions = h.StartedAt, s.live.viewers(d.ID), h.Reactions
	s.live.mu.Lock()
	if t, ok := s.live.since[h.ID]; ok {
		pres.SlideAt = t.UTC().Format(time.RFC3339Nano)
	}
	s.live.mu.Unlock()
	var cur *store.Slide
	if h.Pos >= 0 && h.Pos < len(vis) {
		cur = &vis[h.Pos]
		pres.SlideID, pres.Steps = cur.ID, steps(cur)
	}
	tally, _ := s.store.Tally(ctx, h.ID)
	pres.Tally = tallyJSON(tally)
	qs, _ := s.store.Questions(ctx, h.ID)
	pres.Questions = qs
	if h.Kind != store.KindTalk {
		return pres, aud // a rehearsal is not shown to anyone
	}
	aud = pres
	aud.ShowID = 0
	aud.Questions = []store.Question{}
	for _, q := range qs {
		if !q.Hidden {
			aud.Questions = append(aud.Questions, q)
		}
	}
	if cur != nil && !h.Black {
		sl := forAudience(*cur, h.Step >= 1)
		aud.Slide = &sl
	}
	aud.Tally = map[string]map[string]int{}
	for i := 0; i <= h.Pos && i < len(vis); i++ {
		k := fmt.Sprint(vis[i].ID)
		if t, ok := pres.Tally[k]; ok {
			aud.Tally[k] = t
		}
	}
	return pres, aud
}

// broadcast sends the deck's state to every screen following it.
func (h *liveHub) broadcast(ctx context.Context, s *Server, deckID int64) {
	d, err := s.store.DeckByID(ctx, deckID)
	if err != nil {
		h.endDeck(deckID)
		return
	}
	pres, aud := s.state(ctx, d)
	pb, _ := json.Marshal(pres)
	ab, _ := json.Marshal(aud)
	h.send(deckID, pb, ab)
	if pres.Live && pres.Viewers > 0 {
		if sh, err := s.store.GetShow(ctx, d.UserID, pres.ShowID); err == nil && pres.Viewers > sh.Viewers {
			sh.Viewers = pres.Viewers
			_ = s.store.SaveShow(ctx, sh)
		}
	}
}

// ---- moving the show ----

type controlIn struct {
	Action string `json:"action"` // start | next | prev | go | black | end
	Kind   string `json:"kind"`   // start: talk | rehearsal
	Pos    int    `json:"pos"`    // go
	Black  *bool  `json:"black"`  // black
	Note   string `json:"note"`   // end: how it went
}

// control moves a deck's show; it returns the show (nil once it ended).
func (s *Server) control(ctx context.Context, d *store.Deck, in controlIn) (*store.Show, error) {
	now := time.Now()
	h, err := s.store.LiveShow(ctx, d.ID)
	if in.Action == "start" {
		kind := firstNonEmpty(in.Kind, store.KindTalk)
		if !oneOf(kind, []string{store.KindTalk, store.KindRehearsal}) {
			return nil, &userErr{"a show is a talk or a rehearsal"}
		}
		if err == nil {
			if h.Kind != kind {
				h.Kind = kind
				_ = s.store.SaveShow(ctx, h)
			}
			s.live.broadcast(ctx, s, d.ID)
			return h, nil
		}
		h = &store.Show{UserID: d.UserID, DeckID: d.ID, Kind: kind}
		if in.Pos > 0 {
			h.Pos = in.Pos
		}
		if err := s.store.StartShow(ctx, h); err != nil {
			return nil, err
		}
		s.live.mu.Lock()
		s.live.since[h.ID] = now
		s.live.mu.Unlock()
		s.live.broadcast(ctx, s, d.ID)
		return h, nil
	}
	if err != nil {
		return nil, &userErr{"the deck is not being presented"}
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	vis := visible(slides)
	n := len(vis)
	moveTo := func(pos, step int) {
		pos = clampInt(pos, 0, maxInt(n-1, 0))
		if pos != h.Pos {
			s.tick(h, vis, now)
			h.Pos = pos
		}
		h.Step = step
	}
	switch in.Action {
	case "next":
		if h.Pos < n && h.Step < steps(&vis[h.Pos]) {
			h.Step++
		} else if h.Pos < n-1 {
			moveTo(h.Pos+1, 0)
		}
		h.Black = false
	case "prev":
		if h.Step > 0 {
			h.Step--
		} else if h.Pos > 0 {
			moveTo(h.Pos-1, steps(&vis[h.Pos-1]))
		}
		h.Black = false
	case "go":
		moveTo(in.Pos, 0)
		h.Black = false
	case "black":
		if in.Black != nil {
			h.Black = *in.Black
		} else {
			h.Black = !h.Black
		}
	case "end":
		s.tick(h, vis, now)
		at := store.Now()
		h.EndedAt = &at
		if t, err := time.Parse(time.RFC3339Nano, h.StartedAt); err == nil {
			h.TotalMs = now.Sub(t).Milliseconds()
		}
		if v := s.live.viewers(d.ID); v > h.Viewers {
			h.Viewers = v
		}
		h.Note = cleanText(in.Note, 4000)
		if err := s.store.SaveShow(ctx, h); err != nil {
			return nil, err
		}
		s.live.mu.Lock()
		delete(s.live.since, h.ID)
		s.live.mu.Unlock()
		topic := "slides.presented"
		if h.Kind == store.KindRehearsal {
			topic = "slides.rehearsed"
		}
		s.publish(ctx, d.UserID, topic, fmt.Sprintf("show:%d", h.ID), map[string]any{"deck_id": d.ID, "title": d.Title, "show_id": h.ID, "minutes": h.TotalMs / 60000,
			"target_minutes": d.TargetMinutes, "slides": n, "viewers": h.Viewers, "url": s.deckURL(d)})
		if h.Kind == store.KindTalk {
			go s.afterTalk(context.WithoutCancel(ctx), d, h.ID)
		}
		pres, aud := s.state(ctx, d)
		pres.Type, aud.Type = "ended", "ended"
		pb, _ := json.Marshal(pres)
		ab, _ := json.Marshal(aud)
		s.live.send(d.ID, pb, ab)
		return nil, nil
	default:
		return nil, &userErr{"unknown action"}
	}
	if err := s.store.SaveShow(ctx, h); err != nil {
		return nil, err
	}
	s.live.broadcast(ctx, s, d.ID)
	return h, nil
}

// tick adds the time spent on the slide on the screen to its timing.
func (s *Server) tick(h *store.Show, vis []store.Slide, now time.Time) {
	s.live.mu.Lock()
	since, ok := s.live.since[h.ID]
	s.live.since[h.ID] = now
	s.live.mu.Unlock()
	if !ok || h.Black || h.Pos < 0 || h.Pos >= len(vis) {
		return
	}
	if h.Timings == nil {
		h.Timings = map[string]int64{}
	}
	h.Timings[fmt.Sprint(vis[h.Pos].ID)] += now.Sub(since).Milliseconds()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// handleControl moves the caller's own deck's show.
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	var in controlIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	h, err := s.control(ctx, d, in)
	if err != nil {
		s.userFail(w, err, "present")
		return
	}
	out := map[string]any{"ok": true, "show": h}
	if h != nil {
		out["remote_url"] = s.link("/r/" + h.RemoteKey)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleLiveState(w http.ResponseWriter, r *http.Request) {
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	pres, _ := s.state(r.Context(), d)
	writeJSON(w, http.StatusOK, pres)
}

func (s *Server) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	s.stream(w, r, d, false)
}

// stream keeps an event stream open for one screen of a deck.
func (s *Server) stream(w http.ResponseWriter, r *http.Request, d *store.Deck, audience bool) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming is not supported here")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	sb := s.live.sub(d.ID, audience)
	defer func() {
		s.live.unsub(d.ID, sb)
		if audience {
			go s.live.broadcast(context.Background(), s, d.ID)
		}
	}()
	pres, aud := s.state(r.Context(), d)
	first := pres
	if audience {
		first = aud
	}
	b, _ := json.Marshal(first)
	fmt.Fprintf(w, "data: %s\n\n", b)
	fl.Flush()
	if audience {
		go s.live.broadcast(context.WithoutCancel(r.Context()), s, d.ID) // the viewer count changed
	}
	beat := time.NewTicker(25 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-sb.ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-beat.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// ---- the phone link of a show: the presenter screen without signing in ----

func (s *Server) remoteOf(w http.ResponseWriter, r *http.Request) (*store.Show, *store.Deck, bool) {
	ctx := r.Context()
	h, err := s.store.ShowByRemote(ctx, chiParam(r, "key"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "this phone link has ended (the show is over); open a new one from the screen")
		return nil, nil, false
	}
	d, err := s.store.DeckByID(ctx, h.DeckID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return h, d, true
}

func (s *Server) handleRemote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, d, ok := s.remoteOf(w, r)
	if !ok {
		return
	}
	out := s.deckPayload(ctx, d)
	pres, _ := s.state(ctx, d)
	out["state"] = pres
	st := s.settingsOf(ctx, d.UserID)
	out["prefs"] = map[string]any{"warn_minutes": st.WarnMinutes, "notes_size": st.NotesSize, "keep_awake": st.KeepAwake}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRemoteStream(w http.ResponseWriter, r *http.Request) {
	_, d, ok := s.remoteOf(w, r)
	if !ok {
		return
	}
	s.stream(w, r, d, false)
}

func (s *Server) handleRemoteControl(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, d, ok := s.remoteOf(w, r)
	if !ok {
		return
	}
	var in controlIn
	if err := decode(r, &in); err != nil || in.Action == "start" {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	h, err := s.control(ctx, d, in)
	if err != nil {
		s.userFail(w, err, "present")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ended": h == nil})
}

func (s *Server) handleRemoteQuestion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h, d, ok := s.remoteOf(w, r)
	if !ok {
		return
	}
	q, err := s.store.GetQuestion(ctx, d.UserID, idParam(r, "qid"))
	if err != nil || q.ShowID != h.ID {
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
	_ = s.store.MarkQuestion(ctx, q)
	s.live.broadcast(ctx, s, d.ID)
	writeJSON(w, http.StatusOK, map[string]any{"question": q})
}

func (s *Server) handleRemoteUpload(w http.ResponseWriter, r *http.Request) {
	_, d, ok := s.remoteOf(w, r)
	if !ok {
		return
	}
	s.deckUpload(w, r, d, -1)
}

// ---- the audience link ----

func (s *Server) audienceDeck(w http.ResponseWriter, r *http.Request) (*store.Deck, bool) {
	d, err := s.store.DeckByKey(r.Context(), "live", strings.ToLower(chiParam(r, "key")))
	if err != nil {
		writeErr(w, http.StatusNotFound, "there is no talk with that code")
		return nil, false
	}
	return d, true
}

// handleAudience is what someone following a talk sees: the slides shown so far.
func (s *Server) handleAudience(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	_, aud := s.state(ctx, d)
	seen := []store.Slide{}
	if aud.Live {
		slides, _ := s.store.Slides(ctx, d.ID)
		vis := visible(slides)
		for i := 0; i <= aud.Pos && i < len(vis); i++ {
			seen = append(seen, forAudience(vis[i], i < aud.Pos || aud.Step >= 1))
		}
	}
	out := map[string]any{"deck": publicDeck(d), "state": aud, "seen": seen, "theme": s.deckTheme(ctx, d)}
	if d.Shared {
		out["share_key"] = d.ShareKey
	}
	if h, err := s.store.LiveShow(ctx, d.ID); err == nil && h.Kind == store.KindTalk {
		if v := r.URL.Query().Get("voter"); reVoter.MatchString(v) {
			out["mine"] = s.store.MyVotes(ctx, h.ID, v)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAudienceStream(w http.ResponseWriter, r *http.Request) {
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	s.stream(w, r, d, true)
}

var reVoter = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// talkShow is the running talk of an audience link, for votes, questions and reactions.
func (s *Server) talkShow(w http.ResponseWriter, r *http.Request, d *store.Deck) (*store.Show, bool) {
	h, err := s.store.LiveShow(r.Context(), d.ID)
	if err != nil || h.Kind != store.KindTalk {
		writeErr(w, http.StatusBadRequest, "the talk is not on now")
		return nil, false
	}
	return h, true
}

func (s *Server) handleAudienceVote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	var in struct {
		SlideID int64  `json:"slide_id"`
		Choice  int    `json:"choice"`
		Voter   string `json:"voter"`
	}
	if err := decode(r, &in); err != nil || !reVoter.MatchString(in.Voter) {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !s.allowGuest(r, "vote", 60, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "too many answers from here; wait a minute")
		return
	}
	h, ok := s.talkShow(w, r, d)
	if !ok {
		return
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	vis := visible(slides)
	if h.Pos >= len(vis) || vis[h.Pos].ID != in.SlideID || vis[h.Pos].Layout != "poll" {
		writeErr(w, http.StatusBadRequest, "that poll is not on the screen any more")
		return
	}
	opts := dataOf(&vis[h.Pos]).Options
	if in.Choice < 0 || in.Choice >= len(opts) {
		writeErr(w, http.StatusBadRequest, "pick one of the answers")
		return
	}
	if err := s.store.Vote(ctx, d.UserID, h.ID, in.SlideID, in.Choice, in.Voter); err != nil {
		s.fail(w, err, "vote")
		return
	}
	s.live.broadcast(ctx, s, d.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAudienceQuestion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	var in struct {
		Body  string `json:"body"`
		Name  string `json:"name"`
		Voter string `json:"voter"`
	}
	if err := decode(r, &in); err != nil || cleanText(in.Body, 500) == "" {
		writeErr(w, http.StatusBadRequest, "write your question")
		return
	}
	if !d.Questions {
		writeErr(w, http.StatusBadRequest, "this talk does not take questions here")
		return
	}
	if !s.allowGuest(r, "question", 10, 10*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "that is a lot of questions; wait a few minutes")
		return
	}
	h, ok := s.talkShow(w, r, d)
	if !ok {
		return
	}
	q := &store.Question{ShowID: h.ID, UserID: d.UserID, Name: cleanText(in.Name, 60), Body: cleanText(in.Body, 500)}
	slides, _ := s.store.Slides(ctx, d.ID)
	if vis := visible(slides); h.Pos < len(vis) {
		q.SlideID = &vis[h.Pos].ID
	}
	if err := s.store.AddQuestion(ctx, q); err != nil {
		s.fail(w, err, "question")
		return
	}
	s.live.broadcast(ctx, s, d.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "question": q})
}

func (s *Server) handleAudienceLike(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	var in struct {
		Voter string `json:"voter"`
	}
	if err := decode(r, &in); err != nil || !reVoter.MatchString(in.Voter) {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	h, ok := s.talkShow(w, r, d)
	if !ok {
		return
	}
	qid := idParam(r, "qid")
	k := fmt.Sprintf("%d:%d:%s", h.ID, qid, in.Voter)
	s.live.mu.Lock()
	dup := s.live.likes[k]
	s.live.likes[k] = true
	s.live.mu.Unlock()
	if !dup {
		if err := s.store.LikeQuestion(ctx, h.ID, qid); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		s.live.broadcast(ctx, s, d.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

var reactions = []string{"👏", "❤️", "😂", "🤯", "👍", "🎉"}

func (s *Server) handleAudienceReact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	var in struct {
		Emoji string `json:"emoji"`
		Voter string `json:"voter"`
	}
	if err := decode(r, &in); err != nil || !reVoter.MatchString(in.Voter) || !oneOf(in.Emoji, reactions) {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	h, ok := s.talkShow(w, r, d)
	if !ok {
		return
	}
	k := fmt.Sprintf("%d:%s", h.ID, in.Voter)
	s.live.mu.Lock()
	last := s.live.reacts[k]
	fast := time.Since(last) < 2*time.Second
	if !fast {
		s.live.reacts[k] = time.Now()
	}
	s.live.mu.Unlock()
	if fast {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	h.Reactions++
	_ = s.store.SaveShow(ctx, h)
	b, _ := json.Marshal(map[string]any{"type": "reaction", "emoji": in.Emoji})
	s.live.send(d.ID, b, b)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAudienceFeedback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	var in struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
		Name    string `json:"name"`
		Voter   string `json:"voter"`
	}
	if err := decode(r, &in); err != nil || !reVoter.MatchString(in.Voter) || in.Rating < 1 || in.Rating > 5 {
		writeErr(w, http.StatusBadRequest, "pick from one to five stars")
		return
	}
	if !s.allowGuest(r, "feedback", 10, 10*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "wait a few minutes")
		return
	}
	h, err := s.store.LiveShow(ctx, d.ID)
	if err != nil || h.Kind != store.KindTalk {
		h, err = s.store.LatestShow(ctx, d.ID, store.KindTalk)
	}
	if err != nil || h.Kind != store.KindTalk {
		writeErr(w, http.StatusBadRequest, "there was no talk to say something about")
		return
	}
	if h.EndedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *h.EndedAt); err == nil && time.Since(t) > 48*time.Hour {
			writeErr(w, http.StatusBadRequest, "it is too long after the talk")
			return
		}
	}
	f := &store.Feedback{ShowID: h.ID, UserID: d.UserID, Rating: in.Rating, Comment: cleanText(in.Comment, 2000), Name: cleanText(in.Name, 60)}
	if err := s.store.AddFeedback(ctx, f, in.Voter); err != nil {
		s.fail(w, err, "feedback")
		return
	}
	st := s.settingsOf(ctx, d.UserID)
	if st.NotifyFeedback && f.Comment != "" {
		s.push(ctx, d.UserID, st, "About “"+d.Title+"”", fmt.Sprintf("%s: %s %s", firstNonEmpty(f.Name, "Someone"), strings.Repeat("★", f.Rating), clip(f.Comment, 200)), fmt.Sprintf("/d/%d/shows/%d", d.ID, h.ID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAudienceUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.audienceDeck(w, r)
	if !ok {
		return
	}
	_, aud := s.state(ctx, d)
	if !aud.Live || aud.Kind != store.KindTalk {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.deckUpload(w, r, d, aud.Pos)
}

// ---- guests: a simple limit per address ----

func (s *Server) allowGuest(r *http.Request, what string, n int, per time.Duration) bool {
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	k := what + "|" + ip
	now := time.Now()
	s.guestMu.Lock()
	defer s.guestMu.Unlock()
	keep := []time.Time{}
	for _, t := range s.guestHits[k] {
		if now.Sub(t) < per {
			keep = append(keep, t)
		}
	}
	if len(keep) >= n {
		s.guestHits[k] = keep
		return false
	}
	s.guestHits[k] = append(keep, now)
	return true
}

// afterTalk tells the speaker about the questions they did not get to.
func (s *Server) afterTalk(ctx context.Context, d *store.Deck, showID int64) {
	qs, _ := s.store.Questions(ctx, showID)
	open := []string{}
	for _, q := range qs {
		if !q.Answered && !q.Hidden {
			open = append(open, "• "+clip(q.Body, 120))
		}
	}
	if len(open) == 0 {
		return
	}
	st := s.settingsOf(ctx, d.UserID)
	if !st.NotifyQuestion {
		return
	}
	title := fmt.Sprintf("%d %s you did not get to", len(open), plural(len(open), "question", "questions"))
	if len(open) > 5 {
		open = append(open[:5], "…")
	}
	path := fmt.Sprintf("/d/%d/shows/%d", d.ID, showID)
	s.push(ctx, d.UserID, st, title, "“"+d.Title+"”\n"+strings.Join(open, "\n"), path)
	s.alert(ctx, d.UserID, title, "“"+d.Title+"”\n"+strings.Join(open, "\n"), path, "low")
}
