package server

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

func (a *app) deck(auth, title, template string) int64 {
	a.t.Helper()
	m := a.expect(201, auth, "POST", "/api/v1/decks", map[string]any{"title": title, "template": template})
	return int64(num(obj(m, "deck"), "id"))
}

func (a *app) slides(auth string, d int64) []map[string]any {
	a.t.Helper()
	m := a.ok(auth, "GET", fmt.Sprintf("/api/v1/decks/%d", d), nil)
	return rows(m["slides"])
}

func testPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for x := 0; x < 8; x++ {
		img.Set(x, 2, color.RGBA{200, 120, 10, 255})
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func (a *app) upload(auth string, d int64) (int, int64) {
	a.t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	if d > 0 {
		_ = mw.WriteField("deck", fmt.Sprint(d))
	}
	fw, _ := mw.CreateFormFile("file", "pic.png")
	_, _ = fw.Write(testPNG())
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.url+"/api/v1/uploads", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, int64(num(obj(m, "upload"), "id"))
}

func (a *app) get(path string) (int, string) {
	a.t.Helper()
	res, err := http.Get(a.url + path)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestDecksArePerPerson(t *testing.T) {
	a := newApp(t, nil)
	d := a.deck(a.alice, "Alice's talk", "talk")
	if n := len(a.slides(a.alice, d)); n < 8 {
		t.Fatalf("template made %d slides", n)
	}
	sid := int64(num(a.slides(a.alice, d)[1], "id"))
	_, up := a.upload(a.alice, d)
	p := fmt.Sprintf("/api/v1/decks/%d", d)
	// Bob (a member) and the admin view of others: nothing
	a.expect(404, a.bob, "GET", p, nil)
	a.expect(404, a.bob, "PUT", p, map[string]any{"title": "mine now"})
	a.expect(404, a.bob, "DELETE", p, nil)
	a.expect(404, a.bob, "POST", p+"/slides", map[string]any{"layout": "bullets"})
	a.expect(404, a.bob, "PUT", fmt.Sprintf("/api/v1/slides/%d", sid), map[string]any{"title": "x"})
	a.expect(404, a.bob, "DELETE", fmt.Sprintf("/api/v1/slides/%d", sid), nil)
	a.expect(404, a.bob, "GET", fmt.Sprintf("/api/v1/uploads/%d", up), nil)
	a.expect(404, a.bob, "POST", p+"/live", map[string]any{"action": "start"})
	a.expect(404, a.bob, "GET", p+"/export.md", nil)
	if l := rows(a.ok(a.bob, "GET", "/api/v1/decks", nil)["decks"]); len(l) != 0 {
		t.Fatalf("bob sees %d decks", len(l))
	}
	if l := rows(a.ok(a.bob, "GET", "/api/v1/search?q=talk", nil)["results"]); len(l) != 0 {
		t.Fatal("bob finds alice's deck")
	}
	// Bob may not put Alice's picture on his slide
	bd := a.deck(a.bob, "Bob's", "blank")
	bs := int64(num(a.slides(a.bob, bd)[0], "id"))
	a.expect(400, a.bob, "PUT", fmt.Sprintf("/api/v1/slides/%d", bs), map[string]any{"image_id": up})
	// nor use her theme
	m := a.ok(a.alice, "POST", "/api/v1/themes", map[string]any{"name": "Mine", "accent": "#112233"})
	a.expect(400, a.bob, "PUT", fmt.Sprintf("/api/v1/decks/%d", bd), map[string]any{"theme_id": num(obj(m, "theme"), "id")})
	// Alice still has it all
	a.ok(a.alice, "GET", p, nil)
}

func TestSlidesEditOrderAndLayouts(t *testing.T) {
	a := newApp(t, nil)
	d := a.deck(a.alice, "Order", "blank")
	p := fmt.Sprintf("/api/v1/decks/%d", d)
	first := int64(num(a.slides(a.alice, d)[0], "id"))
	b := a.expect(201, a.alice, "POST", p+"/slides", map[string]any{"layout": "bullets", "title": "B", "body": "- one\n- two"})
	c := a.expect(201, a.alice, "POST", p+"/slides", map[string]any{"layout": "chart", "title": "C", "data": map[string]any{"chart": map[string]any{"kind": "weird", "labels": []string{"a", "b"}, "values": []float64{1, 2, 3}}}})
	cid := int64(num(obj(c, "slide"), "id"))
	if k := str(obj(obj(obj(c, "slide"), "data"), "chart"), "kind"); k != "bar" {
		t.Fatalf("chart kind not cleaned: %q", k)
	}
	a.expect(400, a.alice, "POST", p+"/slides", map[string]any{"layout": "nope"})
	// insert after the first
	ins := a.expect(201, a.alice, "POST", p+"/slides", map[string]any{"layout": "section", "title": "S", "after": first})
	if num(obj(ins, "slide"), "position") != 1 {
		t.Fatal("not inserted after")
	}
	bid := int64(num(obj(b, "slide"), "id"))
	a.ok(a.alice, "PUT", p+"/order", map[string]any{"ids": []int64{cid, bid}})
	got := a.slides(a.alice, d)
	if int64(num(got[0], "id")) != cid || int64(num(got[1], "id")) != bid {
		t.Fatalf("order %v", got)
	}
	// keep a slide as my layout, start a slide from it
	l := a.expect(201, a.alice, "POST", fmt.Sprintf("/api/v1/slides/%d/keep", bid), map[string]any{"name": "Points"})
	lid := num(obj(l, "layout"), "id")
	n := a.expect(201, a.alice, "POST", p+"/slides", map[string]any{"layout_id": lid})
	if str(obj(n, "slide"), "body") != "- one\n- two" {
		t.Fatal("layout not used")
	}
	a.expect(404, a.bob, "POST", fmt.Sprintf("/api/v1/decks/%d/slides", a.deck(a.bob, "x", "blank")), map[string]any{"layout_id": lid})
	// copy a deck: same slides, new links
	cp := a.expect(201, a.alice, "POST", p+"/copy", nil)
	if num(obj(cp, "deck"), "slides") != float64(len(got)+1) || str(obj(cp, "deck"), "share_key") == "" {
		t.Fatal("copy")
	}
}

// presenting: the screens follow, the audience sees only what is shown
func TestPresentingAndAudience(t *testing.T) {
	a := newApp(t, nil)
	d := a.deck(a.alice, "Live", "blank")
	p := fmt.Sprintf("/api/v1/decks/%d", d)
	a.ok(a.alice, "POST", p+"/slides", map[string]any{"layout": "bullets", "title": "Points", "body": "- a\n- b", "data": map[string]any{"build": true}, "notes": "SECRET NOTES"})
	a.ok(a.alice, "POST", p+"/slides", map[string]any{"layout": "bullets", "title": "Hidden one", "hidden": true})
	pm := a.ok(a.alice, "POST", p+"/slides", map[string]any{"layout": "poll", "title": "Which?", "data": map[string]any{"options": []string{"x", "y"}, "correct": 2}})
	poll := int64(num(obj(pm, "slide"), "id"))
	a.ok(a.alice, "POST", p+"/slides", map[string]any{"layout": "end", "title": "Future slide"})
	key := str(obj(a.ok(a.alice, "GET", p, nil), "deck"), "live_key")
	// not on yet
	au := a.ok("", "GET", "/api/v1/public/live/"+key, nil)
	if obj(au, "state")["live"] == true {
		t.Fatal("live before start")
	}
	// a rehearsal is not shown to the audience
	st := a.ok(a.alice, "POST", p+"/live", map[string]any{"action": "start", "kind": "rehearsal"})
	remote := strings.TrimPrefix(str(st, "remote_url"), "http://slides.test/r/")
	if obj(a.ok("", "GET", "/api/v1/public/live/"+key, nil), "state")["slide"] != nil {
		t.Fatal("rehearsal shown")
	}
	a.ok(a.alice, "POST", p+"/live", map[string]any{"action": "start", "kind": "talk"})
	// step through: title, point 1, point 2, (hidden skipped) poll, answer, end
	next := func() map[string]any {
		return a.ok("", "POST", "/api/v1/public/remote/"+remote, map[string]any{"action": "next"})
	}
	next()
	s1 := a.ok(a.alice, "GET", p+"/live", nil)
	if num(s1, "pos") != 1 || num(s1, "step") != 0 || num(s1, "steps") != 2 {
		t.Fatalf("after one next: %v", s1)
	}
	next()
	next()
	next() // to the poll
	s2 := a.ok(a.alice, "GET", p+"/live", nil)
	if int64(num(s2, "slide_id")) != poll {
		t.Fatalf("hidden slide not skipped: %v", s2)
	}
	// the audience: no notes, no hidden slide, no future slide, no answer yet
	au = a.ok("", "GET", "/api/v1/public/live/"+key, nil)
	raw, _ := json.Marshal(au)
	for _, bad := range []string{"SECRET NOTES", "Hidden one", "Future slide", `"correct":2`} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("audience sees %q", bad)
		}
	}
	// votes: only for the poll on the screen, once per voter
	a.expect(400, "", "POST", "/api/v1/public/live/"+key+"/vote", map[string]any{"slide_id": poll, "choice": 5, "voter": "voter-aaaa1"})
	for _, v := range []string{"voter-aaaa1", "voter-bbbb2", "voter-aaaa1"} {
		a.ok("", "POST", "/api/v1/public/live/"+key+"/vote", map[string]any{"slide_id": poll, "choice": 1, "voter": v})
	}
	tally := obj(obj(a.ok(a.alice, "GET", p+"/live", nil), "tally"), fmt.Sprint(poll))
	if num(tally, "1") != 2 {
		t.Fatalf("tally %v", tally)
	}
	// questions: asked, liked once, hidden by the presenter
	q := a.expect(201, "", "POST", "/api/v1/public/live/"+key+"/questions", map[string]any{"body": "Why?", "name": "Mia", "voter": "voter-aaaa1"})
	qid := int64(num(obj(q, "question"), "id"))
	a.ok("", "POST", fmt.Sprintf("/api/v1/public/live/%s/questions/%d/like", key, qid), map[string]any{"voter": "voter-bbbb2"})
	a.ok("", "POST", fmt.Sprintf("/api/v1/public/live/%s/questions/%d/like", key, qid), map[string]any{"voter": "voter-bbbb2"})
	qs := rows(a.ok(a.alice, "GET", p+"/live", nil)["questions"])
	if len(qs) != 1 || num(qs[0], "votes") != 1 {
		t.Fatalf("questions %v", qs)
	}
	a.expect(404, a.bob, "PUT", fmt.Sprintf("/api/v1/questions/%d", qid), map[string]any{"hidden": true})
	a.ok("", "PUT", fmt.Sprintf("/api/v1/public/remote/%s/questions/%d", remote, qid), map[string]any{"hidden": true})
	if l := rows(obj(a.ok("", "GET", "/api/v1/public/live/"+key, nil), "state")["questions"]); len(l) != 0 {
		t.Fatal("hidden question shown")
	}
	// the answer comes with the next press
	next()
	au = a.ok("", "GET", "/api/v1/public/live/"+key, nil)
	if num(obj(obj(obj(au, "state"), "slide"), "data"), "correct") != 2 {
		t.Fatal("answer not shown after the press")
	}
	// the phone link cannot start or touch another deck; it ends with the show
	a.expect(400, "", "POST", "/api/v1/public/remote/"+remote, map[string]any{"action": "start"})
	next()
	a.ok("", "POST", "/api/v1/public/remote/"+remote, map[string]any{"action": "end"})
	a.expect(404, "", "GET", "/api/v1/public/remote/"+remote, nil)
	shows := rows(a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/shows?deck=%d", d), nil)["shows"])
	if len(shows) != 1 || str(shows[0], "kind") != "talk" || shows[0]["ended_at"] == nil || len(obj(shows[0], "timings")) < 3 {
		t.Fatalf("shows %v", shows)
	}
	if str(shows[0], "remote_key") != "" {
		t.Fatal("remote key listed")
	}
	// after the talk: feedback
	a.ok("", "POST", "/api/v1/public/live/"+key+"/feedback", map[string]any{"rating": 5, "comment": "Great", "voter": "voter-aaaa1"})
	h := a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/shows/%d", int64(num(shows[0], "id"))), nil)
	if len(rows(h["feedback"])) != 1 {
		t.Fatal("feedback")
	}
	a.expect(404, a.bob, "GET", fmt.Sprintf("/api/v1/shows/%d", int64(num(shows[0], "id"))), nil)
}

func TestLiveStreamFollows(t *testing.T) {
	a := newApp(t, nil)
	d := a.deck(a.alice, "Stream", "talk")
	p := fmt.Sprintf("/api/v1/decks/%d", d)
	key := str(obj(a.ok(a.alice, "GET", p, nil), "deck"), "live_key")
	a.ok(a.alice, "POST", p+"/live", map[string]any{"action": "start"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", a.url+"/api/v1/public/live/"+key+"/stream", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	rd := bufio.NewReader(res.Body)
	readState := func() map[string]any {
		for {
			l, err := rd.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(l, "data: ") {
				var m map[string]any
				_ = json.Unmarshal([]byte(strings.TrimPrefix(l, "data: ")), &m)
				if m["type"] == "state" || m["type"] == "ended" {
					return m
				}
			}
		}
	}
	if readState()["live"] != true {
		t.Fatal("not live")
	}
	a.ok(a.alice, "POST", p+"/live", map[string]any{"action": "go", "pos": 3})
	for i := 0; i < 5; i++ {
		if m := readState(); num(m, "pos") == 3 {
			if strings.Contains(fmt.Sprint(m), "notes") && obj(m, "slide")["notes"] != "" && obj(m, "slide")["notes"] != nil {
				t.Fatal("notes streamed to the audience")
			}
			return
		}
	}
	t.Fatal("the audience did not follow")
}

func TestShareLink(t *testing.T) {
	a := newApp(t, nil)
	d := a.deck(a.alice, "Shared", "blank")
	p := fmt.Sprintf("/api/v1/decks/%d", d)
	_, up := a.upload(a.alice, d)
	_, hiddenUp := a.upload(a.alice, d)
	a.ok(a.alice, "POST", p+"/slides", map[string]any{"layout": "image", "image_id": up, "notes": "PRIVATE"})
	a.ok(a.alice, "POST", p+"/slides", map[string]any{"layout": "image", "image_id": hiddenUp, "hidden": true})
	key := str(obj(a.ok(a.alice, "GET", p, nil), "deck"), "share_key")
	a.expect(404, "", "GET", "/api/v1/public/s/"+key, nil)
	a.ok(a.alice, "PUT", p+"/share", map[string]any{"shared": true})
	m := a.ok("", "GET", "/api/v1/public/s/"+key, nil)
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), "PRIVATE") || len(rows(m["slides"])) != 2 {
		t.Fatalf("share shows %s", raw)
	}
	if c, _ := a.get(fmt.Sprintf("/api/v1/public/s/%s/uploads/%d", key, up)); c != 200 {
		t.Fatal("picture on the share link")
	}
	if c, _ := a.get(fmt.Sprintf("/api/v1/public/s/%s/uploads/%d", key, hiddenUp)); c != 404 {
		t.Fatal("hidden slide's picture on the share link")
	}
	// Bob's picture through Alice's link: no
	bd := a.deck(a.bob, "b", "blank")
	_, bup := a.upload(a.bob, bd)
	if c, _ := a.get(fmt.Sprintf("/api/v1/public/s/%s/uploads/%d", key, bup)); c != 404 {
		t.Fatal("someone else's picture")
	}
	// new links: the old one stops
	a.ok(a.alice, "PUT", p+"/share", map[string]any{"new_keys": true})
	a.expect(404, "", "GET", "/api/v1/public/s/"+key, nil)
}

func TestMarkdownImport(t *testing.T) {
	_, drafts := parseMarkdown("---\ntitle: My talk\n---\n# My talk\nA subtitle\n\n---\n\n## Points\n- one\n- two\n\nNote: say hello\n\n---\n\n> Be kind.\n> — Someone\n\n---\n\n```go\nfmt.Println(1)\n```\n")
	if len(drafts) != 4 {
		t.Fatalf("got %d slides", len(drafts))
	}
	if drafts[0].Layout != "title" || drafts[1].Notes != "say hello" || drafts[1].Body != "- one\n- two" || drafts[2].Layout != "quote" || drafts[2].Subtitle != "Someone" || drafts[3].Layout != "code" {
		t.Fatalf("%+v", drafts)
	}
	// no rules: a slide per heading
	_, d2 := parseMarkdown("# Intro\nhello\n## One\n- a\n## Two\n- b\n")
	if len(d2) != 3 {
		t.Fatalf("headings: %d", len(d2))
	}
	a := newApp(t, nil)
	m := a.expect(201, a.alice, "POST", "/api/v1/import/deck", map[string]any{"text": "# A\n---\n## B\n- x"})
	if num(obj(m, "deck"), "slides") != 2 {
		t.Fatal("import")
	}
}

func TestPowerPointImport(t *testing.T) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	put := func(name, body string) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	ns := `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	put("ppt/presentation.xml", `<p:presentation `+ns+`><p:sldIdLst><p:sldId id="256" r:id="rId2"/><p:sldId id="257" r:id="rId3"/></p:sldIdLst></p:presentation>`)
	put("ppt/_rels/presentation.xml.rels", `<Relationships><Relationship Id="rId2" Target="slides/slide2.xml"/><Relationship Id="rId3" Target="slides/slide1.xml"/></Relationships>`)
	put("ppt/slides/slide2.xml", `<p:sld `+ns+`><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="ctrTitle"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>Big title</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:nvPr><p:ph type="subTitle"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>Sub</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`)
	put("ppt/slides/slide1.xml", `<p:sld `+ns+`><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>Points</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:nvPr><p:ph idx="1"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>First</a:t></a:r></a:p><a:p><a:pPr lvl="1"/><a:r><a:t>Deeper</a:t></a:r></a:p></p:txBody></p:sp><p:pic><p:blipFill><a:blip r:embed="rId9"/></p:blipFill></p:pic></p:spTree></p:cSld></p:sld>`)
	put("ppt/slides/_rels/slide1.xml.rels", `<Relationships><Relationship Id="rId9" Target="../media/image1.png"/><Relationship Id="rId4" Target="../notesSlides/notesSlide1.xml"/></Relationships>`)
	w, _ := zw.Create("ppt/media/image1.png")
	_, _ = w.Write(testPNG())
	put("ppt/notesSlides/notesSlide1.xml", `<p:notes `+ns+`><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>Say this</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>`)
	_ = zw.Close()
	title, drafts, err := parsePPTX(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if title != "Big title" || len(drafts) != 2 || drafts[0].Layout != "title" || drafts[0].Subtitle != "Sub" {
		t.Fatalf("%q %+v", title, drafts)
	}
	if drafts[1].Layout != "image_right" || drafts[1].Body != "- First\n  - Deeper" || drafts[1].Notes != "Say this" || len(drafts[1].Image) == 0 {
		t.Fatalf("%+v", drafts[1])
	}
	if _, _, err := parsePPTX([]byte("not a zip")); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestLinkScopes(t *testing.T) {
	r := func(method, path string) *http.Request { return httptest.NewRequest(method, path, nil) }
	read, write := []string{"slides:read"}, []string{"slides:write"}
	cases := []struct {
		scopes []string
		req    *http.Request
		want   bool
	}{
		{read, r("GET", "/api/v1/auth/me"), true},
		{read, r("GET", "/api/v1/decks"), true},
		{read, r("GET", "/api/v1/decks/3"), true},
		{read, r("GET", "/api/v1/decks/3/export.md"), true},
		{read, r("GET", "/api/v1/day/2026-11-10"), true},
		{read, r("GET", "/api/v1/search"), true},
		{read, r("POST", "/api/v1/decks"), false},
		{write, r("POST", "/api/v1/decks"), true},
		{write, r("POST", "/api/v1/decks/3/slides"), true},
		{write, r("PUT", "/api/v1/slides/5"), true},
		{write, r("POST", "/api/v1/decks/3/live"), false},
		{write, r("PUT", "/api/v1/decks/3/share"), false},
		{append(read, write...), r("GET", "/api/v1/export.json"), false},
		{append(read, write...), r("GET", "/api/v1/settings"), false},
		{append(read, write...), r("POST", "/api/v1/uploads"), false},
	}
	for _, c := range cases {
		if got := linkAllows(c.scopes, c.req); got != c.want {
			t.Errorf("%v %s %s = %v, want %v", c.scopes, c.req.Method, c.req.URL.Path, got, c.want)
		}
	}
	ctx := withLinkApp(context.Background(), "notes")
	if !mayChange(ctx, "notes") || mayChange(ctx, "") || mayChange(ctx, "voice") {
		t.Fatal("mayChange")
	}
}

func TestExportImportIsIdempotent(t *testing.T) {
	a := newApp(t, nil)
	d := a.deck(a.alice, "Keep me", "talk")
	_, up := a.upload(a.alice, d)
	sl := a.slides(a.alice, d)
	a.ok(a.alice, "PUT", fmt.Sprintf("/api/v1/slides/%d", int64(num(sl[0], "id"))), map[string]any{"image_id": up})
	a.ok(a.alice, "POST", "/api/v1/themes", map[string]any{"name": "T", "accent": "#123456"})
	p := fmt.Sprintf("/api/v1/decks/%d", d)
	a.ok(a.alice, "POST", p+"/live", map[string]any{"action": "start", "kind": "rehearsal"})
	a.ok(a.alice, "POST", p+"/live", map[string]any{"action": "next"})
	a.ok(a.alice, "POST", p+"/live", map[string]any{"action": "end"})
	res, err := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Header.Set("Authorization", "Bearer "+a.alice)
	resp, err := http.DefaultClient.Do(res)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&doc)
	resp.Body.Close()
	if len(rows(doc["decks"])) != 1 || len(rows(doc["files"])) != 1 {
		t.Fatalf("export %v", doc["files"])
	}
	// into Bob's account, twice
	m1 := a.ok(a.bob, "POST", "/api/v1/import", doc)
	m2 := a.ok(a.bob, "POST", "/api/v1/import", doc)
	if num(m1, "decks") != 1 || num(m2, "decks") != 0 || num(m2, "skipped") != 1 {
		t.Fatalf("import %v %v", m1, m2)
	}
	bd := rows(a.ok(a.bob, "GET", "/api/v1/decks", nil)["decks"])
	if len(bd) != 1 || num(bd[0], "slides") != float64(len(sl)) || num(bd[0], "rehearsals") != 1 {
		t.Fatalf("bob's decks %v", bd)
	}
	bs := a.slides(a.bob, int64(num(bd[0], "id")))
	img := int64(num(bs[0], "image_id"))
	if img == 0 || img == up {
		t.Fatal("picture not copied")
	}
	a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/uploads/%d", img), nil)
	if len(rows(a.ok(a.bob, "GET", "/api/v1/themes", nil)["themes"])) != 1 {
		t.Fatal("theme")
	}
}

func TestSettingsArePersonal(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"theme": "chalk", "footer": "Bob", "title": "Bob's box"})
	if s := a.ok(a.alice, "GET", "/api/v1/settings", nil); str(s, "theme") != "paper" || str(s, "footer") != "" || str(s, "title") != "Pocket Slides" {
		t.Fatalf("alice sees bob's settings: %v", s)
	}
	if s := a.ok(a.bob, "GET", "/api/v1/settings", nil); str(s, "theme") != "chalk" {
		t.Fatal("bob's theme")
	}
	// a new deck uses the person's defaults
	m := a.expect(201, a.bob, "POST", "/api/v1/decks", map[string]any{"title": "x"})
	if str(obj(m, "deck"), "theme") != "chalk" || str(obj(m, "deck"), "footer") != "Bob" {
		t.Fatal("defaults")
	}
}

func TestSteps(t *testing.T) {
	mk := func(layout, body, data string) *store.Slide {
		return &store.Slide{Layout: layout, Body: body, Data: json.RawMessage(data)}
	}
	if n := steps(mk("bullets", "- a\n  - a1\n- b\n1. c", `{"build":true}`)); n != 3 {
		t.Fatalf("bullets %d", n)
	}
	if n := steps(mk("bullets", "- a\n- b", `{}`)); n != 0 {
		t.Fatal("no build")
	}
	if n := steps(mk("poll", "", `{"options":["a","b"],"correct":1}`)); n != 1 {
		t.Fatal("quiz")
	}
}
