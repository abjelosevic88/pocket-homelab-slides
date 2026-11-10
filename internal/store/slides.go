package store

import (
	"context"
	"database/sql"
)

const slideCols = `id, deck_id, user_id, position, layout, title, subtitle, body, body2, image_id, data, notes, style, hidden, seconds, source, created_at, updated_at`

func scanSlide(row interface{ Scan(...any) error }) (*Slide, error) {
	var sl Slide
	var data string
	if err := row.Scan(&sl.ID, &sl.DeckID, &sl.UserID, &sl.Position, &sl.Layout, &sl.Title, &sl.Subtitle, &sl.Body, &sl.Body2, &sl.ImageID, &data,
		&sl.Notes, &sl.Style, &sl.Hidden, &sl.Seconds, &sl.Source, &sl.CreatedAt, &sl.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	sl.Data = rawOr(data)
	return &sl, nil
}

// Slides lists a deck's slides in order.
func (s *Store) Slides(ctx context.Context, deckID int64) ([]Slide, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+slideCols+` FROM slides WHERE deck_id = ? ORDER BY position, id`, deckID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Slide{}
	for rows.Next() {
		sl, err := scanSlide(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sl)
	}
	return out, rows.Err()
}

// GetSlide loads one of a person's slides.
func (s *Store) GetSlide(ctx context.Context, uid, id int64) (*Slide, error) {
	return scanSlide(s.DB.QueryRowContext(ctx, `SELECT `+slideCols+` FROM slides WHERE id = ? AND user_id = ?`, id, uid))
}

// CreateSlide adds a slide at position (-1 = at the end), moving the ones after it down.
func (s *Store) CreateSlide(ctx context.Context, sl *Slide) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM slides WHERE deck_id = ?`, sl.DeckID).Scan(&n); err != nil {
		return err
	}
	if sl.Position < 0 || sl.Position > n {
		sl.Position = n
	} else if _, err := tx.ExecContext(ctx, `UPDATE slides SET position = position + 1 WHERE deck_id = ? AND position >= ?`, sl.DeckID, sl.Position); err != nil {
		return err
	}
	now := Now()
	sl.CreatedAt, sl.UpdatedAt = now, now
	res, err := tx.ExecContext(ctx, `INSERT INTO slides (deck_id, user_id, position, layout, title, subtitle, body, body2, image_id, data, notes, style, hidden, seconds, source, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, sl.DeckID, sl.UserID, sl.Position, sl.Layout, sl.Title, sl.Subtitle, sl.Body, sl.Body2, nullI(sl.ImageID),
		string(rawOr(string(sl.Data))), sl.Notes, sl.Style, boolInt(sl.Hidden), sl.Seconds, sl.Source, now, now)
	if err != nil {
		return err
	}
	sl.ID, _ = res.LastInsertId()
	if sl.ImageID != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE uploads SET deck_id = ? WHERE id = ? AND user_id = ?`, sl.DeckID, *sl.ImageID, sl.UserID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE decks SET updated_at = ? WHERE id = ?`, now, sl.DeckID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateSlide saves a slide's content.
func (s *Store) UpdateSlide(ctx context.Context, sl *Slide) error {
	sl.UpdatedAt = Now()
	_, err := s.DB.ExecContext(ctx, `UPDATE slides SET layout=?, title=?, subtitle=?, body=?, body2=?, image_id=?, data=?, notes=?, style=?, hidden=?, seconds=?, updated_at=?
		WHERE id = ? AND user_id = ?`, sl.Layout, sl.Title, sl.Subtitle, sl.Body, sl.Body2, nullI(sl.ImageID), string(rawOr(string(sl.Data))), sl.Notes, sl.Style,
		boolInt(sl.Hidden), sl.Seconds, sl.UpdatedAt, sl.ID, sl.UserID)
	if err != nil {
		return err
	}
	if sl.ImageID != nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE uploads SET deck_id = ? WHERE id = ? AND user_id = ?`, sl.DeckID, *sl.ImageID, sl.UserID)
	}
	s.TouchDeck(ctx, sl.DeckID)
	return nil
}

// DeleteSlide removes a slide and closes the gap.
func (s *Store) DeleteSlide(ctx context.Context, uid, id int64) error {
	sl, err := s.GetSlide(ctx, uid, id)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM slides WHERE id = ? AND user_id = ?`, id, uid); err != nil {
		return err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE slides SET position = position - 1 WHERE deck_id = ? AND position > ?`, sl.DeckID, sl.Position)
	s.TouchDeck(ctx, sl.DeckID)
	return nil
}

// ReorderSlides puts a deck's slides in the given order (ids not in the deck are ignored,
// slides not listed keep their order after the listed ones).
func (s *Store) ReorderSlides(ctx context.Context, deckID int64, ids []int64) error {
	cur, err := s.Slides(ctx, deckID)
	if err != nil {
		return err
	}
	in := map[int64]bool{}
	for _, sl := range cur {
		in[sl.ID] = true
	}
	order := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if in[id] && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	for _, sl := range cur {
		if !seen[sl.ID] {
			order = append(order, sl.ID)
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, `UPDATE slides SET position = ? WHERE id = ? AND deck_id = ?`, i, id, deckID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE decks SET updated_at = ? WHERE id = ?`, Now(), deckID); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- uploads ----

// SaveUpload stores a picture.
func (s *Store) SaveUpload(ctx context.Context, up *Upload) error {
	up.CreatedAt = Now()
	var deck any
	if up.DeckID > 0 {
		deck = up.DeckID
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO uploads (user_id, deck_id, name, mime, data, thumb, width, height, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		up.UserID, deck, up.Name, up.Mime, up.Data, up.Thumb, up.Width, up.Height, up.CreatedAt)
	if err != nil {
		return err
	}
	up.ID, _ = res.LastInsertId()
	up.Size = len(up.Data)
	return nil
}

// GetUpload loads a picture with its bytes.
func (s *Store) GetUpload(ctx context.Context, id int64) (*Upload, error) {
	var up Upload
	var deck sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, deck_id, name, mime, data, thumb, width, height, created_at FROM uploads WHERE id = ?`, id).
		Scan(&up.ID, &up.UserID, &deck, &up.Name, &up.Mime, &up.Data, &up.Thumb, &up.Width, &up.Height, &up.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	up.DeckID = deck.Int64
	up.Size = len(up.Data)
	return &up, nil
}

// UploadsOf lists the pictures of a deck (without their bytes).
func (s *Store) UploadsOf(ctx context.Context, uid, deckID int64) ([]Upload, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, mime, width, height, length(data), created_at FROM uploads WHERE user_id = ? AND deck_id = ? ORDER BY id`, uid, deckID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Upload{}
	for rows.Next() {
		up := Upload{UserID: uid, DeckID: deckID}
		if err := rows.Scan(&up.ID, &up.Name, &up.Mime, &up.Width, &up.Height, &up.Size, &up.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, up)
	}
	return out, rows.Err()
}

// UploadUsed reports whether a deck's slide or a theme shows the picture.
func (s *Store) UploadUsed(ctx context.Context, id int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(1) FROM slides WHERE image_id = ?) + (SELECT COUNT(1) FROM themes WHERE logo_id = ?)`, id, id).Scan(&n)
	return n > 0
}

// PurgeUploads removes pictures no slide or theme uses that are older than before.
func (s *Store) PurgeUploads(ctx context.Context, before string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM uploads WHERE created_at < ? AND id NOT IN (SELECT image_id FROM slides WHERE image_id IS NOT NULL)
		AND id NOT IN (SELECT logo_id FROM themes WHERE logo_id IS NOT NULL)`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
