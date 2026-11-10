package server

import (
	"encoding/json"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// The built-in slide layouts. Every slide has one; what each field means
// depends on it (the web page shows only the fields a layout uses).
type layoutInfo struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	About string   `json:"about"`
	Uses  []string `json:"uses"` // title subtitle body body2 image chart table poll code qr
}

var builtinLayouts = []layoutInfo{
	{"title", "Title", "The first slide: a big title and a line under it", []string{"title", "subtitle", "image"}},
	{"section", "Section", "Starts a new part of the talk", []string{"title", "subtitle"}},
	{"bullets", "Title and points", "A heading with a few points", []string{"title", "body"}},
	{"two", "Two columns", "Two lists side by side: before and after, pros and cons", []string{"title", "subtitle", "body", "body2"}},
	{"image_right", "Picture on the right", "Points on the left, a picture on the right", []string{"title", "body", "image"}},
	{"image_left", "Picture on the left", "A picture on the left, points on the right", []string{"title", "body", "image"}},
	{"image", "Full picture", "One picture filling the slide, with a caption", []string{"title", "subtitle", "image"}},
	{"quote", "Quote", "Somebody's words, big, with who said it", []string{"body", "subtitle"}},
	{"number", "Big number", "One number that matters, and what it means", []string{"title", "subtitle", "body"}},
	{"chart", "Chart", "Bars, a line or a pie from a few numbers", []string{"title", "subtitle", "chart"}},
	{"table", "Table", "A small table of rows and columns", []string{"title", "table"}},
	{"code", "Code", "A piece of code, in a monospaced font", []string{"title", "body", "code"}},
	{"poll", "Live poll", "A question the audience answers on their phones while you present", []string{"title", "poll"}},
	{"agenda", "Agenda", "The parts of the talk, from its section slides", []string{"title"}},
	{"end", "The end", "Thank you, how to reach you, and a code to scan for the slides", []string{"title", "subtitle", "qr"}},
	{"blank", "Words only", "Just a few words in the middle", []string{"body"}},
}

func validLayout(k string) bool {
	for _, l := range builtinLayouts {
		if l.Key == k {
			return true
		}
	}
	return false
}

// The built-in themes (the web page holds their colours and fonts).
var builtinThemes = []struct {
	Key, Name string
	Dark      bool
	Accent    string
}{
	{"paper", "Paper", false, "#c27c0e"}, {"night", "Night", true, "#f0b341"}, {"chalk", "Chalkboard", true, "#f5d76e"},
	{"sunrise", "Sunrise", false, "#e0553a"}, {"studio", "Studio", true, "#ff5a4f"}, {"ocean", "Ocean", true, "#5ee0d0"},
	{"forest", "Forest", false, "#2f7d4f"}, {"mono", "Plain", false, "#111111"}, {"candy", "Candy", false, "#c2378a"},
	{"slate", "Slate", true, "#8ab4ff"},
}

func validTheme(k string) bool {
	for _, t := range builtinThemes {
		if t.Key == k {
			return true
		}
	}
	return false
}

func themeAccent(k string) string {
	for _, t := range builtinThemes {
		if t.Key == k {
			return t.Accent
		}
	}
	return "#c27c0e"
}

var (
	aspects     = []string{"16:9", "4:3"}
	transitions = []string{"fade", "slide", "none"}
	styles      = []string{"", "accent", "dark", "light"}
	fonts       = []string{"", "sans", "serif", "rounded", "mono", "slab", "condensed"}
	chartKinds  = []string{"bar", "hbar", "line", "pie", "donut"}
	qrKinds     = []string{"", "share", "live", "none"}
	fits        = []string{"", "cover", "contain"}
)

// slideData is what a slide keeps besides its words, per layout.
type slideData struct {
	Build   bool       `json:"build,omitempty"`   // points appear one at a time
	Fit     string     `json:"fit,omitempty"`     // cover | contain (pictures)
	Lang    string     `json:"lang,omitempty"`    // code
	Chart   *chartData `json:"chart,omitempty"`   // chart
	Rows    [][]string `json:"rows,omitempty"`    // table (first row = headings)
	Options []string   `json:"options,omitempty"` // poll
	Correct int        `json:"correct,omitempty"` // poll: the right answer (1-based, 0 = none)
	QR      string     `json:"qr,omitempty"`      // end: share | live | none
	Left    string     `json:"left,omitempty"`    // two: the heading of each column
	Right   string     `json:"right,omitempty"`
	From    string     `json:"from,omitempty"` // where the numbers came from (a table in Pocket Tables)
}

type chartData struct {
	Kind   string    `json:"kind"`
	Labels []string  `json:"labels"`
	Values []float64 `json:"values"`
	Unit   string    `json:"unit,omitempty"`
}

// cleanData keeps only what a slide may hold, within reason.
func cleanData(raw json.RawMessage) json.RawMessage {
	var d slideData
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil {
		return json.RawMessage(`{}`)
	}
	if !oneOf(d.Fit, fits) {
		d.Fit = ""
	}
	d.Lang = clip(strings.TrimSpace(d.Lang), 20)
	if !oneOf(d.QR, qrKinds) {
		d.QR = ""
	}
	d.Left, d.Right, d.From = cleanText(d.Left, 80), cleanText(d.Right, 80), cleanText(d.From, 120)
	if c := d.Chart; c != nil {
		if !oneOf(c.Kind, chartKinds) {
			c.Kind = "bar"
		}
		n := len(c.Labels)
		if len(c.Values) < n {
			n = len(c.Values)
		}
		if n > 24 {
			n = 24
		}
		c.Labels, c.Values = c.Labels[:n], c.Values[:n]
		for i := range c.Labels {
			c.Labels[i] = cleanText(c.Labels[i], 40)
		}
		c.Unit = cleanText(c.Unit, 12)
	}
	if len(d.Rows) > 16 {
		d.Rows = d.Rows[:16]
	}
	for i, r := range d.Rows {
		if len(r) > 8 {
			r = r[:8]
		}
		for j := range r {
			r[j] = cleanText(r[j], 120)
		}
		d.Rows[i] = r
	}
	opts := []string{}
	for _, o := range d.Options {
		if o = cleanText(o, 80); o != "" && len(opts) < 8 {
			opts = append(opts, o)
		}
	}
	d.Options = opts
	if len(opts) == 0 {
		d.Options = nil
	}
	d.Correct = clampInt(d.Correct, 0, len(opts))
	b, _ := json.Marshal(d)
	return b
}

func dataOf(sl *store.Slide) slideData {
	var d slideData
	_ = json.Unmarshal(sl.Data, &d)
	return d
}

// points counts the top-level points of a body (for step-by-step slides).
func points(body string) int {
	n := 0
	for _, l := range strings.Split(body, "\n") {
		if t := strings.TrimLeft(l, " \t"); len(t) != len(l) {
			continue // indented: part of the point above
		}
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || numbered(t) {
			n++
		}
	}
	return n
}

func numbered(t string) bool {
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	return i > 0 && i < len(t)-1 && t[i] == '.' && t[i+1] == ' '
}

// steps is how many presses a slide takes before the next one comes.
func steps(sl *store.Slide) int {
	d := dataOf(sl)
	if sl.Layout == "poll" && d.Correct > 0 {
		return 1 // one press shows the right answer
	}
	if !d.Build {
		return 0
	}
	switch sl.Layout {
	case "bullets", "image_left", "image_right", "two":
		return points(sl.Body) + points(sl.Body2)
	}
	return 0
}

// ---- deck templates: a few slides to start from ----

type tmplSlide struct {
	layout, title, subtitle, body, body2, notes, style string
	data                                               string
}

type deckTemplate struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	About  string `json:"about"`
	Theme  string `json:"theme"`
	Minute int    `json:"minutes"`
	slides []tmplSlide
}

var deckTemplates = []deckTemplate{
	{Key: "blank", Name: "Empty deck", About: "Just a title slide", Theme: "", slides: []tmplSlide{
		{layout: "title", title: "{title}", subtitle: "{name}"},
	}},
	{Key: "talk", Name: "A talk", About: "Title, agenda, three parts and questions", Theme: "night", Minute: 20, slides: []tmplSlide{
		{layout: "title", title: "{title}", subtitle: "{name}", notes: "Say hello, who you are and why you are here. Ask how many have heard about this before."},
		{layout: "agenda", title: "Today"},
		{layout: "section", title: "Why it matters", subtitle: "Part one"},
		{layout: "bullets", title: "The problem", body: "- What goes wrong today\n- Who it hurts\n- What it costs", data: `{"build":true}`, notes: "One story from real life works better than numbers here."},
		{layout: "number", title: "3 in 4", subtitle: "people say this every week", body: "Where the number comes from, in one line"},
		{layout: "section", title: "What we can do", subtitle: "Part two"},
		{layout: "two", title: "Before and after", data: `{"left":"Before","right":"After"}`, body: "- Slow\n- By hand\n- Easy to forget", body2: "- Quick\n- Done for you\n- Reminds you"},
		{layout: "quote", body: "The best way to get started is to quit talking and begin doing.", subtitle: "Walt Disney"},
		{layout: "section", title: "How to start", subtitle: "Part three"},
		{layout: "bullets", title: "Three things to try this week", body: "1. The first small step\n2. The second\n3. The third", data: `{"build":true}`},
		{layout: "poll", title: "Which one will you try first?", data: `{"options":["The first","The second","The third"]}`},
		{layout: "end", title: "Thank you!", subtitle: "Questions?", data: `{"qr":"share"}`},
	}},
	{Key: "lesson", Name: "A lesson", About: "Goals, the lesson in parts, a quiz and a recap", Theme: "chalk", Minute: 45, slides: []tmplSlide{
		{layout: "title", title: "{title}", subtitle: "Lesson"},
		{layout: "bullets", title: "Today you will learn", body: "- The first idea\n- The second idea\n- How to use them together"},
		{layout: "section", title: "The first idea"},
		{layout: "image_right", title: "Look at this", body: "- What you see\n- What it means"},
		{layout: "section", title: "The second idea"},
		{layout: "two", title: "Compare", data: `{"left":"This","right":"That"}`, body: "- One\n- Two", body2: "- One\n- Two"},
		{layout: "poll", title: "Quick quiz: which is right?", data: `{"options":["Answer A","Answer B","Answer C"],"correct":2}`, notes: "Wait until most have answered, then show the right one."},
		{layout: "bullets", title: "Remember", body: "- The first idea in one line\n- The second idea in one line"},
		{layout: "end", title: "Homework", subtitle: "Due next week", data: `{"qr":"share"}`},
	}},
	{Key: "update", Name: "Project update", About: "Where we are, what is done, what is next, risks", Theme: "paper", Minute: 10, slides: []tmplSlide{
		{layout: "title", title: "{title}", subtitle: "Update · {date}"},
		{layout: "number", title: "70%", subtitle: "done", body: "On track for the end of the month"},
		{layout: "bullets", title: "Done since last time", body: "- The first thing\n- The second thing\n- The third thing"},
		{layout: "bullets", title: "Next", body: "- What comes next\n- And after that"},
		{layout: "chart", title: "Progress", data: `{"chart":{"kind":"bar","labels":["Week 1","Week 2","Week 3","Week 4"],"values":[12,30,52,70],"unit":"%"}}`},
		{layout: "two", title: "Risks and help needed", data: `{"left":"Risks","right":"Help needed"}`, body: "- What could go wrong", body2: "- Who can help with what"},
		{layout: "end", title: "Questions?", subtitle: ""},
	}},
	{Key: "pitch", Name: "A pitch", About: "Problem, solution, how it works, the market and the ask", Theme: "studio", Minute: 10, slides: []tmplSlide{
		{layout: "title", title: "{title}", subtitle: "The idea in one line"},
		{layout: "bullets", title: "The problem", body: "- Who has it\n- How they deal with it today\n- Why that is not good enough"},
		{layout: "bullets", title: "Our answer", body: "- What it is\n- What it does better"},
		{layout: "image_right", title: "How it works", body: "1. They do this\n2. We do that\n3. They get this"},
		{layout: "number", title: "€2.4 m", subtitle: "the market we start with", body: "How we worked it out"},
		{layout: "chart", title: "The plan", data: `{"chart":{"kind":"line","labels":["2026","2027","2028"],"values":[10,120,900],"unit":"customers"}}`},
		{layout: "two", title: "Who we are", data: `{"left":"Us","right":"Why us"}`, body: "- Names and roles", body2: "- What we did before"},
		{layout: "end", title: "What we ask for", subtitle: "and what you get", data: `{"qr":"none"}`},
	}},
	{Key: "photos", Name: "Photo show", About: "A title and pictures, for a birthday or a trip", Theme: "mono", Minute: 5, slides: []tmplSlide{
		{layout: "title", title: "{title}", subtitle: "{date}"},
		{layout: "image", title: "", subtitle: "Add a picture", data: `{"fit":"cover"}`},
		{layout: "image", title: "", subtitle: "Add a picture", data: `{"fit":"cover"}`},
		{layout: "image", title: "", subtitle: "Add a picture", data: `{"fit":"cover"}`},
		{layout: "end", title: "Thank you", subtitle: "", data: `{"qr":"none"}`},
	}},
	{Key: "workshop", Name: "A workshop", About: "Agenda, exercises, a break and what to take home", Theme: "sunrise", Minute: 90, slides: []tmplSlide{
		{layout: "title", title: "{title}", subtitle: "Workshop"},
		{layout: "agenda", title: "The plan for today"},
		{layout: "section", title: "Warm up"},
		{layout: "poll", title: "How much do you know about this already?", data: `{"options":["Nothing yet","A little","Quite a lot"]}`},
		{layout: "section", title: "Exercise one"},
		{layout: "bullets", title: "Your task", body: "1. Make groups of three\n2. Pick one idea\n3. Ten minutes to sketch it", notes: "Walk around. Give a two-minute warning."},
		{layout: "section", title: "Break", subtitle: "Back in 15 minutes", style: "accent"},
		{layout: "section", title: "Exercise two"},
		{layout: "bullets", title: "What to take home", body: "- The one idea to remember\n- Where to read more"},
		{layout: "end", title: "Thank you!", subtitle: "Tell us how it was", data: `{"qr":"live"}`},
	}},
}

func templateByKey(k string) (deckTemplate, bool) {
	for _, t := range deckTemplates {
		if t.Key == k {
			return t, true
		}
	}
	return deckTemplate{}, false
}
