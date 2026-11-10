package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// Bringing decks in: Markdown (a slide per heading or per ---, the way
// Marp, Slidev, reveal.js and remark write them), and PowerPoint files
// (.pptx; Keynote and Google Slides save them too): the words, the speaker
// notes and the first picture of every slide.

type draftSlide struct {
	Layout, Title, Subtitle, Body, Body2, Notes string
	Data                                        string
	Image                                       []byte
	ImageMime                                   string
}

var (
	reMDImage  = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reMDHead   = regexp.MustCompile(`^(#{1,3})\s+(.+?)\s*#*$`)
	reMDQuote  = regexp.MustCompile(`^>\s?(.*)$`)
	reHTMLNote = regexp.MustCompile(`(?s)<!--\s*(.*?)\s*-->`)
)

// parseMarkdown turns Markdown into slides. With --- lines, each part is a
// slide; without them, every # or ## heading starts one.
func parseMarkdown(text string) (title string, out []draftSlide) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	// front matter
	if strings.HasPrefix(text, "---\n") {
		if i := strings.Index(text[4:], "\n---\n"); i >= 0 {
			for _, l := range strings.Split(text[4:4+i], "\n") {
				if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "title" {
					title = strings.Trim(strings.TrimSpace(v), `"'`)
				}
			}
			text = text[4+i+5:]
		}
	}
	var parts []string
	lines := strings.Split(text, "\n")
	hasRule := false
	inCode := false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inCode = !inCode
		}
		if !inCode && strings.TrimSpace(l) == "---" {
			hasRule = true
		}
	}
	if hasRule {
		cur := []string{}
		inCode = false
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "```") {
				inCode = !inCode
			}
			if !inCode && strings.TrimSpace(l) == "---" {
				parts = append(parts, strings.Join(cur, "\n"))
				cur = []string{}
				continue
			}
			cur = append(cur, l)
		}
		parts = append(parts, strings.Join(cur, "\n"))
	} else {
		cur := []string{}
		inCode = false
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "```") {
				inCode = !inCode
			}
			if m := reMDHead.FindStringSubmatch(l); !inCode && m != nil && len(m[1]) <= 2 && len(cur) > 0 && strings.TrimSpace(strings.Join(cur, "")) != "" {
				parts = append(parts, strings.Join(cur, "\n"))
				cur = []string{}
			}
			cur = append(cur, l)
		}
		parts = append(parts, strings.Join(cur, "\n"))
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, mdSlide(p))
		if len(out) >= 300 {
			break
		}
	}
	if title == "" && len(out) > 0 {
		title = out[0].Title
	}
	if len(out) > 0 && len(out) > 1 {
		f := &out[0]
		if f.Layout == "bullets" && f.Body == "" || (f.Layout == "bullets" && len(strings.Split(strings.TrimSpace(f.Body), "\n")) == 1 && !strings.HasPrefix(strings.TrimSpace(f.Body), "-")) {
			f.Layout, f.Subtitle, f.Body = "title", strings.TrimSpace(f.Body), ""
		}
	}
	return title, out
}

func mdSlide(p string) draftSlide {
	sl := draftSlide{Layout: "bullets"}
	// speaker notes: <!-- ... -->, or everything after "???" / "Note:" / "Notes:"
	if m := reHTMLNote.FindAllStringSubmatch(p, -1); m != nil {
		for _, x := range m {
			sl.Notes = strings.TrimSpace(sl.Notes + "\n" + x[1])
		}
		p = reHTMLNote.ReplaceAllString(p, "")
	}
	lines := strings.Split(p, "\n")
	body := []string{}
	quote := []string{}
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "???" || strings.EqualFold(t, "note:") || strings.EqualFold(t, "notes:") {
			sl.Notes = strings.TrimSpace(sl.Notes + "\n" + strings.TrimSpace(strings.Join(lines[i+1:], "\n")))
			break
		}
		if low := strings.ToLower(t); strings.HasPrefix(low, "note: ") || strings.HasPrefix(low, "notes: ") {
			_, v, _ := strings.Cut(t, ":")
			sl.Notes = strings.TrimSpace(sl.Notes + "\n" + strings.TrimSpace(v) + "\n" + strings.TrimSpace(strings.Join(lines[i+1:], "\n")))
			break
		}
		if m := reMDHead.FindStringSubmatch(t); m != nil && sl.Title == "" {
			sl.Title = stripMD(m[2])
			if len(m[1]) == 1 && i < 3 {
				sl.Layout = "section"
			}
			continue
		}
		if m := reMDHead.FindStringSubmatch(t); m != nil && sl.Subtitle == "" && len(body) == 0 {
			sl.Subtitle = stripMD(m[2])
			continue
		}
		if m := reMDQuote.FindStringSubmatch(t); m != nil {
			quote = append(quote, m[1])
			continue
		}
		body = append(body, reMDImage.ReplaceAllString(strings.TrimRight(l, " "), ""))
	}
	b := strings.TrimSpace(strings.Join(body, "\n"))
	b = strings.ReplaceAll(b, "\n\n\n", "\n\n")
	switch {
	case len(quote) > 0 && b == "":
		sl.Layout, sl.Body = "quote", strings.TrimSpace(strings.Join(quote, "\n"))
		if i := strings.LastIndex(sl.Body, "— "); i > 0 {
			sl.Body, sl.Subtitle = strings.TrimSpace(sl.Body[:i]), strings.TrimSpace(sl.Body[i+len("— "):])
		}
	case strings.HasPrefix(b, "```") && strings.HasSuffix(b, "```"):
		inner := strings.TrimSuffix(strings.TrimPrefix(b, "```"), "```")
		lang, code, _ := strings.Cut(inner, "\n")
		sl.Layout, sl.Body, sl.Data = "code", strings.TrimRight(code, "\n"), fmt.Sprintf(`{"lang":%q}`, strings.TrimSpace(lang))
	case sl.Layout == "section" && b != "":
		sl.Layout, sl.Body = "bullets", b
	default:
		sl.Body = b
		if len(quote) > 0 {
			sl.Body = strings.TrimSpace(sl.Body + "\n\n" + strings.Join(quote, "\n"))
		}
	}
	if sl.Layout == "bullets" && sl.Title == "" && sl.Body != "" && !strings.Contains(sl.Body, "\n") {
		sl.Layout = "blank"
	}
	return sl
}

func stripMD(v string) string {
	return strings.TrimSpace(strings.NewReplacer("**", "", "__", "", "`", "").Replace(v))
}

// ---- PowerPoint ----

type xmlNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []xmlNode  `xml:",any"`
	Text    string     `xml:",chardata"`
}

func (n *xmlNode) attr(local string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func (n *xmlNode) walk(f func(*xmlNode) bool) {
	if !f(n) {
		return
	}
	for i := range n.Nodes {
		n.Nodes[i].walk(f)
	}
}

func (n *xmlNode) find(local string) *xmlNode {
	var out *xmlNode
	n.walk(func(x *xmlNode) bool {
		if out != nil {
			return false
		}
		if x.XMLName.Local == local && x != n {
			out = x
			return false
		}
		return true
	})
	return out
}

// paragraphs are a shape's lines of text, each with its indent level.
func (n *xmlNode) paragraphs() []struct {
	lvl  int
	text string
	list bool
} {
	out := []struct {
		lvl  int
		text string
		list bool
	}{}
	n.walk(func(x *xmlNode) bool {
		if x.XMLName.Local != "p" {
			return true
		}
		lvl, list := 0, false
		var b strings.Builder
		x.walk(func(y *xmlNode) bool {
			switch y.XMLName.Local {
			case "pPr":
				lvl, _ = strconv.Atoi(y.attr("lvl"))
				if y.find("buNone") == nil && (y.find("buChar") != nil || y.find("buAutoNum") != nil) {
					list = true
				}
			case "t":
				b.WriteString(y.Text)
			case "br":
				b.WriteString(" ")
			}
			return true
		})
		if t := strings.TrimSpace(b.String()); t != "" {
			out = append(out, struct {
				lvl  int
				text string
				list bool
			}{lvl, t, list})
		}
		return false
	})
	return out
}

func readZip(zr *zip.Reader, name string, max int64) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			b, err := io.ReadAll(io.LimitReader(rc, max+1))
			if int64(len(b)) > max {
				return nil, &userErr{"a part of the file is too big"}
			}
			return b, err
		}
	}
	return nil, fmt.Errorf("%s missing", name)
}

func parseXML(b []byte) (*xmlNode, error) {
	var n xmlNode
	if err := xml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// rels reads a .rels file: id → target, resolved against dir.
func rels(zr *zip.Reader, file string) map[string]string {
	dir, base := path.Split(file)
	out := map[string]string{}
	b, err := readZip(zr, dir+"_rels/"+base+".rels", 4<<20)
	if err != nil {
		return out
	}
	n, err := parseXML(b)
	if err != nil {
		return out
	}
	for _, r := range n.Nodes {
		t := r.attr("Target")
		if !strings.HasPrefix(t, "/") {
			t = path.Clean(dir + t)
		} else {
			t = strings.TrimPrefix(t, "/")
		}
		out[r.attr("Id")] = t
	}
	return out
}

// parsePPTX reads a PowerPoint file's slides.
func parsePPTX(data []byte) (string, []draftSlide, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", nil, &userErr{"that is not a PowerPoint (.pptx) file"}
	}
	pres, err := readZip(zr, "ppt/presentation.xml", 8<<20)
	if err != nil {
		return "", nil, &userErr{"that is not a PowerPoint (.pptx) file"}
	}
	pn, err := parseXML(pres)
	if err != nil {
		return "", nil, &userErr{"the PowerPoint file could not be read"}
	}
	prel := rels(zr, "ppt/presentation.xml")
	order := []string{}
	if lst := pn.find("sldIdLst"); lst != nil {
		for _, s := range lst.Nodes {
			for _, a := range s.Attrs {
				if a.Name.Local == "id" && strings.Contains(a.Name.Space, "relationships") {
					if t := prel[a.Value]; t != "" {
						order = append(order, t)
					}
				}
			}
		}
	}
	if len(order) == 0 {
		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
				order = append(order, f.Name)
			}
		}
		sort.Slice(order, func(i, j int) bool { return slideNum(order[i]) < slideNum(order[j]) })
	}
	title := ""
	if b, err := readZip(zr, "docProps/core.xml", 1<<20); err == nil {
		if n, err := parseXML(b); err == nil {
			if t := n.find("title"); t != nil {
				title = strings.TrimSpace(t.Text)
			}
		}
	}
	out := []draftSlide{}
	for _, file := range order {
		if len(out) >= 300 {
			break
		}
		b, err := readZip(zr, file, 16<<20)
		if err != nil {
			continue
		}
		n, err := parseXML(b)
		if err != nil {
			continue
		}
		r := rels(zr, file)
		sl := draftSlide{Layout: "bullets"}
		var bodies [][]string
		kind := ""
		var imgRef string
		n.walk(func(x *xmlNode) bool {
			switch x.XMLName.Local {
			case "sp":
				ph := x.find("ph")
				typ := ""
				if ph != nil {
					typ = firstNonEmpty(ph.attr("type"), "body")
				}
				paras := x.paragraphs()
				if len(paras) == 0 {
					return false
				}
				switch typ {
				case "title", "ctrTitle":
					if sl.Title == "" {
						t := []string{}
						for _, p := range paras {
							t = append(t, p.text)
						}
						sl.Title = strings.Join(t, " ")
						if typ == "ctrTitle" {
							kind = "title"
						}
					}
				case "subTitle":
					t := []string{}
					for _, p := range paras {
						t = append(t, p.text)
					}
					sl.Subtitle = strings.Join(t, " ")
				case "sldNum", "dt", "ftr":
				default:
					lines := []string{}
					for _, p := range paras {
						pre := strings.Repeat("  ", clampInt(p.lvl, 0, 3))
						if p.list || typ == "body" || typ == "obj" || p.lvl > 0 {
							lines = append(lines, pre+"- "+p.text)
						} else {
							lines = append(lines, pre+p.text)
						}
					}
					bodies = append(bodies, lines)
				}
				return false
			case "pic":
				if imgRef == "" {
					if bl := x.find("blip"); bl != nil {
						imgRef = bl.attr("embed")
					}
				}
				return false
			}
			return true
		})
		if len(bodies) > 0 {
			sl.Body = strings.Join(bodies[0], "\n")
		}
		if len(bodies) > 1 {
			sl.Body2 = strings.Join(bodies[1], "\n")
			for _, extra := range bodies[2:] {
				sl.Body2 += "\n" + strings.Join(extra, "\n")
			}
		}
		if t := r[imgRef]; imgRef != "" && t != "" {
			if img, err := readZip(zr, t, maxUpload); err == nil {
				mime := http.DetectContentType(img)
				if allowedMime[strings.Split(mime, ";")[0]] {
					sl.Image, sl.ImageMime = img, strings.Split(mime, ";")[0]
				}
			}
		}
		// speaker notes
		for _, t := range r {
			if strings.Contains(t, "notesSlides/") {
				if nb, err := readZip(zr, t, 4<<20); err == nil {
					if nn, err := parseXML(nb); err == nil {
						notes := []string{}
						nn.walk(func(x *xmlNode) bool {
							if x.XMLName.Local == "sp" {
								if ph := x.find("ph"); ph != nil && ph.attr("type") == "body" {
									for _, p := range x.paragraphs() {
										notes = append(notes, p.text)
									}
								}
								return false
							}
							return true
						})
						sl.Notes = strings.Join(notes, "\n")
					}
				}
			}
		}
		switch {
		case kind == "title" || (len(out) == 0 && sl.Body == "" && sl.Image == nil):
			sl.Layout = "title"
			if sl.Subtitle == "" && sl.Body != "" {
				sl.Subtitle, sl.Body = strings.TrimPrefix(firstLine(sl.Body), "- "), ""
			}
		case sl.Image != nil && sl.Body == "":
			sl.Layout, sl.Subtitle = "image", firstNonEmpty(sl.Subtitle, sl.Title)
			sl.Title = ""
		case sl.Image != nil:
			sl.Layout = "image_right"
		case sl.Body2 != "":
			sl.Layout = "two"
		case sl.Body == "" && sl.Title != "":
			sl.Layout = "section"
		}
		out = append(out, sl)
	}
	if len(out) == 0 {
		return "", nil, &userErr{"there are no slides in that file"}
	}
	if title == "" {
		title = out[0].Title
	}
	return title, out, nil
}

var reSlideNum = regexp.MustCompile(`(\d+)\.xml$`)

func slideNum(name string) int {
	m := reSlideNum.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// makeDeck stores a deck from drafted slides.
func (s *Server) makeDeck(ctx context.Context, u *store.User, title string, drafts []draftSlide, source, ref string) (*store.Deck, error) {
	d := s.newDeck(ctx, u.ID, firstNonEmpty(cleanText(title, 200), "Imported deck"))
	if source != "" {
		d.Source = source
	}
	d.SourceRef = ref
	if err := s.store.CreateDeck(ctx, d); err != nil {
		return nil, err
	}
	if err := s.addDrafts(ctx, u, d, drafts, -1); err != nil {
		return nil, err
	}
	return s.store.GetDeck(ctx, u.ID, d.ID)
}

// addDrafts adds drafted slides to a deck at pos (-1 = at the end).
func (s *Server) addDrafts(ctx context.Context, u *store.User, d *store.Deck, drafts []draftSlide, pos int) error {
	for i, ds := range drafts {
		sl := &store.Slide{DeckID: d.ID, UserID: u.ID, Position: -1, Layout: ds.Layout, Title: clip(ds.Title, 300), Subtitle: clip(ds.Subtitle, 300),
			Body: clip(ds.Body, 8000), Body2: clip(ds.Body2, 8000), Notes: clip(ds.Notes, maxNotes), Data: cleanData(json.RawMessage(firstNonEmpty(ds.Data, "{}"))), Source: sourceOf(ctx)}
		if pos >= 0 {
			sl.Position = pos + i
		}
		if !validLayout(sl.Layout) {
			sl.Layout = "bullets"
		}
		if len(ds.Image) > 0 {
			up := &store.Upload{UserID: u.ID, DeckID: d.ID, Name: fmt.Sprintf("slide-%d", i+1), Mime: ds.ImageMime, Data: ds.Image}
			if err := s.store.SaveUpload(ctx, up); err == nil {
				sl.ImageID = &up.ID
			}
		}
		if err := s.store.CreateSlide(ctx, sl); err != nil {
			return err
		}
	}
	return nil
}

// handleImportDeck makes a deck from Markdown text or a PowerPoint file.
func (s *Server) handleImportDeck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var title string
	var drafts []draftSlide
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "a file can be up to 100 MB")
			return
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "no file")
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 100<<20))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the file could not be read")
			return
		}
		title, drafts, err = s.draftsFromFile(hdr.Filename, data)
		if err != nil {
			s.userFail(w, err, "import")
			return
		}
		title = firstNonEmpty(r.FormValue("title"), title)
	} else {
		var in struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		}
		if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
			writeErr(w, http.StatusBadRequest, "paste the Markdown to make slides from")
			return
		}
		title, drafts = parseMarkdown(in.Text)
		title = firstNonEmpty(in.Title, title)
	}
	if len(drafts) == 0 {
		writeErr(w, http.StatusBadRequest, "there are no slides in that")
		return
	}
	d, err := s.makeDeck(ctx, u, title, drafts, "", "")
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deck": d, "slides": d.Slides, "text": fmt.Sprintf("%d %s brought in", d.Slides, plural(d.Slides, "slide", "slides"))})
}

func (s *Server) draftsFromFile(name string, data []byte) (string, []draftSlide, error) {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".pptx", ".potx", ".ppsx":
		t, d, err := parsePPTX(data)
		if t == "" {
			t = strings.TrimSuffix(path.Base(name), path.Ext(name))
		}
		return t, d, err
	case ".md", ".markdown", ".txt":
		t, d := parseMarkdown(string(data))
		return firstNonEmpty(t, strings.TrimSuffix(path.Base(name), path.Ext(name))), d, nil
	case ".ppt", ".key", ".odp":
		return "", nil, &userErr{"save it as PowerPoint (.pptx) first, then bring that in"}
	}
	return "", nil, &userErr{"PowerPoint (.pptx) or Markdown (.md) files only"}
}
