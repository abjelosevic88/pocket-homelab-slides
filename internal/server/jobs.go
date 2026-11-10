package server

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

const notifyTestBody = "This is how Pocket Slides tells you a talk is tomorrow, or what the audience thought."

func htmlEsc(v string) string { return html.EscapeString(v) }

// push sends a phone message through the person's own notification URL.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, path string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"projector"}, Link: s.link(path)}); err != nil {
		s.log.Debug("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := s.settingsOf(ctx, actorID(ctx))
	if st.NotifyURL == "" {
		writeErr(w, http.StatusBadRequest, "add a notification URL first")
		return
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Slides", Body: notifyTestBody, Priority: "info", Tags: []string{"projector"}}); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// talkReminders tells people at their reminder hour about tomorrow's talks,
// with how the rehearsals went (once per talk).
func (s *Server) talkReminders(ctx context.Context) {
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	talks, err := s.store.TalksBetween(ctx, tomorrow, now.AddDate(0, 0, 2).Format("2006-01-02"))
	if err != nil {
		return
	}
	for _, d := range talks {
		st := s.settingsOf(ctx, d.UserID)
		if now.Hour() < st.NotifyHour {
			continue
		}
		key := fmt.Sprintf("remind:%d:%s", d.ID, d.TalkAt)
		if _, _, done := s.store.CacheGet(ctx, key); done {
			continue
		}
		_ = s.store.CachePut(ctx, key, "1")
		when := "tomorrow"
		if len(d.TalkAt) > 11 {
			when += " at " + d.TalkAt[11:]
		}
		body := fmt.Sprintf("“%s” is %s", firstNonEmpty(d.TalkTitle, d.Title), when)
		if d.TalkPlace != "" {
			body += ", " + d.TalkPlace
		}
		body += ". " + s.rehearsalWords(ctx, &d)
		path := fmt.Sprintf("/d/%d", d.ID)
		if st.NotifyTalk {
			s.push(ctx, d.UserID, st, "Your talk is tomorrow", body, path)
		}
		if st.AlertsDayBefore {
			s.alert(ctx, d.UserID, "Your talk is tomorrow", body, path, "default")
		}
	}
}

// rehearsalWords says how the practising went, in a sentence.
func (s *Server) rehearsalWords(ctx context.Context, d *store.Deck) string {
	shows, _ := s.store.Shows(ctx, d.UserID, d.ID, 50)
	n := 0
	var last *store.Show
	for i := range shows {
		if shows[i].Kind == store.KindRehearsal && shows[i].EndedAt != nil {
			n++
			if last == nil {
				last = &shows[i]
			}
		}
	}
	if n == 0 {
		return "You have not rehearsed it yet."
	}
	out := fmt.Sprintf("You rehearsed it %d %s", n, plural(n, "time", "times"))
	mins := float64(last.TotalMs) / 60000
	if d.TargetMinutes > 0 {
		diff := mins - float64(d.TargetMinutes)
		switch {
		case diff > 1:
			out += fmt.Sprintf("; the last one took %.0f minutes, %.0f over", mins, diff)
		case diff < -2:
			out += fmt.Sprintf("; the last one took %.0f minutes, %.0f under", mins, -diff)
		default:
			out += fmt.Sprintf("; the last one took %.0f minutes, right on time", mins)
		}
	}
	return out + "."
}

// endForgotten ends shows left running for more than 12 hours (a closed
// laptop), keeping the time up to their last change.
func (s *Server) endForgotten(ctx context.Context) {
	shows, err := s.store.RunningShows(ctx)
	if err != nil {
		return
	}
	for _, h := range shows {
		t, err := time.Parse(time.RFC3339Nano, h.StartedAt)
		if err != nil || time.Since(t) < 12*time.Hour {
			continue
		}
		at := store.Now()
		h.EndedAt = &at
		var sum int64
		for _, v := range h.Timings {
			sum += v
		}
		h.TotalMs = sum
		_ = s.store.SaveShow(ctx, &h)
		s.live.endDeck(h.DeckID)
	}
}
