package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// seedDemo fills the owner's account with a few decks, talks and rehearsals
// (DEMO_DATA=true): for trying it out, CI and the screenshots.
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if n, _ := s.store.Decks(ctx, uid, false); len(n) > 0 {
		return nil
	}
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return err
	}
	rnd := rand.New(rand.NewSource(42))
	now := time.Now()
	mk := func(title, theme string, minutes int, talkIn int, place string, slides []tmplSlide) *store.Deck {
		d := s.newDeck(ctx, uid, title)
		d.Theme, d.TargetMinutes, d.TalkPlace = theme, minutes, place
		if talkIn != 0 {
			d.TalkAt = now.AddDate(0, 0, talkIn).Format("2006-01-02") + "T10:30"
			d.TalkTitle = title
		}
		_ = s.store.CreateDeck(ctx, d)
		for _, t := range slides {
			sl := &store.Slide{DeckID: d.ID, UserID: uid, Position: -1, Layout: t.layout, Title: t.title, Subtitle: t.subtitle, Body: t.body, Body2: t.body2,
				Notes: t.notes, Style: t.style, Data: cleanData(json.RawMessage(firstNonEmpty(t.data, "{}")))}
			_ = s.store.CreateSlide(ctx, sl)
		}
		return d
	}
	homelab := mk("Running a home server, the calm way", "night", 20, 9, "Tech meetup, Sarajevo", []tmplSlide{
		{layout: "title", title: "Running a home server, the calm way", subtitle: "Aleksandar · Tech meetup", notes: "Hello! Quick show of hands: who has a computer at home that never sleeps?"},
		{layout: "agenda", title: "Tonight"},
		{layout: "section", title: "Why bother", subtitle: "Part one"},
		{layout: "bullets", title: "What we gave away", body: "- Our photos live on someone else's computer\n- Prices go up every year\n- Apps close, and take our data with them", data: `{"build":true}`, notes: "Tell the story of the photo app that shut down in 2024 and the weekend spent getting the pictures out."},
		{layout: "number", title: "€38", subtitle: "a month in subscriptions we stopped paying", body: "Photos, notes, passwords, files and a VPN"},
		{layout: "section", title: "How it works", subtitle: "Part two"},
		{layout: "two", title: "Before and after", data: `{"left":"Cloud","right":"At home"}`, body: "- Pay every month\n- Their rules\n- Fast to start", body2: "- Pay once for a box\n- Your rules\n- One evening to set up"},
		{layout: "chart", title: "What the box does all day", subtitle: "Share of the processor, a normal week", data: `{"chart":{"kind":"donut","labels":["Photos","Backups","Media","Everything else"],"values":[34,22,28,16],"unit":"%"}}`, notes: "Most of the time it is asleep. Photos indexing is the hungry one."},
		{layout: "table", title: "The apps on it", data: `{"rows":[["App","Replaces","Since"],["Photos","Google Photos","2023"],["Notes","Evernote","2024"],["Money","A spreadsheet","2025"],["Slides","PowerPoint","2026"]]}`},
		{layout: "code", title: "Starting one is one line", body: "docker compose up -d\ncurl -s localhost:8269/healthz", data: `{"lang":"bash"}`},
		{layout: "quote", body: "The cloud is just someone else's computer.", subtitle: "A sticker on many laptops"},
		{layout: "section", title: "Start this weekend", subtitle: "Part three"},
		{layout: "bullets", title: "Three steps", body: "1. An old laptop or a small box\n2. One app you use every day\n3. A backup you have tested once", data: `{"build":true}`, notes: "The backup is the step everyone skips. Do not skip it."},
		{layout: "poll", title: "What would you move home first?", data: `{"options":["Photos","Files","Passwords","Notes"]}`},
		{layout: "end", title: "Thank you!", subtitle: "Questions? · slides on the code", data: `{"qr":"share"}`},
	})
	lesson := mk("The water cycle", "chalk", 40, 0, "", []tmplSlide{
		{layout: "title", title: "The water cycle", subtitle: "Science · Year 5"},
		{layout: "bullets", title: "Today you will learn", body: "- Where rain comes from\n- What evaporation means\n- Why clouds are white"},
		{layout: "section", title: "Up into the sky"},
		{layout: "bullets", title: "Evaporation", body: "- The sun warms the sea\n- Water turns into vapour\n- The vapour rises", data: `{"build":true}`},
		{layout: "section", title: "Down again"},
		{layout: "two", title: "Rain and snow", data: `{"left":"Warm air","right":"Cold air"}`, body: "- Drops join\n- Rain falls", body2: "- Drops freeze\n- Snow falls"},
		{layout: "poll", title: "Quick quiz: what makes water rise?", data: `{"options":["The wind","The sun's warmth","The moon"],"correct":2}`, notes: "Wait for most to answer, then press next to show the right one."},
		{layout: "end", title: "Homework", subtitle: "Draw the cycle with four arrows", data: `{"qr":"none"}`},
	})
	update := mk("Kitchen renovation: where we are", "paper", 10, 2, "Family call", []tmplSlide{
		{layout: "title", title: "Kitchen renovation", subtitle: "Where we are · November"},
		{layout: "number", title: "70%", subtitle: "done", body: "Tiles this week, worktop next week"},
		{layout: "chart", title: "Spent so far", subtitle: "Of €9,000 planned", data: `{"chart":{"kind":"bar","labels":["Cabinets","Tiles","Plumber","Electrician","Worktop"],"values":[3200,850,1100,700,0],"unit":"€"}}`},
		{layout: "bullets", title: "Still to do", body: "- Worktop (ordered)\n- Hob and oven\n- Paint the ceiling"},
		{layout: "end", title: "Questions?", subtitle: ""},
	})
	_ = mk("Book club: autumn picks", "sunrise", 15, 0, "", []tmplSlide{
		{layout: "title", title: "Autumn picks", subtitle: "Book club"},
		{layout: "poll", title: "Which one first?", data: `{"options":["The long one","The funny one","The short one"]}`},
		{layout: "end", title: "See you on Thursday", subtitle: ""},
	})
	// rehearsals and a past talk, with timings, questions and feedback
	for i, deck := range []*store.Deck{homelab, homelab, homelab, lesson, update} {
		slides, _ := s.store.Slides(ctx, deck.ID)
		vis := visible(slides)
		started := now.AddDate(0, 0, -12+i*3).Add(time.Duration(18+i) * time.Hour)
		h := &store.Show{UserID: uid, DeckID: deck.ID, Kind: store.KindRehearsal, StartedAt: started.UTC().Format(time.RFC3339Nano), Timings: map[string]int64{}}
		if deck == lesson {
			h.Kind = store.KindTalk
		}
		var total int64
		for _, sl := range vis {
			ms := int64(40000 + rnd.Intn(90000))
			if sl.Layout == "section" || sl.Layout == "title" {
				ms = int64(12000 + rnd.Intn(15000))
			}
			if deck == homelab {
				ms = ms * int64(14-i*2) / 10
			}
			h.Timings[fmt.Sprint(sl.ID)] = ms
			total += ms
		}
		h.TotalMs, h.Pos = total, len(vis)-1
		end := started.Add(time.Duration(total) * time.Millisecond).UTC().Format(time.RFC3339Nano)
		h.EndedAt = &end
		if h.Kind == store.KindTalk {
			h.Viewers, h.Reactions = 23, 41
		}
		_ = s.store.StartShow(ctx, h)
		if h.Kind == store.KindTalk {
			for j, q := range []string{"Is evaporation the same as boiling?", "Why are some clouds grey?", "Does the sea ever run out?"} {
				_ = s.store.AddQuestion(ctx, &store.Question{ShowID: h.ID, UserID: uid, Name: []string{"Mia", "", "Luka"}[j], Body: q, Votes: 5 - j*2, Answered: j == 0})
			}
			for _, sl := range vis {
				if sl.Layout == "poll" {
					for v := 0; v < 21; v++ {
						c := 1
						if v%5 == 0 {
							c = 0
						} else if v%7 == 0 {
							c = 2
						}
						_ = s.store.Vote(ctx, uid, h.ID, sl.ID, c, fmt.Sprintf("demo-voter-%02d", v))
					}
				}
			}
			for j, f := range []struct {
				r int
				c string
			}{{5, "The quiz was the best part!"}, {4, "Loved the drawings"}, {5, ""}, {4, "A bit fast at the end"}} {
				_ = s.store.AddFeedback(ctx, &store.Feedback{ShowID: h.ID, UserID: uid, Rating: f.r, Comment: f.c}, fmt.Sprintf("demo-%d", j))
			}
		}
	}
	// a theme and a layout of my own
	t := &store.Theme{UserID: uid, Name: "Meetup", Base: "night", BG: "#0f1a2e", BG2: "#1d2f52", FG: "#f4f1ea", Muted: "#9fb0cc", Accent: "#ffb347", HeadFont: "rounded", BodyFont: "sans"}
	_ = s.store.SaveTheme(ctx, t)
	_ = s.store.CreateLayout(ctx, &store.Layout{UserID: uid, Name: "Story with a lesson", Layout: "two", Title: "What happened", Body: "- The moment\n- What went wrong", Body2: "- What we learned\n- What we do now",
		Data: json.RawMessage(`{"left":"The story","right":"The lesson"}`), Notes: "Tell it in under a minute."})
	homelab.ThemeID = &t.ID
	homelab.Shared = true
	_ = s.store.UpdateDeck(ctx, homelab)
	s.log.Info("demo data ready", "user", u.Email)
	return nil
}
