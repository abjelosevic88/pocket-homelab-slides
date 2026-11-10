package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-slides/internal/store"
)

const maxUpload = 25 << 20

var allowedMime = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true, "image/svg+xml": false}

// handleUpload stores a picture (shrunk by the browser) for one of the
// caller's decks, or for a theme's logo (no deck).
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeErr(w, http.StatusBadRequest, "a picture can be up to 25 MB")
		return
	}
	up := &store.Upload{UserID: u.ID}
	if id, _ := strconv.ParseInt(r.FormValue("deck"), 10, 64); id > 0 {
		d, err := s.store.GetDeck(ctx, u.ID, id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		up.DeckID = d.ID
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no file")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUpload+1))
	if err != nil || len(data) > maxUpload {
		writeErr(w, http.StatusBadRequest, "a picture can be up to 25 MB")
		return
	}
	mime := http.DetectContentType(data)
	if i := strings.Index(mime, ";"); i > 0 {
		mime = mime[:i]
	}
	if !allowedMime[mime] {
		writeErr(w, http.StatusBadRequest, "pictures only (JPEG, PNG, WebP or GIF)")
		return
	}
	up.Mime, up.Data, up.Name = mime, data, safeName(hdr.Filename)
	up.Width, _ = strconv.Atoi(r.FormValue("width"))
	up.Height, _ = strconv.Atoi(r.FormValue("height"))
	if tf, _, err := r.FormFile("thumb"); err == nil {
		up.Thumb, _ = io.ReadAll(io.LimitReader(tf, 1<<20))
		tf.Close()
	}
	if err := s.store.SaveUpload(ctx, up); err != nil {
		s.fail(w, err, "upload")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"upload": up, "url": fmt.Sprintf("/api/v1/uploads/%d", up.ID)})
}

func mimeByExt(name string) string {
	return map[string]string{".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/plain", ".csv": "text/csv", ".zip": "application/zip", ".mp3": "audio/mpeg",
		".m4a": "audio/mp4", ".mp4": "video/mp4", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".odt": "application/vnd.oasis.opendocument.text", ".doc": "application/msword", ".xls": "application/vnd.ms-excel"}[strings.ToLower(filepath.Ext(name))]
}

func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*[]()`, r) {
			return '_'
		}
		return r
	}, n)
	return firstNonEmpty(clip(strings.TrimSpace(n), 120), "file")
}

func (s *Server) serveUpload(w http.ResponseWriter, r *http.Request, up *store.Upload) {
	data := up.Data
	if queryBool(r, "thumb") && len(up.Thumb) > 0 {
		data = up.Thumb
	}
	w.Header().Set("Content-Type", up.Mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disp := "inline"
	if !strings.HasPrefix(up.Mime, "image/") && up.Mime != "application/pdf" && !strings.HasPrefix(up.Mime, "audio/") && !strings.HasPrefix(up.Mime, "video/") {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, strings.ReplaceAll(up.Name, `"`, "")))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	http.ServeContent(w, r, up.Name, time.Time{}, bytes.NewReader(data))
}

func (s *Server) handleGetUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	up, err := s.store.GetUpload(ctx, idParam(r, "id"))
	if err != nil || up.UserID != actorID(ctx) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.serveUpload(w, r, up)
}
