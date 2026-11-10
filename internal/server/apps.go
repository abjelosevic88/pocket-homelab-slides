package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

// The other Pocket apps. None of it is needed: when an app is not there,
// the page greys the button and says what to do.

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

func plainErr(a pocketApp, err error) string {
	if isNotConnected(err) {
		return err.Error()
	}
	return a.name + ": " + err.Error()
}

func (s *Server) appFail(w http.ResponseWriter, a pocketApp, err error) {
	if isNotConnected(err) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if ue, ok := err.(*userErr); ok {
		writeErr(w, http.StatusBadRequest, ue.msg)
		return
	}
	writeErr(w, http.StatusBadGateway, plainErr(a, err))
}

// handleApps lists how the person reaches each app, in words they can act on.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

func (s *Server) sib(w http.ResponseWriter, r *http.Request, a pocketApp, scopes ...string) (*siblingConn, bool) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, a, scopes...)
	if err != nil {
		s.appFail(w, a, err)
		return nil, false
	}
	return sib, true
}

// deckBody decodes the request and finds the caller's deck it is about.
func (s *Server) deckBody(w http.ResponseWriter, r *http.Request, v any, id func() int64) (*store.Deck, bool) {
	if v != nil {
		if err := decode(r, v); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return nil, false
		}
	}
	return s.deckOf(w, r, id())
}

func (s *Server) actorName(u *store.User) string {
	if u == nil {
		return "Someone"
	}
	if n := strings.TrimSpace(u.Name); n != "" {
		return n
	}
	if i := strings.Index(u.Email, "@"); i > 0 {
		return u.Email[:i]
	}
	return "Someone"
}

// ---- Pocket Notes ----

func (s *Server) handleNotesSearch(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appNotes, scopeNotesRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/search", url.Values{"q": {r.URL.Query().Get("q")}, "limit": {"30"}})
	if err != nil {
		s.appFail(w, appNotes, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "hits", "notes") {
		n := m
		if inner, ok := m["note"].(map[string]any); ok {
			n = inner
		}
		out = append(out, map[string]any{"id": idOf(n), "title": firstNonEmpty(str(n, "title"), "Untitled"), "folder": str(n, "category"), "snippet": clip(firstNonEmpty(str(m, "snippet"), str(n, "excerpt")), 200)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": out})
}

// handleDeckFromNote makes a deck from a note: each heading (or --- part) a slide.
func (s *Server) handleDeckFromNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		NoteID string `json:"note_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, err := strconv.ParseInt(in.NoteID, 10, 64); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, ok := s.sib(w, r, appNotes, scopeNotesRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/notes/"+in.NoteID, nil)
	if err != nil {
		s.appFail(w, appNotes, err)
		return
	}
	m, _ := v.(map[string]any)
	if inner, ok := m["note"].(map[string]any); ok {
		m = inner
	}
	body := strings.TrimSpace(str(m, "content"))
	if body == "" {
		writeErr(w, http.StatusBadRequest, "that note is empty")
		return
	}
	title, drafts := parseMarkdown(body)
	title = firstNonEmpty(str(m, "title"), title)
	if len(drafts) > 0 && drafts[0].Layout != "title" {
		drafts = append([]draftSlide{{Layout: "title", Title: title, Subtitle: s.actorName(userFrom(ctx))}}, drafts...)
	}
	d, err := s.makeDeck(ctx, userFrom(ctx), title, drafts, "", "")
	if err != nil {
		s.fail(w, err, "deck")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deck": d, "text": fmt.Sprintf("A deck of %d %s from the note", d.Slides, plural(d.Slides, "slide", "slides"))})
}

// handleToNotes keeps a deck's outline with its speaker notes as a note.
func (s *Server) handleToNotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID int64 `json:"deck_id"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appNotes, scopeNotesWrite)
	if !ok {
		return
	}
	folder := firstNonEmpty(s.settingsOf(ctx, d.UserID).NotesFolder, "Slides")
	md := s.deckMarkdown(ctx, d, true) + "\n\n---\n[Open the slides](" + s.deckURL(d) + ")\n"
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(d.Title, 200), "content": md, "category": folder})
	if err != nil {
		s.appFail(w, appNotes, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": linkTo(sib, "/notes/"+idOf(res)), "text": "Kept in Pocket Notes"})
}

// ---- Pocket Photos ----

func (s *Server) handlePhotosList(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appPhotos, scopePhotosRead)
	if !ok {
		return
	}
	body := map[string]any{"filter": map[string]any{"trashedAt": map[string]any{"eq": nil}, "type": map[string]any{"eq": "IMAGE"}, "visibility": map[string]any{"eq": "timeline"}},
		"size": 60, "withExif": false, "orderBy": map[string]any{"field": "fileCreatedAt", "direction": "desc"}}
	path := "/api/v1/immich/search/metadata"
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		body["query"] = q
		path = "/api/v1/immich/search/smart"
	}
	if c := r.URL.Query().Get("cursor"); c != "" {
		body["cursor"] = c
	}
	res, err := postJSONTo(r.Context(), sib, path, body)
	if err != nil {
		s.appFail(w, appPhotos, err)
		return
	}
	assets := obj(res, "assets")
	out := []map[string]any{}
	for _, m := range rows(assets["items"]) {
		id := str(m, "id")
		if !reUUID.MatchString(id) {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": str(m, "originalFileName"), "date": day10(m["fileCreatedAt"]), "thumb_url": "/api/v1/apps/photos/" + id + "/image"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"photos": out, "cursor": assets["nextCursor"]})
}

func (s *Server) handlePhotoImage(w http.ResponseWriter, r *http.Request) {
	id := chiParam(r, "uuid")
	if !reUUID.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, ok := s.sib(w, r, appPhotos, scopePhotosRead)
	if !ok {
		return
	}
	b, ctype, err := sib.client.Bytes(r.Context(), "/api/v1/immich/assets/"+id+"/thumbnail", url.Values{"size": {"thumbnail"}}, 16<<20)
	if err != nil {
		s.appFail(w, appPhotos, err)
		return
	}
	w.Header().Set("Content-Type", firstNonEmpty(ctype, http.DetectContentType(b)))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
}

// handlePhotoCopy puts copies of Pocket Photos pictures in a deck: on one
// slide (slide_id), or a new full-picture slide each.
func (s *Server) handlePhotoCopy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID  int64    `json:"deck_id"`
		SlideID int64    `json:"slide_id"`
		After   int64    `json:"after"`
		IDs     []string `json:"ids"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 40 {
		writeErr(w, http.StatusBadRequest, "pick 1 to 40 photos")
		return
	}
	var target *store.Slide
	if in.SlideID > 0 {
		sl, err := s.store.GetSlide(ctx, d.UserID, in.SlideID)
		if err != nil || sl.DeckID != d.ID {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		target = sl
		in.IDs = in.IDs[:1]
	}
	sib, ok := s.sib(w, r, appPhotos, scopePhotosRead)
	if !ok {
		return
	}
	pos := -1
	if in.After > 0 {
		if a, err := s.store.GetSlide(ctx, d.UserID, in.After); err == nil && a.DeckID == d.ID {
			pos = a.Position + 1
		}
	}
	added := 0
	for _, id := range in.IDs {
		if !reUUID.MatchString(id) {
			continue
		}
		b, ctype, err := sib.client.Bytes(ctx, "/api/v1/immich/assets/"+id+"/thumbnail", url.Values{"size": {"preview"}}, maxUpload)
		if err != nil {
			s.appFail(w, appPhotos, err)
			return
		}
		thumb, _, _ := sib.client.Bytes(ctx, "/api/v1/immich/assets/"+id+"/thumbnail", url.Values{"size": {"thumbnail"}}, 4<<20)
		mime := firstNonEmpty(strings.Split(ctype, ";")[0], http.DetectContentType(b))
		if !allowedMime[mime] {
			mime = "image/jpeg"
		}
		up := &store.Upload{UserID: d.UserID, DeckID: d.ID, Name: "photo.jpg", Mime: mime, Data: b, Thumb: thumb}
		if err := s.store.SaveUpload(ctx, up); err != nil {
			s.fail(w, err, "photo")
			return
		}
		if target != nil {
			target.ImageID = &up.ID
			if err := s.store.UpdateSlide(ctx, target); err != nil {
				s.fail(w, err, "slide")
				return
			}
		} else {
			sl := &store.Slide{DeckID: d.ID, UserID: d.UserID, Position: pos, Layout: "image", ImageID: &up.ID, Data: json.RawMessage(`{"fit":"cover"}`), Source: sourceOf(ctx)}
			if err := s.store.CreateSlide(ctx, sl); err != nil {
				s.fail(w, err, "slide")
				return
			}
			if pos >= 0 {
				pos++
			}
		}
		added++
	}
	s.live.deckChanged(ctx, s, d.ID)
	text := fmt.Sprintf("%d %s added", added, plural(added, "photo slide", "photo slides"))
	if target != nil {
		text = "The photo is on the slide"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "added": added, "text": text})
}

// ---- Pocket Files ----

// handleFilesList lists a folder: folders, PowerPoint and Markdown files and pictures.
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, ok := s.sib(w, r, appFiles, scopeFilesRead)
	if !ok {
		return
	}
	space := firstNonEmpty(r.URL.Query().Get("space"), "home")
	path := firstNonEmpty(r.URL.Query().Get("path"), "/")
	v, err := getJSONFrom(ctx, sib, "/api/v1/fs/list", url.Values{"space": {space}, "path": {path}})
	if err != nil {
		s.appFail(w, appFiles, err)
		return
	}
	want := map[string]string{"pptx": "deck", "potx": "deck", "ppsx": "deck", "md": "deck", "markdown": "deck", "jpg": "picture", "jpeg": "picture", "png": "picture", "webp": "picture", "gif": "picture"}
	out := []map[string]any{}
	for _, e := range rows(v, "entries") {
		name := str(e, "name")
		dir := e["is_dir"] == true
		ext := ""
		if i := strings.LastIndex(name, "."); i >= 0 {
			ext = strings.ToLower(name[i+1:])
		}
		kind := want[ext]
		if dir {
			kind = "folder"
		}
		if kind == "" || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, map[string]any{"name": name, "path": str(e, "path"), "kind": kind, "size": num(e, "size"), "modified": str(e, "modified")})
	}
	spaces := []map[string]any{}
	if sv, err := getJSONFrom(ctx, sib, "/api/v1/spaces", nil); err == nil {
		for _, sp := range rows(sv, "spaces") {
			spaces = append(spaces, map[string]any{"id": str(sp, "id"), "name": str(sp, "name")})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out, "space": space, "path": path, "spaces": spaces})
}

func (s *Server) fileBytes(ctx context.Context, sib *siblingConn, space, path string, max int64) ([]byte, error) {
	b, _, err := sib.client.Bytes(ctx, "/api/v1/fs/raw", url.Values{"space": {firstNonEmpty(space, "home")}, "path": {path}}, max)
	return b, err
}

// handleFromFiles makes a deck from a PowerPoint or Markdown file in Pocket Files.
func (s *Server) handleFromFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Space string `json:"space"`
		Path  string `json:"path"`
	}
	if err := decode(r, &in); err != nil || in.Path == "" {
		writeErr(w, http.StatusBadRequest, "pick a file")
		return
	}
	sib, ok := s.sib(w, r, appFiles, scopeFilesRead)
	if !ok {
		return
	}
	data, err := s.fileBytes(ctx, sib, in.Space, in.Path, 100<<20)
	if err != nil {
		s.appFail(w, appFiles, err)
		return
	}
	title, drafts, err := s.draftsFromFile(in.Path, data)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	d, err := s.makeDeck(ctx, userFrom(ctx), title, drafts, "", "")
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deck": d, "text": fmt.Sprintf("%d %s brought in from your files", d.Slides, plural(d.Slides, "slide", "slides"))})
}

// handleFilePicture puts a picture from Pocket Files on a slide.
func (s *Server) handleFilePicture(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID  int64  `json:"deck_id"`
		SlideID int64  `json:"slide_id"`
		Space   string `json:"space"`
		Path    string `json:"path"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	sl, err := s.store.GetSlide(ctx, d.UserID, in.SlideID)
	if err != nil || sl.DeckID != d.ID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, ok := s.sib(w, r, appFiles, scopeFilesRead)
	if !ok {
		return
	}
	data, err := s.fileBytes(ctx, sib, in.Space, in.Path, maxUpload)
	if err != nil {
		s.appFail(w, appFiles, err)
		return
	}
	mime := strings.Split(http.DetectContentType(data), ";")[0]
	if !allowedMime[mime] {
		writeErr(w, http.StatusBadRequest, "pictures only (JPEG, PNG, WebP or GIF)")
		return
	}
	up := &store.Upload{UserID: d.UserID, DeckID: d.ID, Name: safeName(in.Path), Mime: mime, Data: data}
	if err := s.store.SaveUpload(ctx, up); err != nil {
		s.fail(w, err, "picture")
		return
	}
	sl.ImageID = &up.ID
	if err := s.store.UpdateSlide(ctx, sl); err != nil {
		s.fail(w, err, "slide")
		return
	}
	s.live.deckChanged(ctx, s, d.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slide": sl, "text": "The picture is on the slide"})
}

// handleToFiles saves a deck in Pocket Files: the web page the browser made
// (one file that opens anywhere), or the outline as Markdown.
func (s *Server) handleToFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID int64  `json:"deck_id"`
		Kind   string `json:"kind"` // html | md
		HTML   string `json:"html"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 200<<20)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	d, ok := s.deckOf(w, r, in.DeckID)
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appFiles, scopeFilesWrite)
	if !ok {
		return
	}
	var data []byte
	name, mime := fileSafe(d.Title), ""
	switch in.Kind {
	case "html":
		if !strings.HasPrefix(strings.TrimSpace(in.HTML), "<!doctype html>") {
			writeErr(w, http.StatusBadRequest, "the page is missing")
			return
		}
		data, name, mime = []byte(in.HTML), name+".html", "text/html"
	default:
		data, name, mime = []byte(s.deckMarkdown(ctx, d, true)), name+".md", "text/markdown"
	}
	dir := strings.Trim(firstNonEmpty(s.settingsOf(ctx, d.UserID).FilesFolder, "Slides"), "/")
	var res map[string]any
	q := url.Values{"space": {"home"}, "path": {"/"}, "name": {dir + "/" + name}, "conflict": {"rename"}, "mtime": {strconv.FormatInt(time.Now().UnixMilli(), 10)}}
	if err := sib.client.PutRaw(ctx, "/api/v1/fs/upload", q, data, mime, &res); err != nil {
		s.appFail(w, appFiles, err)
		return
	}
	p := firstNonEmpty(str(res, "path"), "/"+dir+"/"+name)
	folder := p
	if i := strings.LastIndex(p, "/"); i > 0 {
		folder = p[:i]
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Saved in Pocket Files as " + p, "url": linkTo(sib, "/f/home"+folder)})
}

// ---- Pocket Calendar ----

// handleCalendarList shows what is on in the next weeks, to pick the talk from.
func (s *Server) handleCalendarList(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appCalendar, scopeCalendarRead)
	if !ok {
		return
	}
	from := time.Now().Format("2006-01-02")
	to := time.Now().AddDate(0, 0, clampInt(firstPos(queryInt(r, "days"), 90), 1, 365)).Format("2006-01-02")
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/events", url.Values{"from": {from}, "to": {to}})
	if err != nil {
		s.appFail(w, appCalendar, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "events") {
		if m["all_day"] == true || len(out) >= 80 {
			continue
		}
		start := str(m, "start")
		at := ""
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			at = t.Local().Format("2006-01-02T15:04")
		}
		out = append(out, map[string]any{"id": firstNonEmpty(str(m, "id"), idOf(m)), "title": str(m, "title"), "at": at, "place": str(m, "location"), "end": str(m, "end")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}

// toCalendar puts a deck's talk in the person's calendar with the link to its slides.
func (s *Server) toCalendar(ctx context.Context, d *store.Deck) (string, error) {
	sib, err := s.clients(ctx, d.UserID).conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return "", err
	}
	if d.TalkAt == "" {
		return "", &userErr{"set the day of the talk first"}
	}
	desc := "Slides: " + s.deckURL(d)
	if d.About != "" {
		desc = d.About + "\n\n" + desc
	}
	body := map[string]any{"title": clip(firstNonEmpty(d.TalkTitle, d.Title), 200), "description": desc, "url": s.deckURL(d), "location": d.TalkPlace, "categories": []string{"Talks"}}
	if len(d.TalkAt) == 10 {
		body["all_day"], body["start"], body["end"] = true, d.TalkAt, d.TalkAt
	} else {
		start, err := time.ParseInLocation("2006-01-02T15:04", d.TalkAt, time.Local)
		if err != nil {
			return "", &userErr{"the talk's time is YYYY-MM-DDTHH:MM"}
		}
		mins := firstPos(d.TargetMinutes, 30)
		body["start"], body["end"] = start.Format(time.RFC3339), start.Add(time.Duration(mins)*time.Minute).Format(time.RFC3339)
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		return "", err
	}
	link := linkTo(sib, "/?event="+url.QueryEscape(firstNonEmpty(str(res, "event_id"), idOf(res))))
	d.CalendarURL = link
	_ = s.store.UpdateDeck(ctx, d)
	return link, nil
}

func (s *Server) handleToCalendar(w http.ResponseWriter, r *http.Request) {
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	link, err := s.toCalendar(r.Context(), d)
	if err != nil {
		s.appFail(w, appCalendar, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": link, "text": "The talk is in Pocket Calendar"})
}

// afterTalkSet runs when a talk's day is set: the calendar and the
// rehearse tasks, as the person chose in Settings (best effort, never in the way).
func (s *Server) afterTalkSet(ctx context.Context, u *store.User, d *store.Deck) map[string]any {
	st := s.settingsOf(ctx, d.UserID)
	out := map[string]any{}
	if st.CalendarOnTalk && d.CalendarURL == "" {
		if link, err := s.toCalendar(ctx, d); err == nil {
			out["calendar"] = link
		}
	}
	if st.RehearseTasks {
		if n, link, err := s.rehearseTasks(ctx, d); err == nil && n > 0 {
			out["tasks"], out["tasks_url"] = n, link
		}
	}
	return out
}

// ---- Pocket Tasks ----

// rehearseTasks puts getting ready for a talk in the person's task list.
func (s *Server) rehearseTasks(ctx context.Context, d *store.Deck) (int, string, error) {
	sib, err := s.clients(ctx, d.UserID).conn(ctx, appTasks, scopeTasksWrite)
	if err != nil {
		return 0, "", err
	}
	if len(d.TalkAt) < 10 {
		return 0, "", &userErr{"set the day of the talk first"}
	}
	day, err := time.ParseInLocation("2006-01-02", d.TalkAt[:10], time.Local)
	if err != nil {
		return 0, "", &userErr{"the talk's day is YYYY-MM-DD"}
	}
	name := firstNonEmpty(d.TalkTitle, d.Title)
	today := time.Now().Format("2006-01-02")
	todo := []struct{ title, due string }{
		{"Rehearse “" + name + "” once, start to end", day.AddDate(0, 0, -3).Format("2006-01-02")},
		{"Rehearse “" + name + "” again, with a timer", day.AddDate(0, 0, -1).Format("2006-01-02")},
		{"Send the slides of “" + name + "” to the people who came", day.AddDate(0, 0, 1).Format("2006-01-02")},
	}
	n, link := 0, ""
	for _, t := range todo {
		due := t.due
		if due < today {
			due = today
		}
		task, err := sib.client.CreateTask(ctx, t.title, "Slides: "+s.deckURL(d), due, s.deckURL(d))
		if err != nil {
			if n == 0 {
				return 0, "", err
			}
			break
		}
		n++
		link = linkTo(sib, "/tasks/"+strconv.FormatInt(task.ID, 10))
	}
	return n, link, nil
}

func (s *Server) handleToTasks(w http.ResponseWriter, r *http.Request) {
	d, ok := s.deckOf(w, r, idParam(r, "id"))
	if !ok {
		return
	}
	n, link, err := s.rehearseTasks(r.Context(), d)
	if err != nil {
		s.appFail(w, appTasks, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": link, "text": fmt.Sprintf("%d %s added to Pocket Tasks", n, plural(n, "task", "tasks"))})
}

// ---- Pocket Chat ----

func (s *Server) handleChats(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appChat, scopeChatRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/conversations", nil)
	if err != nil {
		s.appFail(w, appChat, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "conversations") {
		if m["archived"] == true {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": firstNonEmpty(str(m, "name"), str(m, "title")), "kind": str(m, "kind"), "emoji": str(m, "emoji")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// handleShareChat sends a deck's share link or its live audience link to a chat.
func (s *Server) handleShareChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID int64  `json:"deck_id"`
		To     string `json:"to"`
		What   string `json:"what"` // share | live
		Text   string `json:"text"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appChat, scopeChatWrite)
	if !ok {
		return
	}
	link, sub, text := s.link("/s/"+d.ShareKey), fmt.Sprintf("%d %s", d.Slides, plural(d.Slides, "slide", "slides")), "Here are the slides."
	if in.What == "live" {
		link, sub, text = s.link("/w/"+d.LiveKey), "Follow along · code "+strings.ToUpper(d.LiveKey), "Follow the talk on your phone: the slides, the polls and questions."
	} else if !d.Shared {
		d.Shared = true
		_ = s.store.UpdateDeck(ctx, d)
	}
	card := map[string]any{"title": d.Title, "subtitle": sub, "lines": []string{clip(firstNonEmpty(d.About, d.TalkTitle), 160)}, "icon": "📽️", "url": link, "app": "slides", "kind": "card"}
	st := s.settingsOf(ctx, d.UserID)
	body := map[string]any{"to": firstNonEmpty(strings.TrimSpace(in.To), st.ChatTo, "me"), "text": clip(firstNonEmpty(strings.TrimSpace(in.Text), text), 2000), "card": card,
		"client_id": fmt.Sprintf("slides-%d-%d-%s-%d", d.UserID, d.ID, in.What, time.Now().Unix()/30)}
	res, err := postJSONTo(ctx, sib, "/api/v1/send", body)
	if err != nil {
		s.appFail(w, appChat, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": linkTo(sib, "/c/"+str(res, "conversation_id")), "text": "Sent to Pocket Chat"})
}

// ---- Pocket Contacts and Email ----

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appContacts, scopeContactsRead)
	if !ok {
		return
	}
	q := url.Values{"limit": {"40"}}
	if t := strings.TrimSpace(r.URL.Query().Get("q")); t != "" {
		q.Set("q", t)
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/contacts", q)
	if err != nil {
		s.appFail(w, appContacts, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "contacts") {
		emails := []string{}
		if arr, ok := m["emails"].([]any); ok {
			for _, e := range arr {
				switch x := e.(type) {
				case string:
					emails = append(emails, x)
				case map[string]any:
					if v := str(x, "value", "email", "addr"); v != "" {
						emails = append(emails, v)
					}
				}
			}
		}
		if len(emails) == 0 {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": firstNonEmpty(str(m, "fn"), str(m, "name"), joinNonEmpty(" ", str(m, "given"), str(m, "family"))), "emails": emails})
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": out})
}

// handleEmail sends the slides (the share link, switched on) from the person's own mail account.
func (s *Server) handleEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID int64  `json:"deck_id"`
		Text   string `json:"text"`
		To     []struct {
			Name string `json:"name"`
			Addr string `json:"addr"`
		} `json:"to"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	to := []map[string]string{}
	for _, t := range in.To {
		if ad := strings.TrimSpace(t.Addr); strings.Contains(ad, "@") && len(to) < 100 {
			to = append(to, map[string]string{"name": strings.TrimSpace(t.Name), "addr": ad})
		}
	}
	if len(to) == 0 {
		writeErr(w, http.StatusBadRequest, "add at least one email address")
		return
	}
	sib, ok := s.sib(w, r, appEmail, scopeMailSend)
	if !ok {
		return
	}
	if !d.Shared {
		d.Shared = true
		_ = s.store.UpdateDeck(ctx, d)
	}
	u := userFrom(ctx)
	link := s.link("/s/" + d.ShareKey)
	text := firstNonEmpty(strings.TrimSpace(in.Text), fmt.Sprintf("Thank you for coming. Here are the slides of “%s”.", d.Title))
	html := `<div style="font-family:-apple-system,Segoe UI,Roboto,sans-serif;line-height:1.5;font-size:15px">` + strings.ReplaceAll(htmlEsc(text), "\n", "<br>") +
		`<p style="margin-top:20px"><a href="` + htmlEsc(link) + `" style="display:inline-block;padding:10px 18px;background:#c0571b;color:#fff;border-radius:8px;text-decoration:none">📽️ See the slides</a></p></div>`
	if _, err := postJSONTo(ctx, sib, "/api/v1/send", map[string]any{"to": to, "subject": clip("The slides: "+d.Title, 200), "body_text": text + "\n\n" + link + "\n\n— " + s.actorName(u), "body_html": html}); err != nil {
		s.appFail(w, appEmail, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": fmt.Sprintf("Sent to %d %s", len(to), plural(len(to), "person", "people"))})
}

// ---- Pocket Voice ----

func (s *Server) handleVoiceList(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appVoice, scopeVoiceRead)
	if !ok {
		return
	}
	q := url.Values{"limit": {"40"}}
	if t := strings.TrimSpace(r.URL.Query().Get("q")); t != "" {
		q.Set("q", t)
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/memos", q)
	if err != nil {
		s.appFail(w, appVoice, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "items", "memos") {
		out = append(out, map[string]any{"id": idOf(m), "title": firstNonEmpty(str(m, "title"), "Voice memo"), "date": day10(m["recorded_at"]), "seconds": int(num(m, "duration_ms") / 1000),
			"summary": clip(firstNonEmpty(str(m, "summary"), str(m, "transcript")), 200)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"memos": out})
}

// handleDeckFromVoice turns a voice memo into a first draft: with AI, an
// outline of what was said; without, a slide per paragraph.
func (s *Server) handleDeckFromVoice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		MemoID string `json:"memo_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, err := strconv.ParseInt(in.MemoID, 10, 64); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, ok := s.sib(w, r, appVoice, scopeVoiceRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/memos/"+in.MemoID, nil)
	if err != nil {
		s.appFail(w, appVoice, err)
		return
	}
	m, _ := v.(map[string]any)
	if inner, ok := m["memo"].(map[string]any); ok {
		m = inner
	}
	said := strings.TrimSpace(firstNonEmpty(str(m, "tidied"), str(m, "transcript")))
	if said == "" {
		writeErr(w, http.StatusBadRequest, "that memo has no words yet (Pocket Voice is still writing it down?)")
		return
	}
	u := userFrom(ctx)
	title := firstNonEmpty(str(m, "title"), "From a voice memo")
	drafts, aiErr := s.aiOutline(ctx, u, "Turn what was said in this voice memo into slides for the talk it describes. Keep the speaker's words and ideas; put what to say in the notes.", clip(said, 30000), 0)
	if aiErr != nil {
		drafts = []draftSlide{{Layout: "title", Title: title, Subtitle: s.actorName(u)}}
		for _, p := range strings.Split(said, "\n\n") {
			if p = strings.TrimSpace(p); p != "" && len(drafts) < 40 {
				drafts = append(drafts, draftSlide{Layout: "bullets", Title: clip(firstSentence(p), 80), Notes: p})
			}
		}
	}
	d, err := s.makeDeck(ctx, u, title, drafts, "", "")
	if err != nil {
		s.fail(w, err, "deck")
		return
	}
	text := fmt.Sprintf("A first draft of %d %s from the memo", d.Slides, plural(d.Slides, "slide", "slides"))
	if aiErr != nil {
		text += " (a slide per paragraph; with AI it makes an outline)"
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deck": d, "text": text})
}

func firstSentence(p string) string {
	for i, r := range p {
		if (r == '.' || r == '?' || r == '!') && i > 10 {
			return strings.TrimSpace(p[:i])
		}
	}
	return firstLine(p)
}

// ---- Pocket Tables ----

func (s *Server) handleTablesList(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appTables, scopeTablesRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/bases", nil)
	if err != nil {
		s.appFail(w, appTables, err)
		return
	}
	out := []map[string]any{}
	for _, b := range rows(v, "bases") {
		for _, t := range rows(b["tables"]) {
			out = append(out, map[string]any{"id": idOf(t), "name": str(t, "name"), "base": str(b, "name"), "icon": str(b, "icon")})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": out})
}

func (s *Server) tableGrid(ctx context.Context, sib *siblingConn, id string) ([][]string, error) {
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, &userErr{"pick a table"}
	}
	b, _, err := sib.client.Bytes(ctx, "/api/v1/tables/"+id+"/export.csv", nil, 8<<20)
	if err != nil {
		return nil, err
	}
	cr := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
	cr.FieldsPerRecord = -1
	grid, err := cr.ReadAll()
	if err != nil {
		return nil, &userErr{"the table could not be read"}
	}
	return grid, nil
}

// handleTablePreview shows a table's columns and first rows, to pick from.
func (s *Server) handleTablePreview(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appTables, scopeTablesRead)
	if !ok {
		return
	}
	grid, err := s.tableGrid(r.Context(), sib, chiParam(r, "tid"))
	if err != nil {
		s.appFail(w, appTables, err)
		return
	}
	if len(grid) > 13 {
		grid = grid[:13]
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": grid})
}

// handleTableSlide makes a table or a chart slide from a table (or fills one).
func (s *Server) handleTableSlide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID   int64  `json:"deck_id"`
		SlideID  int64  `json:"slide_id"`
		After    int64  `json:"after"`
		TableID  string `json:"table_id"`
		Name     string `json:"name"`
		Kind     string `json:"kind"` // table | chart
		Chart    string `json:"chart"`
		LabelCol int    `json:"label_col"`
		ValueCol int    `json:"value_col"`
		Cols     []int  `json:"cols"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appTables, scopeTablesRead)
	if !ok {
		return
	}
	grid, err := s.tableGrid(ctx, sib, in.TableID)
	if err != nil {
		s.appFail(w, appTables, err)
		return
	}
	if len(grid) < 2 {
		writeErr(w, http.StatusBadRequest, "that table has no rows")
		return
	}
	data := slideData{From: "Pocket Tables: " + cleanText(in.Name, 100)}
	layout := "table"
	if in.Kind == "chart" {
		layout = "chart"
		c := &chartData{Kind: firstNonEmpty(in.Chart, "bar")}
		for _, row := range grid[1:] {
			if in.LabelCol >= len(row) || in.ValueCol >= len(row) || in.LabelCol < 0 || in.ValueCol < 0 {
				continue
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.NewReplacer(",", "", "€", "", "$", "", "%", "", " ", "").Replace(row[in.ValueCol])), 64)
			if err != nil {
				continue
			}
			c.Labels, c.Values = append(c.Labels, row[in.LabelCol]), append(c.Values, v)
			if len(c.Labels) >= 24 {
				break
			}
		}
		if len(c.Labels) == 0 {
			writeErr(w, http.StatusBadRequest, "that column has no numbers")
			return
		}
		if in.ValueCol < len(grid[0]) {
			c.Unit = ""
		}
		data.Chart = c
	} else {
		cols := in.Cols
		if len(cols) == 0 {
			for i := range grid[0] {
				if i < 6 {
					cols = append(cols, i)
				}
			}
		}
		for i, row := range grid {
			if i > 15 {
				break
			}
			out := []string{}
			for _, c := range cols {
				if c >= 0 && c < len(row) {
					out = append(out, row[c])
				}
			}
			data.Rows = append(data.Rows, out)
		}
	}
	raw, _ := json.Marshal(data)
	title := cleanText(in.Name, 200)
	if in.SlideID > 0 {
		sl, err := s.store.GetSlide(ctx, d.UserID, in.SlideID)
		if err != nil || sl.DeckID != d.ID {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		sl.Layout, sl.Data = layout, cleanData(raw)
		if sl.Title == "" {
			sl.Title = title
		}
		if err := s.store.UpdateSlide(ctx, sl); err != nil {
			s.fail(w, err, "slide")
			return
		}
		s.live.deckChanged(ctx, s, d.ID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slide": sl, "text": "The slide shows the table's numbers"})
		return
	}
	sl := &store.Slide{DeckID: d.ID, UserID: d.UserID, Position: -1, Layout: layout, Title: title, Data: cleanData(raw), Source: sourceOf(ctx)}
	if in.After > 0 {
		if a, err := s.store.GetSlide(ctx, d.UserID, in.After); err == nil && a.DeckID == d.ID {
			sl.Position = a.Position + 1
		}
	}
	if err := s.store.CreateSlide(ctx, sl); err != nil {
		s.fail(w, err, "slide")
		return
	}
	s.live.deckChanged(ctx, s, d.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "slide": sl, "text": "A slide from the table"})
}

// ---- Pocket Translate ----

// handleTranslate makes a copy of a deck with its words and notes in another language.
func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		To    string `json:"to"`
		Notes bool   `json:"notes"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return idParam(r, "id") })
	if !ok {
		return
	}
	lang := strings.TrimSpace(in.To)
	if lang == "" || len(lang) > 20 {
		writeErr(w, http.StatusBadRequest, "pick the language to translate to")
		return
	}
	sib, ok := s.sib(w, r, appTranslate, scopeTranslate)
	if !ok {
		return
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	texts := []string{d.Title}
	type ref struct {
		i     int
		field string
		opt   int
	}
	refs := []ref{}
	add := func(i int, field string, v string, opt int) {
		if strings.TrimSpace(v) != "" {
			texts = append(texts, v)
			refs = append(refs, ref{i, field, opt})
		}
	}
	for i, sl := range slides {
		add(i, "title", sl.Title, 0)
		add(i, "subtitle", sl.Subtitle, 0)
		add(i, "body", sl.Body, 0)
		add(i, "body2", sl.Body2, 0)
		if in.Notes {
			add(i, "notes", sl.Notes, 0)
		}
		for j, o := range dataOf(&sl).Options {
			add(i, "option", o, j)
		}
	}
	out := []string{}
	for start := 0; start < len(texts); start += 150 {
		end := start + 150
		if end > len(texts) {
			end = len(texts)
		}
		res, err := postJSONTo(ctx, sib, "/api/v1/translate", map[string]any{"texts": texts[start:end], "to": lang, "from": "auto"})
		if err != nil {
			s.appFail(w, appTranslate, err)
			return
		}
		arr, _ := res["texts"].([]any)
		if len(arr) != end-start {
			writeErr(w, http.StatusBadGateway, "Pocket Translate gave back a different number of texts")
			return
		}
		for _, x := range arr {
			v, _ := x.(string)
			out = append(out, v)
		}
	}
	n, err := s.copyDeck(ctx, userFrom(ctx), d, firstNonEmpty(out[0], d.Title)+" ("+lang+")")
	if err != nil {
		s.fail(w, err, "copy")
		return
	}
	copies, _ := s.store.Slides(ctx, n.ID)
	for k, rf := range refs {
		if rf.i >= len(copies) {
			continue
		}
		sl := &copies[rf.i]
		v := out[k+1]
		switch rf.field {
		case "title":
			sl.Title = v
		case "subtitle":
			sl.Subtitle = v
		case "body":
			sl.Body = v
		case "body2":
			sl.Body2 = v
		case "notes":
			sl.Notes = v
		case "option":
			dd := dataOf(sl)
			if rf.opt < len(dd.Options) {
				dd.Options[rf.opt] = v
				b, _ := json.Marshal(dd)
				sl.Data = cleanData(b)
			}
		}
	}
	for i := range copies {
		_ = s.store.UpdateSlide(ctx, &copies[i])
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deck": n, "text": "A copy in " + lang + " is ready"})
}

// ---- Pocket Blog ----

// handleToBlog turns a deck into a blog post draft: each slide a part, the notes as the words.
func (s *Server) handleToBlog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID int64 `json:"deck_id"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appBlog, scopeBlogWrite)
	if !ok {
		return
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	var b strings.Builder
	for _, sl := range visible(slides) {
		if sl.Layout == "title" || sl.Layout == "agenda" || sl.Layout == "end" {
			continue
		}
		if sl.Title != "" {
			fmt.Fprintf(&b, "## %s\n\n", sl.Title)
		}
		switch sl.Layout {
		case "quote":
			fmt.Fprintf(&b, "> %s\n>\n> — %s\n\n", strings.ReplaceAll(sl.Body, "\n", "\n> "), sl.Subtitle)
		case "number":
			fmt.Fprintf(&b, "**%s** %s\n\n", sl.Title, sl.Subtitle)
		default:
			if sl.Subtitle != "" && sl.Layout != "section" {
				fmt.Fprintf(&b, "*%s*\n\n", sl.Subtitle)
			}
			if sl.Body != "" {
				fmt.Fprintf(&b, "%s\n\n", sl.Body)
			}
			if sl.Body2 != "" {
				fmt.Fprintf(&b, "%s\n\n", sl.Body2)
			}
		}
		if n := strings.TrimSpace(sl.Notes); n != "" {
			fmt.Fprintf(&b, "%s\n\n", n)
		}
	}
	if d.Shared {
		fmt.Fprintf(&b, "---\n[See the slides](%s)\n", s.link("/s/"+d.ShareKey))
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/posts", map[string]any{"title": clip(d.Title, 200), "body": b.String(), "excerpt": clip(firstNonEmpty(d.About, d.TalkTitle), 200),
		"tags": append([]string{"talk"}, d.Tags...), "source_ref": fmt.Sprintf("slides:%d", d.ID)})
	if err != nil {
		s.appFail(w, appBlog, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "A draft post is waiting in Pocket Blog", "url": linkTo(sib, "/posts/"+idOf(res))})
}

// ---- Pocket Learn ----

// handleToLearn turns a deck's slides into flashcards: the heading on the
// front, the points on the back; a quiz poll with its right answer as a card too.
func (s *Server) handleToLearn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		DeckID int64  `json:"deck_id"`
		Deck   string `json:"deck"`
	}
	d, ok := s.deckBody(w, r, &in, func() int64 { return in.DeckID })
	if !ok {
		return
	}
	slides, _ := s.store.Slides(ctx, d.ID)
	type card struct{ front, back, ref string }
	cards := []card{}
	for _, sl := range visible(slides) {
		ref := fmt.Sprintf("slides:%d:%d", d.ID, sl.ID)
		switch sl.Layout {
		case "poll":
			dd := dataOf(&sl)
			if dd.Correct > 0 && dd.Correct <= len(dd.Options) && sl.Title != "" {
				cards = append(cards, card{sl.Title, dd.Options[dd.Correct-1], ref})
			}
		case "bullets", "two", "image_left", "image_right", "number":
			back := strings.TrimSpace(strings.Join([]string{sl.Subtitle, sl.Body, sl.Body2}, "\n"))
			if sl.Title != "" && back != "" {
				cards = append(cards, card{sl.Title, back, ref})
			}
		case "quote":
			if sl.Body != "" && sl.Subtitle != "" {
				cards = append(cards, card{"Who said: “" + clip(sl.Body, 300) + "”", sl.Subtitle, ref})
			}
		}
	}
	if len(cards) == 0 {
		writeErr(w, http.StatusBadRequest, "no slide has a heading with points to learn")
		return
	}
	sib, ok := s.sib(w, r, appLearn, scopeLearnWrite)
	if !ok {
		return
	}
	deck := firstNonEmpty(strings.TrimSpace(in.Deck), s.settingsOf(ctx, d.UserID).LearnDeck, d.Title)
	n, dup := 0, 0
	for _, c := range cards {
		res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"deck": deck, "kind": "basic", "front": clip(c.front, 500), "back": clip(c.back, 2000),
			"external_id": c.ref, "source_ref": fmt.Sprintf("slides:%d", d.ID), "source_url": s.deckURL(d)})
		if err != nil {
			if n == 0 {
				s.appFail(w, appLearn, err)
				return
			}
			break
		}
		if res["duplicate"] == true {
			dup++
		} else {
			n++
		}
	}
	text := fmt.Sprintf("%d %s added to “%s” in Pocket Learn", n, plural(n, "card", "cards"), deck)
	if dup > 0 {
		text += fmt.Sprintf(" (%d already there)", dup)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": text, "url": linkTo(sib, "/")})
}

// ---- Pocket Alerts ----

// alert tells the person through Pocket Alerts (best effort, never in the way).
func (s *Server) alert(ctx context.Context, uid int64, title, body, path, priority string) bool {
	sib, err := s.clients(ctx, uid).conn(ctx, appAlerts, scopeAlertsWrite)
	if err != nil {
		return false
	}
	_, err = postJSONTo(ctx, sib, "/api/v1/alerts", map[string]any{"title": clip(title, 200), "body": clip(body, 2000), "priority": priority, "tags": []string{"projector"}, "click": s.link(path)})
	if err != nil {
		s.log.Debug("pocket alerts: send failed", "err", err)
		return false
	}
	return true
}
