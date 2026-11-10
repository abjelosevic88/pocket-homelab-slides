package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// handleHome is the start page: the next talk, decks and recent shows.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	decks, err := s.store.Decks(ctx, uid, false)
	if err != nil {
		s.fail(w, err, "home")
		return
	}
	upcoming, _ := s.store.UpcomingTalks(ctx, uid, time.Now().Format("2006-01-02"))
	shows, _ := s.store.Shows(ctx, uid, 0, 8)
	for i := range shows {
		shows[i].RemoteKey = ""
	}
	live := []store.Deck{}
	for _, d := range decks {
		if d.Live {
			live = append(live, d)
		}
	}
	archived, _ := s.store.Decks(ctx, uid, true)
	s.withCovers(ctx, decks)
	writeJSON(w, http.StatusOK, map[string]any{"decks": decks, "upcoming": upcoming, "shows": shows, "live": live, "archived": len(archived)})
}

// handleSummary is a short overview for Pocket Day, AI and dashboards.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	decks, err := s.store.Decks(ctx, uid, false)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	slides, talks, rehearsals := 0, 0, 0
	for _, d := range decks {
		slides += d.Slides
		talks += d.Talks
		rehearsals += d.Rehearsals
	}
	upcoming, _ := s.store.UpcomingTalks(ctx, uid, time.Now().Format("2006-01-02"))
	next := map[string]any(nil)
	if len(upcoming) > 0 {
		d := upcoming[0]
		next = map[string]any{"deck_id": d.ID, "title": firstNonEmpty(d.TalkTitle, d.Title), "at": d.TalkAt, "place": d.TalkPlace, "rehearsals": d.Rehearsals, "slides": d.Slides, "url": s.deckURL(&d)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"decks": len(decks), "slides": slides, "talks": talks, "rehearsals": rehearsals, "upcoming": len(upcoming), "next_talk": next, "url": s.link("/")})
}

// handleDay is what happened on a day (talks given, rehearsals) and what is on it (a talk).
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chiParam(r, "date")
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	shows, _ := s.store.ShowsBetween(ctx, uid, day.UTC().Format(time.RFC3339Nano), day.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano))
	items := []map[string]any{}
	for _, h := range shows {
		what := "Gave the talk"
		if h.Kind == store.KindRehearsal {
			what = "Rehearsed"
		}
		t, _ := time.Parse(time.RFC3339Nano, h.StartedAt)
		items = append(items, map[string]any{"kind": h.Kind, "title": what + " “" + h.DeckTitle + "”", "time": t.Local().Format("15:04"), "minutes": h.TotalMs / 60000,
			"viewers": h.Viewers, "questions": h.Questions, "url": s.link(fmt.Sprintf("/d/%d/shows/%d", h.DeckID, h.ID))})
	}
	talks, _ := s.store.UpcomingTalks(ctx, uid, date)
	for _, d := range talks {
		if !strings.HasPrefix(d.TalkAt, date) {
			continue
		}
		tm := ""
		if len(d.TalkAt) > 11 {
			tm = d.TalkAt[11:]
		}
		items = append(items, map[string]any{"kind": "planned", "title": "Talk: " + firstNonEmpty(d.TalkTitle, d.Title), "time": tm, "place": d.TalkPlace, "url": s.deckURL(&d)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "items": items})
}

// handleSearch finds decks by their words (for Pocket Search and AI).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}})
		return
	}
	decks, hits, err := s.store.SearchDecks(ctx, actorID(ctx), q, clampInt(firstPos(queryInt(r, "limit"), 20), 1, 50))
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	out := []map[string]any{}
	for _, d := range decks {
		m := map[string]any{"id": d.ID, "title": d.Title, "about": d.About, "slides": d.Slides, "updated_at": d.UpdatedAt, "url": s.deckURL(&d), "archived": d.Archived}
		if sl, ok := hits[d.ID]; ok {
			m["slide_id"], m["slide_title"] = sl.ID, sl.Title
			m["snippet"] = clip(firstNonEmpty(matchLine(sl.Body, q), matchLine(sl.Body2, q), matchLine(sl.Notes, q), sl.Title), 200)
			m["url"] = s.link(fmt.Sprintf("/d/%d?slide=%d", d.ID, sl.ID))
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

func matchLine(text, q string) string {
	lq := strings.ToLower(q)
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(l), lq) {
			return strings.TrimSpace(strings.TrimLeft(l, "-*0123456789. "))
		}
	}
	return ""
}
