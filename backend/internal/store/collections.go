package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrSmartCollection is returned when files are added to a collection that fills itself.
var ErrSmartCollection = errors.New("smart collections fill themselves")

type Collection struct {
	ID       int64  `json:"id"`
	ParentID *int64 `json:"parent_id"`
	Name     string `json:"name"`
	// Rules is set for a smart collection, whose files are whatever matches.
	Rules *Rules `json:"rules"`
	// Files counts the present files in this collection itself, not the ones nested in it.
	Files     int64     `json:"files"`
	CreatedAt time.Time `json:"created_at"`
}

func scanCollection(r pgx.Row) (Collection, error) {
	var c Collection
	var raw []byte
	if err := r.Scan(&c.ID, &c.ParentID, &c.Name, &raw, &c.CreatedAt); err != nil {
		return c, err
	}
	if raw != nil {
		var rules Rules
		if err := json.Unmarshal(raw, &rules); err == nil {
			c.Rules = &rules
		}
	}
	return c, nil
}

// ListCollections returns every collection with its file count, parents before children.
func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.db.Query(ctx, "SELECT id, parent_id, name, rules, created_at FROM collections ORDER BY lower(name), id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	manual := map[int64]int64{}
	crows, err := s.db.Query(ctx, `
		SELECT cf.collection_id, count(*) FROM collection_files cf
		JOIN files f ON f.id = cf.file_id AND f.status = 'present'
		GROUP BY cf.collection_id`)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var id, n int64
		if err := crows.Scan(&id, &n); err != nil {
			crows.Close()
			return nil, err
		}
		manual[id] = n
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		if out[i].Rules == nil {
			out[i].Files = manual[out[i].ID]
			continue
		}
		n, err := s.countRules(ctx, *out[i].Rules)
		if err != nil {
			return nil, err
		}
		out[i].Files = n
	}
	return out, nil
}

func (s *Store) GetCollection(ctx context.Context, id int64) (Collection, error) {
	c, err := scanCollection(s.db.QueryRow(ctx,
		"SELECT id, parent_id, name, rules, created_at FROM collections WHERE id = $1", id))
	if err != nil {
		return c, notFound(err)
	}
	if c.Rules != nil {
		c.Files, err = s.countRules(ctx, *c.Rules)
	} else {
		err = s.db.QueryRow(ctx, `
			SELECT count(*) FROM collection_files cf JOIN files f ON f.id = cf.file_id AND f.status = 'present'
			WHERE cf.collection_id = $1`, id).Scan(&c.Files)
	}
	return c, err
}

func (s *Store) CreateCollection(ctx context.Context, name string, parentID *int64, rules *Rules) (Collection, error) {
	var raw []byte
	if rules != nil {
		raw, _ = json.Marshal(rules)
	}
	var id int64
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		if parentID != nil {
			if _, err := q.GetCollection(ctx, *parentID); err != nil {
				return err
			}
		}
		return q.db.QueryRow(ctx,
			"INSERT INTO collections (name, parent_id, rules) VALUES ($1, $2, $3) RETURNING id",
			strings.TrimSpace(name), parentID, raw).Scan(&id)
	})
	if isUniqueViolation(err) {
		return Collection{}, ErrConflict
	}
	if err != nil {
		return Collection{}, err
	}
	return s.GetCollection(ctx, id)
}

type CollectionUpdate struct {
	Name *string
	// ParentSet marks Parent as meant: nil Parent with ParentSet moves it to the top level.
	ParentSet bool
	Parent    *int64
	Rules     *Rules
}

func (s *Store) UpdateCollection(ctx context.Context, id int64, u CollectionUpdate) (Collection, error) {
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		cur, err := q.GetCollection(ctx, id)
		if err != nil {
			return err
		}
		if u.Name != nil {
			n := strings.TrimSpace(*u.Name)
			if _, err := q.db.Exec(ctx, "UPDATE collections SET name = $2 WHERE id = $1", id, n); err != nil {
				return err
			}
		}
		if u.ParentSet {
			if u.Parent != nil {
				if _, err := q.GetCollection(ctx, *u.Parent); err != nil {
					return err
				}
				var cycle bool
				if err := q.db.QueryRow(ctx, `
					WITH RECURSIVE sub AS (
						SELECT id FROM collections WHERE id = $1
						UNION
						SELECT c.id FROM collections c JOIN sub ON c.parent_id = sub.id
					) SELECT EXISTS (SELECT 1 FROM sub WHERE id = $2)`, id, *u.Parent).Scan(&cycle); err != nil {
					return err
				}
				if cycle {
					return ErrConflict
				}
			}
			if _, err := q.db.Exec(ctx, "UPDATE collections SET parent_id = $2 WHERE id = $1", id, u.Parent); err != nil {
				return err
			}
		}
		if u.Rules != nil {
			if cur.Rules == nil {
				return ErrConflict // a hand-filled collection can't become smart (or the reverse)
			}
			raw, _ := json.Marshal(u.Rules)
			if _, err := q.db.Exec(ctx, "UPDATE collections SET rules = $2 WHERE id = $1", id, raw); err != nil {
				return err
			}
		}
		return nil
	})
	if isUniqueViolation(err) {
		return Collection{}, ErrConflict
	}
	if err != nil {
		return Collection{}, err
	}
	return s.GetCollection(ctx, id)
}

// DeleteCollection removes a collection and hands the ones nested in it to its parent.
// The files themselves are untouched.
func (s *Store) DeleteCollection(ctx context.Context, id int64) error {
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		c, err := q.GetCollection(ctx, id)
		if err != nil {
			return err
		}
		if _, err := q.db.Exec(ctx, "UPDATE collections SET parent_id = $2 WHERE parent_id = $1", id, c.ParentID); err != nil {
			return err
		}
		_, err = q.db.Exec(ctx, "DELETE FROM collections WHERE id = $1", id)
		return err
	})
	if isUniqueViolation(err) {
		return ErrConflict // a nested collection would clash with one next to the deleted one
	}
	return err
}

func (s *Store) AddToCollection(ctx context.Context, id int64, fileIDs []int64) (int64, error) {
	var added int64
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		c, err := q.GetCollection(ctx, id)
		if err != nil {
			return err
		}
		if c.Rules != nil {
			return ErrSmartCollection
		}
		tag, err := q.db.Exec(ctx, `
			INSERT INTO collection_files (collection_id, file_id)
			SELECT $1, f.id FROM files f WHERE f.id = ANY($2)
			ON CONFLICT DO NOTHING`, id, fileIDs)
		added = tag.RowsAffected()
		return err
	})
	return added, err
}

func (s *Store) RemoveFromCollection(ctx context.Context, id int64, fileIDs []int64) error {
	c, err := s.GetCollection(ctx, id)
	if err != nil {
		return err
	}
	if c.Rules != nil {
		return ErrSmartCollection
	}
	_, err = s.db.Exec(ctx, "DELETE FROM collection_files WHERE collection_id = $1 AND file_id = ANY($2)", id, fileIDs)
	return err
}

// CollectionRef is a collection a file belongs to.
type CollectionRef struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Smart bool   `json:"smart"`
}

// FileCollections lists the collections holding a file: the ones it was added to, and
// the smart ones whose rules it matches.
func (s *Store) FileCollections(ctx context.Context, fileID int64) ([]CollectionRef, error) {
	out := []CollectionRef{}
	rows, err := s.db.Query(ctx, `
		SELECT c.id, c.name FROM collection_files cf JOIN collections c ON c.id = cf.collection_id
		WHERE cf.file_id = $1 ORDER BY lower(c.name)`, fileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r CollectionRef
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	all, err := s.ListSmart(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range all {
		q := &queryBuilder{}
		q.add("f.id = " + q.arg(fileID))
		q.add(ruleCond(q, *c.Rules))
		var ok bool
		if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM files f WHERE "+q.sql()+")", q.args...).Scan(&ok); err != nil {
			return nil, err
		}
		if ok {
			out = append(out, CollectionRef{ID: c.ID, Name: c.Name, Smart: true})
		}
	}
	return out, nil
}

// ListSmart returns the smart collections, without counting their files.
func (s *Store) ListSmart(ctx context.Context) ([]Collection, error) {
	rows, err := s.db.Query(ctx,
		"SELECT id, parent_id, name, rules, created_at FROM collections WHERE rules IS NOT NULL ORDER BY lower(name)")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		if c.Rules != nil {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// collectionScopeOf finds a collection and everything nested in it.
func (s *Store) collectionScopeOf(ctx context.Context, id int64) (*collectionScope, error) {
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id, rules FROM collections WHERE id = $1
			UNION
			SELECT c.id, c.rules FROM collections c JOIN sub ON c.parent_id = sub.id
		) SELECT id, rules FROM sub`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scope := &collectionScope{}
	for rows.Next() {
		var cid int64
		var raw []byte
		if err := rows.Scan(&cid, &raw); err != nil {
			return nil, err
		}
		scope.ids = append(scope.ids, cid)
		if raw != nil {
			var r Rules
			if json.Unmarshal(raw, &r) == nil {
				scope.smart = append(scope.smart, r)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(scope.ids) == 0 {
		return nil, ErrNotFound
	}
	return scope, nil
}

// replaceTagInRules rewrites smart collections that mention a tag that is being deleted
// (with = 0) or merged into another.
func (s *Store) replaceTagInRules(ctx context.Context, tagID, with int64) error {
	smart, err := s.ListSmart(ctx)
	if err != nil {
		return err
	}
	swap := func(ids []int64) ([]int64, bool) {
		changed := false
		var out []int64
		for _, id := range ids {
			if id != tagID {
				out = append(out, id)
				continue
			}
			changed = true
			if with != 0 {
				out = append(out, with)
			}
		}
		return uniqueIDs(out), changed
	}
	for _, c := range smart {
		r := *c.Rules
		var a, b bool
		r.Tags, a = swap(r.Tags)
		r.NotTags, b = swap(r.NotTags)
		if !a && !b {
			continue
		}
		raw, _ := json.Marshal(r)
		if _, err := s.db.Exec(ctx, "UPDATE collections SET rules = $2 WHERE id = $1", c.ID, raw); err != nil {
			return err
		}
	}
	return nil
}

// countRules counts the present files a smart collection matches.
func (s *Store) countRules(ctx context.Context, r Rules) (int64, error) {
	if r.Empty() {
		return 0, nil
	}
	return s.CountFiles(ctx, r.filter())
}
