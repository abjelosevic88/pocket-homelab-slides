package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Slides as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "slides:read", Name: "See your presentations", Description: "Read your decks — their slides, speaker notes and talks (when and where, how long the rehearsals took, what the audience asked) — and search them: for your journal, search and Pocket AI."},
	{Scope: "slides:write", Name: "Make presentations for you", Description: "Start a deck with slides (from a note, a memo or when you ask Pocket AI) and add slides to your decks. Changes or removes only what that app made itself; never presents, shares or deletes your own decks."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "slides.json", Kind: core.KindPocket, Title: "Every deck with its slides, notes and pictures, your themes and layouts, and your talks and rehearsals"},
		{Path: "/api/v1/export.csv", File: "talks.csv", Kind: core.KindCSV, Title: "Your talks and rehearsals as a table"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", MaxBytes: 1 << 30, Title: "Adds the decks, themes and layouts from another box; decks already here are skipped"},
		{Path: "/api/v1/import/deck", Kind: core.KindMarkdown, Form: "file", MaxBytes: 100 << 20, Title: "Makes a deck from a Markdown file (a slide per heading or per ---); the same file twice makes one deck"},
		{Path: "/api/v1/import/deck", Kind: "pptx", Form: "file", MaxBytes: 100 << 20, Title: "Makes a deck from a PowerPoint file (.pptx, also saved by Keynote and Google Slides): words, speaker notes and pictures; the same file twice makes one deck"},
	},
}

// capabilities are what Pocket Search, Day and Hub may ask Slides
// (docs/capabilities.md in pocket-homelab-core): one route each, in the
// shape every Pocket app answers in (capabilities.go). Register adds the
// pocket:* offers. No quick capture: a deck needs slides.
var capabilities = core.Capabilities{
	Search: &core.Searchable{What: "Your decks, by title, talk and the words on the slides", Kinds: []string{"deck"}, Bangs: []string{"slides", "deck", "talk"}},
	Day:    &core.DayFeed{Label: "Slides", One: "talk", Many: "talks", What: "The talks you gave or had planned that day, the rehearsals with how long they took, and the decks you made"},
	Widget: &core.Widget{Title: "Next talk", What: "How long until your next talk and how the rehearsals went, then the decks you changed last"},
}

// linkRoute is one request a Pocket Core link token may make.
type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(home|summary|search|decks|shows|builtins|layouts|themes|day/\d{4}-\d{2}-\d{2})$`), "slides:read"},
	{"GET", regexp.MustCompile(`^/api/v1/decks/\d+(/export\.md)?$`), "slides:read"},
	{"GET", regexp.MustCompile(`^/api/v1/shows/\d+$`), "slides:read"},
	{"GET", regexp.MustCompile(`^/api/v1/uploads/\d+$`), "slides:read"},
	{"POST", regexp.MustCompile(`^/api/v1/decks$`), "slides:write"},
	{"POST", regexp.MustCompile(`^/api/v1/decks/\d+/slides$`), "slides:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/slides/\d+$`), "slides:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/decks/\d+$`), "slides:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) || capabilities.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "slides" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}

// withLinkApp marks a context as acting for another app (tests, and the auth middleware).
func withLinkApp(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, linkAppKey, slug)
}
