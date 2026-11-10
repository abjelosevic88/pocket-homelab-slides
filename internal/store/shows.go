package store

import (
	"context"
	"encoding/json"
)

const showCols = `h.id, h.user_id, h.deck_id, h.kind, h.remote_key, h.pos, h.step, h.black, h.started_at, h.ended_at, h.timings, h.total_ms, h.viewers, h.reactions, h.note,
	COALESCE((SELECT d.title FROM decks d WHERE d.id = h.deck_id), ''),
	(SELECT COUNT(1) FROM questions q WHERE q.show_id = h.id),
	(SELECT COUNT(1) FROM votes v WHERE v.show_id = h.id),
	(SELECT COUNT(1) FROM feedback f WHERE f.show_id = h.id),
	COALESCE((SELECT AVG(f.rating) FROM feedback f WHERE f.show_id = h.id AND f.rating > 0), 0)`

func scanShow(row interface{ Scan(...any) error }) (*Show, error) {
	var h Show
	var timings string
	if err := row.Scan(&h.ID, &h.UserID, &h.DeckID, &h.Kind, &h.RemoteKey, &h.Pos, &h.Step, &h.Black, &h.StartedAt, &h.EndedAt, &timings, &h.TotalMs,
		&h.Viewers, &h.Reactions, &h.Note, &h.DeckTitle, &h.Questions, &h.Votes, &h.Feedback, &h.Rating); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(timings), &h.Timings)
	if h.Timings == nil {
		h.Timings = map[string]int64{}
	}
	return &h, nil
}

// StartShow begins presenting (or practising) a deck.
func (s *Store) StartShow(ctx context.Context, h *Show) error {
	if h.RemoteKey == "" {
		h.RemoteKey = RandomToken(12)
	}
	if h.Timings == nil {
		h.Timings = map[string]int64{}
	}
	h.StartedAt = firstSet(h.StartedAt, Now())
	res, err := s.DB.ExecContext(ctx, `INSERT INTO shows (user_id, deck_id, kind, remote_key, pos, step, black, started_at, ended_at, timings, total_ms, viewers, reactions, note)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, h.UserID, h.DeckID, h.Kind, h.RemoteKey, h.Pos, h.Step, boolInt(h.Black), h.StartedAt, h.EndedAt, toJSON(h.Timings),
		h.TotalMs, h.Viewers, h.Reactions, h.Note)
	if err != nil {
		return conflict(err)
	}
	h.ID, _ = res.LastInsertId()
	return nil
}

// SaveShow keeps where a show is and how long each slide took.
func (s *Store) SaveShow(ctx context.Context, h *Show) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE shows SET pos=?, step=?, black=?, ended_at=?, timings=?, total_ms=?, viewers=?, reactions=?, note=?, kind=? WHERE id = ? AND user_id = ?`,
		h.Pos, h.Step, boolInt(h.Black), h.EndedAt, toJSON(h.Timings), h.TotalMs, h.Viewers, h.Reactions, h.Note, h.Kind, h.ID, h.UserID)
	return err
}

// GetShow loads one of a person's shows.
func (s *Store) GetShow(ctx context.Context, uid, id int64) (*Show, error) {
	return scanShow(s.DB.QueryRowContext(ctx, `SELECT `+showCols+` FROM shows h WHERE h.id = ? AND h.user_id = ?`, id, uid))
}

// ShowByRemote finds a running show by its phone key.
func (s *Store) ShowByRemote(ctx context.Context, key string) (*Show, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	return scanShow(s.DB.QueryRowContext(ctx, `SELECT `+showCols+` FROM shows h WHERE h.remote_key = ? AND h.ended_at IS NULL`, key))
}

// LiveShow is the deck's show that is running now, if any.
func (s *Store) LiveShow(ctx context.Context, deckID int64) (*Show, error) {
	return scanShow(s.DB.QueryRowContext(ctx, `SELECT `+showCols+` FROM shows h WHERE h.deck_id = ? AND h.ended_at IS NULL ORDER BY h.id DESC LIMIT 1`, deckID))
}

// LatestShow is the deck's most recent talk (for feedback after it ended).
func (s *Store) LatestShow(ctx context.Context, deckID int64, kind string) (*Show, error) {
	return scanShow(s.DB.QueryRowContext(ctx, `SELECT `+showCols+` FROM shows h WHERE h.deck_id = ? AND h.kind = ? ORDER BY h.id DESC LIMIT 1`, deckID, kind))
}

// Shows lists a deck's shows (deckID 0 = all of a person's), newest first.
func (s *Store) Shows(ctx context.Context, uid, deckID int64, limit int) ([]Show, error) {
	q := `SELECT ` + showCols + ` FROM shows h WHERE h.user_id = ?`
	args := []any{uid}
	if deckID > 0 {
		q += ` AND h.deck_id = ?`
		args = append(args, deckID)
	}
	q += ` ORDER BY h.started_at DESC, h.id DESC LIMIT ?`
	args = append(args, limit)
	return s.showList(ctx, q, args...)
}

// ShowsBetween lists a person's shows started in [from, to) (UTC ISO).
func (s *Store) ShowsBetween(ctx context.Context, uid int64, from, to string) ([]Show, error) {
	return s.showList(ctx, `SELECT `+showCols+` FROM shows h WHERE h.user_id = ? AND h.started_at >= ? AND h.started_at < ? ORDER BY h.started_at`, uid, from, to)
}

// RunningShows lists every show not ended yet (to end forgotten ones).
func (s *Store) RunningShows(ctx context.Context) ([]Show, error) {
	return s.showList(ctx, `SELECT `+showCols+` FROM shows h WHERE h.ended_at IS NULL`)
}

func (s *Store) showList(ctx context.Context, q string, args ...any) ([]Show, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Show{}
	for rows.Next() {
		h, err := scanShow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

// DeleteShow removes a show with its questions, votes and feedback.
func (s *Store) DeleteShow(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM shows WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- votes ----

// Vote records (or changes) one person's answer to a poll slide.
func (s *Store) Vote(ctx context.Context, uid, showID, slideID int64, choice int, voter string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO votes (show_id, user_id, slide_id, choice, voter) VALUES (?,?,?,?,?)
		ON CONFLICT(show_id, slide_id, voter) DO UPDATE SET choice = excluded.choice`, showID, uid, slideID, choice, voter)
	return err
}

// Tally counts a show's votes per slide and choice.
func (s *Store) Tally(ctx context.Context, showID int64) (map[int64]map[int]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT slide_id, choice, COUNT(1) FROM votes WHERE show_id = ? GROUP BY slide_id, choice`, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[int]int{}
	for rows.Next() {
		var sid int64
		var c, n int
		if err := rows.Scan(&sid, &c, &n); err != nil {
			return nil, err
		}
		if out[sid] == nil {
			out[sid] = map[int]int{}
		}
		out[sid][c] = n
	}
	return out, rows.Err()
}

// MyVotes is what one voter picked in a show, per slide.
func (s *Store) MyVotes(ctx context.Context, showID int64, voter string) map[int64]int {
	out := map[int64]int{}
	rows, err := s.DB.QueryContext(ctx, `SELECT slide_id, choice FROM votes WHERE show_id = ? AND voter = ?`, showID, voter)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var sid int64
		var c int
		if rows.Scan(&sid, &c) == nil {
			out[sid] = c
		}
	}
	return out
}

// ---- questions ----

// AddQuestion keeps a question from the audience.
func (s *Store) AddQuestion(ctx context.Context, q *Question) error {
	q.CreatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO questions (show_id, user_id, slide_id, name, body, votes, answered, hidden, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		q.ShowID, q.UserID, nullI(q.SlideID), q.Name, q.Body, q.Votes, boolInt(q.Answered), boolInt(q.Hidden), q.CreatedAt)
	if err != nil {
		return err
	}
	q.ID, _ = res.LastInsertId()
	return nil
}

// Questions lists a show's questions, unanswered and most liked first.
func (s *Store) Questions(ctx context.Context, showID int64) ([]Question, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, show_id, user_id, slide_id, name, body, votes, answered, hidden, created_at FROM questions WHERE show_id = ?
		ORDER BY answered, votes DESC, id`, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Question{}
	for rows.Next() {
		var q Question
		if err := rows.Scan(&q.ID, &q.ShowID, &q.UserID, &q.SlideID, &q.Name, &q.Body, &q.Votes, &q.Answered, &q.Hidden, &q.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// GetQuestion loads one of a person's questions.
func (s *Store) GetQuestion(ctx context.Context, uid, id int64) (*Question, error) {
	var q Question
	err := s.DB.QueryRowContext(ctx, `SELECT id, show_id, user_id, slide_id, name, body, votes, answered, hidden, created_at FROM questions WHERE id = ? AND user_id = ?`, id, uid).
		Scan(&q.ID, &q.ShowID, &q.UserID, &q.SlideID, &q.Name, &q.Body, &q.Votes, &q.Answered, &q.Hidden, &q.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &q, nil
}

// MarkQuestion answers or hides a question.
func (s *Store) MarkQuestion(ctx context.Context, q *Question) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE questions SET answered = ?, hidden = ? WHERE id = ? AND user_id = ?`, boolInt(q.Answered), boolInt(q.Hidden), q.ID, q.UserID)
	return err
}

// LikeQuestion adds one to a question of a show.
func (s *Store) LikeQuestion(ctx context.Context, showID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE questions SET votes = votes + 1 WHERE id = ? AND show_id = ? AND hidden = 0`, id, showID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- feedback ----

// AddFeedback keeps (or replaces) what one person thought of a talk.
func (s *Store) AddFeedback(ctx context.Context, f *Feedback, voter string) error {
	f.CreatedAt = Now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO feedback (show_id, user_id, rating, comment, name, voter, created_at) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(show_id, voter) DO UPDATE SET rating = excluded.rating, comment = excluded.comment, name = excluded.name`,
		f.ShowID, f.UserID, f.Rating, f.Comment, f.Name, voter, f.CreatedAt)
	return err
}

// FeedbackOf lists what the audience thought of a show.
func (s *Store) FeedbackOf(ctx context.Context, showID int64) ([]Feedback, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, show_id, user_id, rating, comment, name, created_at FROM feedback WHERE show_id = ? ORDER BY id DESC`, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Feedback{}
	for rows.Next() {
		var f Feedback
		if err := rows.Scan(&f.ID, &f.ShowID, &f.UserID, &f.Rating, &f.Comment, &f.Name, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
