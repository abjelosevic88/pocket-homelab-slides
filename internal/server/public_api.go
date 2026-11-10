package server

import (
	"context"
	"net/http"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// publicDeck is a deck as people with one of its links may see it.
func publicDeck(d *store.Deck) map[string]any {
	return map[string]any{"title": d.Title, "about": d.About, "theme": d.Theme, "aspect": d.Aspect, "footer": d.Footer, "numbers": d.Numbers,
		"transition": d.Transition, "talk_title": d.TalkTitle, "talk_at": d.TalkAt, "talk_place": d.TalkPlace, "questions": d.Questions,
		"live_key": d.LiveKey, "share_key": shareKeyIf(d)}
}

func shareKeyIf(d *store.Deck) string {
	if d.Shared {
		return d.ShareKey
	}
	return ""
}

// deckTheme is the deck's own theme, if it has one.
func (s *Server) deckTheme(ctx context.Context, d *store.Deck) *store.Theme {
	if d.ThemeID == nil {
		return nil
	}
	t, err := s.store.ThemeByID(ctx, *d.ThemeID)
	if err != nil || t.UserID != d.UserID {
		return nil
	}
	return t
}

// handleShared is the read-only deck behind a share link (no speaker notes).
func (s *Server) handleShared(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, err := s.store.DeckByKey(ctx, "share", chiParam(r, "key"))
	if err != nil || !d.Shared {
		writeErr(w, http.StatusNotFound, "this link does not work (any more)")
		return
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	out := []store.Slide{}
	for _, sl := range visible(slides) {
		out = append(out, forAudience(sl, true))
	}
	owner := ""
	if u, err := s.store.GetUser(ctx, d.UserID); err == nil {
		owner = s.actorName(u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deck": publicDeck(d), "slides": out, "theme": s.deckTheme(ctx, d), "owner": owner})
}

func (s *Server) handleSharedUpload(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.DeckByKey(r.Context(), "share", chiParam(r, "key"))
	if err != nil || !d.Shared {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.deckUpload(w, r, d, -2)
}

// deckUpload serves a picture of a deck to someone with one of its links:
// upTo -1 = any picture of the deck (the presenter's phone), -2 = any picture
// on a shown slide (the share link), n = pictures on the shown slides up to
// the nth (the audience). A theme's logo always.
func (s *Server) deckUpload(w http.ResponseWriter, r *http.Request, d *store.Deck, upTo int) {
	ctx := r.Context()
	id := idParam(r, "id")
	up, err := s.store.GetUpload(ctx, id)
	if err != nil || up.UserID != d.UserID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	ok := false
	if t := s.deckTheme(ctx, d); t != nil && t.LogoID != nil && *t.LogoID == id {
		ok = true
	}
	if !ok && up.DeckID == d.ID {
		if upTo == -1 {
			ok = true
		} else {
			slides, _ := s.store.Slides(ctx, d.ID)
			for i, sl := range visible(slides) {
				if upTo >= 0 && i > upTo {
					break
				}
				if sl.ImageID != nil && *sl.ImageID == id {
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.serveUpload(w, r, up)
}
