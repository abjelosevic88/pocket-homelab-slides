package server

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Slides (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They read what /search, /day and /summary read, as the request's
// person, and change nothing. There is no quick capture: a deck needs slides.

// deckPath opens a deck in the web app; showPath one talk or rehearsal.
func deckPath(id int64) string { return "/d/" + itoa(id) }

func showPath(deckID, showID int64) string { return fmt.Sprintf("/d/%d/shows/%d", deckID, showID) }

// dots joins the parts of a line that are there: "12 slides · Talk at DevCon".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// counted: "1 talk", "3 talks".
func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// slidesWords: "12 slides", "1 slide", "No slides yet".
func slidesWords(n int) string {
	if n == 0 {
		return "No slides yet"
	}
	return counted(n, "slide", "slides")
}

// minutesWords is a time in words: "under a minute", "18 min", "1 h 20 min".
func minutesWords(m float64) string {
	n := int(math.Round(m))
	switch {
	case m <= 0:
		return ""
	case n < 1:
		return "under a minute"
	case n < 60:
		return fmt.Sprintf("%d min", n)
	case n%60 == 0:
		return fmt.Sprintf("%d h", n/60)
	}
	return fmt.Sprintf("%d h %d min", n/60, n%60)
}

// showMinutes is how long a show took, to a tenth of a minute.
func showMinutes(h store.Show) float64 { return math.Round(float64(h.TotalMs)/6000) / 10 }

// talkTime splits a deck's talk (local "YYYY-MM-DD" or "YYYY-MM-DDTHH:MM")
// into its day and its clock time ("" when the talk has none).
func talkTime(d store.Deck) (time.Time, string, bool) {
	day, clock, _ := strings.Cut(d.TalkAt, "T")
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	return t, clock, err == nil
}

// onDay: "Fri 5 Dec", with the year when it is not this one.
func onDay(t, now time.Time) string {
	if t.Year() != now.Year() {
		return t.Format("Mon 2 Jan 2006")
	}
	return t.Format("Mon 2 Jan")
}

// talkWords is a deck's talk in plain words: "Talk at DevCon on Fri 5 Dec,
// 14:00", "“Why tests” at DevCon" when the talk has its own title; "" when
// no talk is set.
func talkWords(d store.Deck, now time.Time) string {
	what := "Talk"
	if d.TalkTitle != "" && !strings.EqualFold(strings.TrimSpace(d.TalkTitle), strings.TrimSpace(d.Title)) {
		what = "“" + d.TalkTitle + "”"
	}
	line := what
	if d.TalkPlace != "" {
		line += " at " + d.TalkPlace
	}
	if t, clock, ok := talkTime(d); ok {
		line += " on " + onDay(t, now)
		if clock != "" {
			line += ", " + clock
		}
	}
	if line == what && what == "Talk" {
		return ""
	}
	return line
}

// editedWords: "edited today", "edited yesterday", "edited 3 Nov".
func editedWords(at string, now time.Time) string {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return ""
	}
	t = t.In(time.Local)
	y1, m1, d1 := t.Date()
	day := time.Date(y1, m1, d1, 0, 0, 0, 0, time.Local)
	y2, m2, d2 := now.Date()
	today := time.Date(y2, m2, d2, 0, 0, 0, 0, time.Local)
	switch {
	case day.Equal(today):
		return "edited today"
	case day.Equal(today.AddDate(0, 0, -1)):
		return "edited yesterday"
	case t.Year() != now.Year():
		return "edited " + t.Format("2 Jan 2006")
	}
	return "edited " + t.Format("2 Jan")
}

// hasWord reports whether a search has a letter or a digit in it.
func hasWord(q string) bool {
	return strings.IndexFunc(q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// ---------- search ----------

// pocketSearch answers Pocket Search with the decks /search finds (by their
// title, description, tags, slides and speaker notes, archived ones too) and
// the decks whose talk has the words in its title or place. A deck whose
// title has the words comes first, then its talk, then the rest.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if !hasWord(q) {
		return nil, nil
	}
	ctx := r.Context()
	uid := actorID(ctx)
	decks, hits, err := s.store.SearchDecks(ctx, uid, q, limit)
	if err != nil {
		return nil, err
	}
	talks, err := s.store.SearchTalks(ctx, uid, q, limit)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, d := range decks {
		seen[d.ID] = true
	}
	for _, d := range talks {
		if !seen[d.ID] {
			decks = append(decks, d)
		}
	}
	now := time.Now()
	lq := strings.ToLower(q)
	has := func(v ...string) bool { return strings.Contains(strings.ToLower(strings.Join(v, " ")), lq) }
	out := make([]core.SearchItem, 0, len(decks))
	for _, d := range decks {
		it := core.SearchItem{ID: itoa(d.ID), Title: firstNonEmpty(d.Title, d.TalkTitle), Kind: "deck", Date: d.UpdatedAt, URL: deckPath(d.ID), Score: 1,
			Snippet: dots(slidesWords(d.Slides), talkWords(d, now))}
		switch {
		case has(d.Title):
			it.Score = 3
		case has(d.TalkTitle, d.TalkPlace):
			it.Score = 2
		default:
			// say where the words were found when the line above does not show it
			if sl, ok := hits[d.ID]; ok {
				it.Snippet = dots(it.Snippet, clip(firstNonEmpty(matchLine(sl.Body, q), matchLine(sl.Body2, q), matchLine(sl.Notes, q), matchLine(sl.Subtitle, q), sl.Title), 160))
			}
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- day ----------

// pocketDay answers Pocket Day with what /day reads: the talks given and
// the rehearsals done that day (with how long they took), a talk planned for
// it that was not given here, and the decks made that day. Times are in the
// caller's zone when it says one, else the box's.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	loc := time.Local
	if tz != nil {
		loc = tz
	}
	start, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return nil, nil
	}
	from, to := start.UTC().Format(time.RFC3339Nano), start.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	shows, err := s.store.ShowsBetween(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	planned, err := s.store.UpcomingTalks(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	made, err := s.store.DecksCreatedBetween(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	decks := map[int64]*store.Deck{}
	deckOf := func(id int64) *store.Deck {
		if d, ok := decks[id]; ok {
			return d
		}
		d, _ := s.store.GetDeck(ctx, uid, id)
		decks[id] = d
		return d
	}
	clock := func(at string) string {
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return ""
		}
		return t.In(loc).Format("15:04")
	}

	day := &core.Day{Date: date}
	given := map[int64]bool{}
	talks, rehearsals := 0, 0
	var talkMin, rehearseMin float64
	for _, h := range shows {
		d := deckOf(h.DeckID)
		mins := showMinutes(h)
		it := core.DayItem{ID: "show:" + itoa(h.ID), Time: clock(h.StartedAt), Title: h.DeckTitle, Minutes: mins, URL: showPath(h.DeckID, h.ID)}
		took := minutesWords(mins)
		if h.EndedAt == nil {
			took = "going on now"
		}
		if h.Kind == store.KindRehearsal {
			rehearsals++
			rehearseMin += mins
			it.Kind = "rehearsal"
			if d != nil && d.TargetMinutes > 0 && h.EndedAt != nil {
				took = fmt.Sprintf("%s of %d planned", firstNonEmpty(took, "0 min"), d.TargetMinutes)
			}
			it.Sub, it.Note = took, h.Note
		} else {
			talks++
			talkMin += mins
			given[h.DeckID] = true
			it.Kind, it.Done = "talk", h.EndedAt != nil
			place := ""
			if d != nil && strings.HasPrefix(d.TalkAt, date) {
				it.Title, place = firstNonEmpty(d.TalkTitle, h.DeckTitle), d.TalkPlace
			}
			var heard []string
			if h.Viewers > 0 {
				heard = append(heard, counted(h.Viewers, "person followed", "people followed"))
			}
			if h.Questions > 0 {
				heard = append(heard, counted(h.Questions, "question", "questions"))
			}
			if h.Rating > 0 {
				heard = append(heard, "rated "+strconv.FormatFloat(math.Round(h.Rating*10)/10, 'f', -1, 64)+" of 5")
			}
			it.Sub, it.Note = dots(place, took, strings.Join(heard, ", ")), h.Note
		}
		day.Items = append(day.Items, it)
	}
	plannedN := 0
	for _, d := range planned {
		if !strings.HasPrefix(d.TalkAt, date) || given[d.ID] {
			continue
		}
		plannedN++
		_, at, _ := talkTime(d)
		rehearsed := "not rehearsed yet"
		if d.Rehearsals > 0 {
			rehearsed = "rehearsed " + times(d.Rehearsals)
		}
		day.Items = append(day.Items, core.DayItem{ID: "talk:" + itoa(d.ID), Time: at, Title: firstNonEmpty(d.TalkTitle, d.Title), Kind: "talk",
			Sub: dots(d.TalkPlace, slidesWords(d.Slides), rehearsed), URL: deckPath(d.ID)})
	}
	for _, d := range made {
		day.Items = append(day.Items, core.DayItem{ID: "deck:" + itoa(d.ID), Time: clock(d.CreatedAt), Title: d.Title, Kind: "new deck",
			Sub: slidesWords(d.Slides), URL: deckPath(d.ID)})
	}
	if len(day.Items) == 0 {
		return nil, nil
	}

	// "1 talk given, 2 rehearsals (35 min), 1 new deck"
	var parts []string
	if talks > 0 {
		parts = append(parts, counted(talks, "talk", "talks")+" given")
	}
	if plannedN > 0 {
		parts = append(parts, counted(plannedN, "talk", "talks")+" planned")
	}
	if rehearsals > 0 {
		p := counted(rehearsals, "rehearsal", "rehearsals")
		if m := minutesWords(rehearseMin); m != "" {
			p += " (" + m + ")"
		}
		parts = append(parts, p)
	}
	if len(made) > 0 {
		parts = append(parts, counted(len(made), "new deck", "new decks"))
	}
	day.Summary = strings.Join(parts, ", ")
	if m := minutesWords(talkMin); m != "" {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Presenting", Value: m})
	}
	if m := minutesWords(rehearseMin); m != "" {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Rehearsing", Value: m})
	}
	return day, nil
}

// times: "once", "twice", "3 times".
func times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return strconv.Itoa(n) + " times"
}

// ---------- widget ----------

// widgetDecks is how many rows the widget shows at most.
const widgetDecks = 4

// whenWords is how far off a talk is: "today", "tomorrow at 14:00", "in 5 days".
func whenWords(t time.Time, clock string, now time.Time) string {
	y, m, d := now.Date()
	days := int(math.Round(t.Sub(time.Date(y, m, d, 0, 0, 0, 0, time.Local)).Hours() / 24))
	at := ""
	if clock != "" {
		at = " at " + clock
	}
	switch {
	case days <= 0:
		return "today" + at
	case days == 1:
		return "tomorrow" + at
	}
	return fmt.Sprintf("in %d days", days)
}

// pocketWidget answers Pocket Hub with the next talk, as the first page and
// /summary show it: how far off it is, how often it was rehearsed since the
// deck was last given and how the last run compares with the planned time;
// then the decks changed last. Someone without decks gets nothing to show.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	now := time.Now()
	upcoming, err := s.store.UpcomingTalks(ctx, uid, now.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	recent, err := s.store.RecentDecks(ctx, uid, widgetDecks+1)
	if err != nil {
		return nil, err
	}
	if len(upcoming) == 0 && len(recent) == 0 {
		return nil, nil
	}
	w := &core.WidgetData{URL: "/", Line: "No talks coming up"}
	next := int64(0)
	if len(upcoming) > 0 {
		d := upcoming[0]
		next = d.ID
		t, clock, _ := talkTime(d)
		when := whenWords(t, clock, now)
		w.Line, w.URL = "Next talk "+when, "/talks"
		if d.TalkPlace != "" {
			w.Line = "Talk at " + d.TalkPlace + " " + when
		}
		// the rehearsals since the deck was last given, the newest first
		shows, err := s.store.Shows(ctx, uid, d.ID, 50)
		if err != nil {
			return nil, err
		}
		runs, last := 0, 0.0
		for _, h := range shows {
			if h.Kind == store.KindTalk {
				break
			}
			if h.EndedAt == nil {
				continue
			}
			if runs == 0 {
				last = showMinutes(h)
			}
			runs++
		}
		rehearsed := "Not rehearsed yet"
		if runs > 0 {
			rehearsed = "Rehearsed " + times(runs)
			if m := minutesWords(last); m != "" {
				rehearsed += ", last run " + m
			}
		}
		if d.TargetMinutes > 0 {
			rehearsed += fmt.Sprintf(" (planned %d min)", d.TargetMinutes)
		}
		w.Metrics = append(w.Metrics, core.Metric{Label: "Rehearsals", Value: strconv.Itoa(runs)})
		if m := minutesWords(last); m != "" {
			w.Metrics = append(w.Metrics, core.Metric{Label: "Last run", Value: m})
		}
		if d.TargetMinutes > 0 {
			w.Metrics = append(w.Metrics, core.Metric{Label: "Planned", Value: minutesWords(float64(d.TargetMinutes))})
		}
		w.Items = append(w.Items, core.WidgetItem{ID: itoa(d.ID), Title: firstNonEmpty(d.TalkTitle, d.Title), Detail: rehearsed, URL: deckPath(d.ID)})
	}
	for _, d := range recent {
		if d.ID == next || len(w.Items) == widgetDecks {
			continue
		}
		w.Items = append(w.Items, core.WidgetItem{ID: itoa(d.ID), Title: d.Title, Detail: dots(slidesWords(d.Slides), editedWords(d.UpdatedAt, now)), URL: deckPath(d.ID)})
	}
	return w, nil
}
