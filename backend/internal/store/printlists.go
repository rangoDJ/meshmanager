package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Print item states, in the order a model moves through them.
const (
	PrintTodo     = "todo"
	PrintPrinting = "printing"
	PrintDone     = "done"
)

func ValidPrintStatus(s string) bool {
	return s == PrintTodo || s == PrintPrinting || s == PrintDone
}

type PrintList struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Printer   string    `json:"printer"`
	CreatedAt time.Time `json:"created_at"`
	Todo      int64     `json:"todo"`
	Printing  int64     `json:"printing"`
	Done      int64     `json:"done"`
}

const printListSelect = `
	SELECT l.id, l.name, l.printer, l.created_at,
	       count(i.id) FILTER (WHERE i.status = 'todo'),
	       count(i.id) FILTER (WHERE i.status = 'printing'),
	       count(i.id) FILTER (WHERE i.status = 'done')
	FROM print_lists l LEFT JOIN print_items i ON i.list_id = l.id`

func (s *Store) ListPrintLists(ctx context.Context) ([]PrintList, error) {
	rows, err := s.db.Query(ctx, printListSelect+" GROUP BY l.id ORDER BY lower(l.printer), lower(l.name)")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[PrintList])
}

func (s *Store) GetPrintList(ctx context.Context, id int64) (PrintList, error) {
	rows, err := s.db.Query(ctx, printListSelect+" WHERE l.id = $1 GROUP BY l.id", id)
	if err != nil {
		return PrintList{}, err
	}
	l, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[PrintList])
	return l, notFound(err)
}

func (s *Store) CreatePrintList(ctx context.Context, name, printer string) (PrintList, error) {
	var id int64
	err := s.db.QueryRow(ctx, "INSERT INTO print_lists (name, printer) VALUES ($1, $2) RETURNING id",
		strings.TrimSpace(name), strings.TrimSpace(printer)).Scan(&id)
	if isUniqueViolation(err) {
		return PrintList{}, ErrConflict
	}
	if err != nil {
		return PrintList{}, err
	}
	return s.GetPrintList(ctx, id)
}

func (s *Store) UpdatePrintList(ctx context.Context, id int64, name, printer *string) (PrintList, error) {
	trim := func(p *string) *string {
		if p == nil {
			return nil
		}
		v := strings.TrimSpace(*p)
		return &v
	}
	tag, err := s.db.Exec(ctx, "UPDATE print_lists SET name = coalesce($2, name), printer = coalesce($3, printer) WHERE id = $1",
		id, trim(name), trim(printer))
	if isUniqueViolation(err) {
		return PrintList{}, ErrConflict
	}
	if err != nil {
		return PrintList{}, err
	}
	if tag.RowsAffected() == 0 {
		return PrintList{}, ErrNotFound
	}
	return s.GetPrintList(ctx, id)
}

func (s *Store) DeletePrintList(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM print_lists WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type PrintItem struct {
	ID         int64      `json:"id"`
	ListID     int64      `json:"list_id"`
	Quantity   int        `json:"quantity"`
	Status     string     `json:"status"`
	Note       string     `json:"note"`
	Position   float64    `json:"position"`
	AddedAt    time.Time  `json:"added_at"`
	FinishedAt *time.Time `json:"finished_at"`
	File       FileItem   `json:"-"`
}

// ListPrintItems returns a list's items with their files, in queue order. Files that
// have gone missing from disk are included, so a queue never silently shrinks.
func (s *Store) ListPrintItems(ctx context.Context, listID int64) ([]PrintItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT i.id, i.list_id, i.quantity, i.status, i.note, i.position, i.added_at, i.finished_at, `+fileColumns+`
		FROM print_items i JOIN files f ON f.id = i.file_id`+fileJoinsRest+`
		WHERE i.list_id = $1
		ORDER BY i.position, i.id`, listID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (PrintItem, error) {
		var it PrintItem
		f, err := scanFileItemWith(r, &it.ID, &it.ListID, &it.Quantity, &it.Status, &it.Note, &it.Position,
			&it.AddedAt, &it.FinishedAt)
		it.File = f
		return it, err
	})
}

// AddPrintItems queues files on a list. A file already waiting or printing stays as it
// is; one that was done goes back to waiting.
func (s *Store) AddPrintItems(ctx context.Context, listID int64, fileIDs []int64, quantity int) (int64, error) {
	if quantity < 1 {
		quantity = 1
	}
	var added int64
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		if _, err := q.GetPrintList(ctx, listID); err != nil {
			return err
		}
		tag, err := q.db.Exec(ctx, `
			INSERT INTO print_items (list_id, file_id, quantity, position)
			SELECT $1, f.id, $3,
			       coalesce((SELECT max(position) FROM print_items WHERE list_id = $1), 0)
			       + row_number() OVER (ORDER BY lower(f.name), f.id)
			FROM files f WHERE f.id = ANY($2)
			ON CONFLICT (list_id, file_id) DO UPDATE SET
				status = CASE WHEN print_items.status = 'done' THEN 'todo' ELSE print_items.status END,
				finished_at = CASE WHEN print_items.status = 'done' THEN NULL ELSE print_items.finished_at END`,
			listID, fileIDs, quantity)
		added = tag.RowsAffected()
		return err
	})
	return added, err
}

type PrintItemUpdate struct {
	Status   *string
	Quantity *int
	Note     *string
	Position *float64
	ListID   *int64
}

func (s *Store) UpdatePrintItem(ctx context.Context, id int64, u PrintItemUpdate) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE print_items SET
			status = coalesce($2, status),
			quantity = coalesce($3, quantity),
			note = coalesce($4, note),
			position = coalesce($5, position),
			list_id = coalesce($6, list_id),
			finished_at = CASE
				WHEN $2::text IS NULL THEN finished_at
				WHEN $2 = 'done' THEN coalesce(finished_at, now())
				ELSE NULL END
		WHERE id = $1`, id, u.Status, u.Quantity, u.Note, u.Position, u.ListID)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeletePrintItem(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM print_items WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearDonePrintItems drops a list's finished items.
func (s *Store) ClearDonePrintItems(ctx context.Context, listID int64) (int64, error) {
	tag, err := s.db.Exec(ctx, "DELETE FROM print_items WHERE list_id = $1 AND status = 'done'", listID)
	return tag.RowsAffected(), err
}

// QueueRef says where a file sits in the print queue.
type QueueRef struct {
	ItemID   int64  `json:"item_id"`
	ListID   int64  `json:"list_id"`
	ListName string `json:"list_name"`
	Status   string `json:"status"`
	Quantity int    `json:"quantity"`
}

func (s *Store) FileQueue(ctx context.Context, fileID int64) ([]QueueRef, error) {
	rows, err := s.db.Query(ctx, `
		SELECT i.id, l.id, l.name, i.status, i.quantity
		FROM print_items i JOIN print_lists l ON l.id = i.list_id
		WHERE i.file_id = $1 ORDER BY lower(l.name)`, fileID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[QueueRef])
	if out == nil {
		out = []QueueRef{}
	}
	return out, err
}
