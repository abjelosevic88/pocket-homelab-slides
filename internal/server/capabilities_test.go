package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-slides/internal/config"
	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// fakeCapCore is a Pocket Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|read|write> from Pocket Hub and
// remembers the manifest Slides registers with.
type fakeCapCore struct {
	*httptest.Server
	mu       sync.Mutex
	manifest map[string]any
}

func newCapApp(t *testing.T) (*app, *fakeCapCore) {
	t.Helper()
	fc := &fakeCapCore{}
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "slides:read", "write": "slides:write"}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			fc.mu.Lock()
			fc.manifest = m
			fc.mu.Unlock()
			io.WriteString(w, `{}`)
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(in.Token, "_")
			if len(parts) != 3 || parts[0] != "pcx" || scopes[parts[2]] == "" {
				io.WriteString(w, `{"active":false}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "slides", "scope": scopes[parts[2]],
				"user": map[string]any{"id": 1, "sub": parts[1] + "@example.com", "email": parts[1] + "@example.com", "name": parts[1], "role": "member"},
				"app":  map[string]any{"id": 9, "slug": "hub", "name": "Pocket Hub"}})
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(fc.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL, c.CoreRegistrationKey = fc.URL, "key" }), fc
}

// items are the rows of a capability answer.
func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	if _, ok := m["items"].([]any); !ok {
		t.Fatalf("no items list in %v", m)
	}
	return rows(m["items"])
}

// item finds the row with that title.
func item(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range items(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

// setDeck changes a deck's fields as the web app does.
func (a *app) setDeck(auth string, d int64, fields map[string]any) {
	a.t.Helper()
	a.ok(auth, "PUT", fmt.Sprintf("/api/v1/decks/%d", d), fields)
}

// show keeps a talk or rehearsal that took minutes, started at a UTC time.
func (a *app) show(user string, d int64, kind string, at time.Time, minutes float64) int64 {
	a.t.Helper()
	ended := at.Add(time.Duration(minutes * float64(time.Minute))).UTC().Format(time.RFC3339Nano)
	h := &store.Show{UserID: a.ids[user], DeckID: d, Kind: kind, StartedAt: at.UTC().Format(time.RFC3339Nano), EndedAt: &ended, TotalMs: int64(minutes * 60000)}
	if err := a.st.StartShow(context.Background(), h); err != nil {
		a.t.Fatal(err)
	}
	return h.ID
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	garden := a.deck(a.alice, "Garden talk", "blank")
	numbers := a.deck(a.alice, "Quarterly numbers", "blank")
	a.expect(201, a.alice, "POST", fmt.Sprintf("/api/v1/decks/%d/slides", numbers), map[string]any{"layout": "bullets", "title": "Harvest", "body": "- Tomatoes grow fast\n- Beans too"})
	a.setDeck(a.alice, numbers, map[string]any{"talk_title": "Growing food", "talk_place": "Town hall", "talk_at": "2026-12-04T18:30"})
	year := time.Now().Year() == 2026

	got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=garden", nil))
	if len(got) != 1 || got[0]["id"] != itoa(garden) || got[0]["title"] != "Garden talk" || got[0]["kind"] != "deck" || got[0]["url"] != deckPath(garden) ||
		got[0]["snippet"] != "1 slide" || got[0]["date"] == "" {
		t.Fatalf("by title: %v", got)
	}
	when := "Fri 4 Dec"
	if !year {
		when = "Fri 4 Dec 2026"
	}
	talk := "2 slides · “Growing food” at Town hall on " + when + ", 18:30"
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=town+hall", nil)); len(got) != 1 || got[0]["title"] != "Quarterly numbers" || got[0]["snippet"] != talk {
		t.Fatalf("by the talk's place: %v", got)
	}
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=growing", nil)); len(got) != 1 || got[0]["id"] != itoa(numbers) {
		t.Fatalf("by the talk's title: %v", got)
	}
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=tomatoes", nil)); len(got) != 1 || got[0]["snippet"] != talk+" · Tomatoes grow fast" {
		t.Fatalf("by the words on a slide: %v", got)
	}
	// a deck whose title has the words comes before one whose talk has them
	other := a.deck(a.alice, "Town hall rehearsal notes", "blank")
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=town", nil)); len(got) != 2 || got[0]["id"] != itoa(other) || got[1]["id"] != itoa(numbers) {
		t.Fatalf("title first: %v", got)
	}
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=town&limit=1", nil)); len(got) != 1 {
		t.Fatalf("limit 1: %v", got)
	}
	for _, q := range []string{"", "%3F%3F", "*", "%25"} {
		if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q="+q, nil)); len(got) != 0 {
			t.Fatalf("no words %q: %v", q, got)
		}
	}
	// another person's decks never appear
	for _, q := range []string{"garden", "town", "tomatoes", "growing"} {
		if got := items(t, a.ok(a.bob, "GET", "/api/v1/pocket/search?q="+q, nil)); len(got) != 0 {
			t.Fatalf("bob finds alice's %q: %v", q, got)
		}
	}
	a.expect(401, "", "GET", "/api/v1/pocket/search?q=garden", nil)
	// the old route answers as before: the talk's place is not among its words
	if l := rows(a.ok(a.alice, "GET", "/api/v1/search?q=hall", nil)["results"]); len(l) != 1 || num(l[0], "id") != float64(other) {
		t.Fatalf("/search changed: %v", l)
	}
}

func TestPocketDay(t *testing.T) {
	a := newApp(t, nil)
	sarajevo, _ := time.LoadLocation("Europe/Sarajevo")
	at := func(clock string) time.Time {
		t, _ := time.ParseInLocation("2006-01-02 15:04", "2026-03-10 "+clock, sarajevo)
		return t
	}
	talk := a.deck(a.alice, "Testing in Go", "talk")
	a.setDeck(a.alice, talk, map[string]any{"talk_title": "Why tests", "talk_place": "DevCon", "talk_at": "2026-03-10T14:00", "target_minutes": 20})
	planned := a.deck(a.alice, "Garden club", "blank")
	a.setDeck(a.alice, planned, map[string]any{"talk_at": "2026-03-10T18:30", "talk_place": "Library"})
	a.setDeck(a.alice, a.deck(a.alice, "Next week", "blank"), map[string]any{"talk_at": "2026-03-17"})
	r1 := a.show("Alice", talk, store.KindRehearsal, at("08:00"), 22.5)
	a.show("Alice", talk, store.KindRehearsal, at("09:30"), 19)
	given := a.show("Alice", talk, store.KindTalk, at("14:02"), 21)
	a.show("Alice", talk, store.KindRehearsal, at("23:59").Add(2*time.Minute), 18) // the next day there
	made := a.deck(a.alice, "Fresh deck", "blank")
	if _, err := a.st.DB.Exec(`UPDATE decks SET created_at = ? WHERE id = ?`, at("09:15").UTC().Format(time.RFC3339Nano), made); err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.DB.Exec(`UPDATE shows SET viewers = 40 WHERE id = ?`, given); err != nil {
		t.Fatal(err)
	}

	day := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-03-10?tz=Europe/Sarajevo", nil)
	if day["date"] != "2026-03-10" || day["summary"] != "1 talk given, 1 talk planned, 2 rehearsals (42 min), 1 new deck" || len(items(t, day)) != 5 {
		t.Fatalf("day: %v", day)
	}
	ms := rows(day["metrics"])
	if len(ms) != 2 || ms[0]["label"] != "Presenting" || ms[0]["value"] != "21 min" || ms[1]["label"] != "Rehearsing" || ms[1]["value"] != "42 min" {
		t.Fatalf("metrics: %v", ms)
	}
	if it := item(t, day, "Why tests"); it["id"] != "show:"+itoa(given) || it["kind"] != "talk" || it["time"] != "14:02" || it["done"] != true || num(it, "minutes") != 21 ||
		it["sub"] != "DevCon · 21 min · 40 people followed" || it["url"] != fmt.Sprintf("/d/%d/shows/%d", talk, given) {
		t.Fatalf("the talk given: %v", it)
	}
	var runs []map[string]any
	for _, it := range items(t, day) {
		if it["kind"] == "rehearsal" {
			runs = append(runs, it)
		}
	}
	if len(runs) != 2 || runs[0]["id"] != "show:"+itoa(r1) || runs[0]["title"] != "Testing in Go" || runs[0]["time"] != "08:00" || num(runs[0], "minutes") != 22.5 ||
		runs[0]["sub"] != "23 min of 20 planned" || runs[1]["sub"] != "19 min of 20 planned" {
		t.Fatalf("rehearsals: %v", runs)
	}
	if it := item(t, day, "Garden club"); it["id"] != "talk:"+itoa(planned) || it["kind"] != "talk" || it["time"] != "18:30" || it["done"] != nil ||
		it["sub"] != "Library · 1 slide · not rehearsed yet" || it["url"] != deckPath(planned) {
		t.Fatalf("the talk planned: %v", it)
	}
	if it := item(t, day, "Fresh deck"); it["kind"] != "new deck" || it["time"] != "09:15" || it["sub"] != "1 slide" || it["url"] != deckPath(made) {
		t.Fatalf("the new deck: %v", it)
	}
	// a planned talk on its own, ahead
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-03-17", nil); d["summary"] != "1 talk planned" || len(items(t, d)) != 1 {
		t.Fatalf("next week: %v", d)
	}
	// a day with nothing, and somebody else's day
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2019-03-04", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("an empty day: %v", d)
	}
	if d := a.ok(a.bob, "GET", "/api/v1/pocket/day/2026-03-10?tz=Europe/Sarajevo", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("bob sees alice's day: %v", d)
	}
	a.expect(400, a.alice, "GET", "/api/v1/pocket/day/yesterday", nil)
	// the old route still answers as before
	if old := a.ok(a.alice, "GET", "/api/v1/day/2026-03-17", nil); len(rows(old["items"])) != 1 {
		t.Fatalf("/day changed: %v", old)
	}
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	// no decks: nothing to show
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || w["metrics"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("without decks: %v", w)
	}
	now := time.Now()
	talk := a.deck(a.alice, "Testing in Go", "talk")
	a.setDeck(a.alice, talk, map[string]any{"talk_title": "Why tests", "talk_place": "DevCon", "talk_at": now.AddDate(0, 0, 5).Format("2006-01-02") + "T14:00", "target_minutes": 20})
	a.show("Alice", talk, store.KindRehearsal, now.AddDate(0, 0, -10), 30) // before the deck was last given
	a.show("Alice", talk, store.KindTalk, now.AddDate(0, 0, -9), 25)
	a.show("Alice", talk, store.KindRehearsal, now.AddDate(0, 0, -2), 15)
	a.show("Alice", talk, store.KindRehearsal, now.AddDate(0, 0, -1), 18)
	later := a.deck(a.alice, "Later talk", "blank")
	a.setDeck(a.alice, later, map[string]any{"talk_at": now.AddDate(0, 0, 30).Format("2006-01-02")})
	for _, title := range []string{"One", "Two", "Three", "Four"} {
		a.deck(a.alice, title, "blank")
	}

	w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	if w["line"] != "Talk at DevCon in 5 days" || w["url"] != "/talks" {
		t.Fatalf("widget: %v", w)
	}
	ms := rows(w["metrics"])
	for i, want := range [][2]string{{"Rehearsals", "2"}, {"Last run", "18 min"}, {"Planned", "20 min"}} {
		if len(ms) != 3 || ms[i]["label"] != want[0] || ms[i]["value"] != want[1] {
			t.Fatalf("metrics: %v", ms)
		}
	}
	got := items(t, w)
	if len(got) != widgetDecks || got[0]["id"] != itoa(talk) || got[0]["title"] != "Why tests" || got[0]["detail"] != "Rehearsed twice, last run 18 min (planned 20 min)" ||
		got[0]["url"] != deckPath(talk) || got[1]["title"] != "Four" || got[1]["detail"] != "1 slide · edited today" || got[3]["title"] != "Two" {
		t.Fatalf("the next talk, then the decks changed last: %v", got)
	}
	// tomorrow, without a place, never rehearsed
	soon := a.deck(a.alice, "Lightning talk", "blank")
	a.setDeck(a.alice, soon, map[string]any{"talk_at": now.AddDate(0, 0, 1).Format("2006-01-02") + "T09:00"})
	w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	if w["line"] != "Next talk tomorrow at 09:00" || items(t, w)[0]["detail"] != "Not rehearsed yet" || len(rows(w["metrics"])) != 1 {
		t.Fatalf("tomorrow: %v", w)
	}
	// decks, and no talk coming up
	a.deck(a.bob, "Bob's deck", "blank")
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != "No talks coming up" || w["url"] != "/" || len(items(t, w)) != 1 || w["metrics"] != nil {
		t.Fatalf("bob's widget: %v", w)
	}
}

// A link token with one pocket:* scope reaches its one route and nothing else.
func TestPocketLinkTokens(t *testing.T) {
	a, _ := newCapApp(t)
	d := a.deck(a.alice, "Garden talk", "blank")
	today := time.Now().Format("2006-01-02")
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_alice_search", "GET", "/api/v1/pocket/search?q=garden", nil, 200},
		{"pcx_alice_day", "GET", "/api/v1/pocket/day/" + today, nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_alice_search", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/search?q=garden", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/search?q=garden", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		// no quick capture here: the route does not exist, whatever the scope
		{"pcx_alice_capture", "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000001", "text": "x"}, 404},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/widget", nil, 403},
		// the rest of the app
		{"pcx_alice_widget", "GET", "/api/v1/summary", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/home", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/decks", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/search?q=garden", nil, 403},
		{"pcx_alice_search", "GET", fmt.Sprintf("/api/v1/decks/%d", d), nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/day/" + today, nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/shows", nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/decks", map[string]any{"title": "x"}, 403},
		{"pcx_alice_widget", "PUT", fmt.Sprintf("/api/v1/decks/%d", d), map[string]any{"title": "x"}, 403},
		{"pcx_alice_day", "GET", "/api/v1/settings", nil, 403},
		// the app's own scopes do not open the new routes
		{"pcx_alice_read", "GET", "/api/v1/pocket/search?q=garden", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/search?q=garden", nil, 200},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, m, _ := a.call(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%v)", c.tok, c.method, c.path, code, c.want, m)
		}
	}
	// the token acts for its person only
	if got := items(t, a.ok("pcx_alice_search", "GET", "/api/v1/pocket/search?q=garden", nil)); len(got) != 1 || got[0]["title"] != "Garden talk" {
		t.Fatalf("alice through the link: %v", got)
	}
	if got := items(t, a.ok("pcx_bob_search", "GET", "/api/v1/pocket/search?q=garden", nil)); len(got) != 0 {
		t.Fatalf("bob through the link finds alice's deck: %v", got)
	}
	if w := a.ok("pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil); len(items(t, w)) != 0 {
		t.Fatalf("bob's widget through the link: %v", w)
	}
	if day := a.ok("pcx_alice_day", "GET", "/api/v1/pocket/day/"+today, nil); day["summary"] != "1 new deck" {
		t.Fatalf("alice's day through the link: %v", day)
	}
	if day := a.ok("pcx_bob_day", "GET", "/api/v1/pocket/day/"+today, nil); len(items(t, day)) != 0 {
		t.Fatalf("bob's day through the link: %v", day)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), false},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/day/2026-01-02"), false},
		{core.ScopeWidget, req("GET", "/api/v1/summary"), false},
		{core.ScopeSearch, req("GET", "/api/v1/search"), false},
		{"slides:read", req("GET", "/api/v1/pocket/widget"), false},
		{"slides:write", req("GET", "/api/v1/pocket/search"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Slides tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	a, fc := newCapApp(t)
	if err := a.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	fc.mu.Lock()
	m := fc.manifest
	fc.mu.Unlock()
	if m["slug"] != "slides" || m["category"] != "Work" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	if search == nil || fmt.Sprint(search["bangs"]) != "[slides deck talk]" || fmt.Sprint(search["kinds"]) != "[deck]" ||
		day == nil || day["label"] != "Slides" || day["one"] != "talk" || day["many"] != "talks" || widget == nil || widget["title"] != "Next talk" || m["capture"] != nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, m["capture"])
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"slides:read", "slides:write", core.ScopeSearch, core.ScopeDay, core.ScopeWidget} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
	if offered[core.ScopeCapture] {
		t.Error("pocket:capture is offered without a capture")
	}
}
