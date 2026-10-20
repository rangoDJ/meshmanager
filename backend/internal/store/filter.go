package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// FileFilter selects files for listing, counting, bulk actions and smart collections.
type FileFilter struct {
	SourceID int64
	// Dir limits results to a folder and its subfolders; requires SourceID. Empty means all.
	Dir     string
	Kinds   []string
	Formats []string
	Query   string
	Sort    string // "name", "-mtime" (default), "-size"
	Cursor  string
	Limit   int

	// Tags: files must carry all of these, or any of them when TagAny is set.
	Tags   []int64
	TagAny bool
	// NotTags excludes files carrying any of these.
	NotTags []int64
	// Collection limits results to a collection and everything nested in it. A smart
	// collection contributes whatever its rules match.
	Collection int64
	// Unfiled limits results to files in no collection, hand-filled or smart.
	Unfiled bool

	scope *collectionScope
}

// collectionScope is a collection's contents worked out ahead of building SQL.
type collectionScope struct {
	ids   []int64 // the collection and the ones nested in it
	smart []Rules // rules of the smart ones among them
	all   []Rules // rules of every smart collection (for Unfiled)
}

// Rules is the saved filter of a smart collection.
type Rules struct {
	Tags     []int64  `json:"tags,omitempty"`
	TagAny   bool     `json:"tag_any,omitempty"`
	NotTags  []int64  `json:"not_tags,omitempty"`
	Query    string   `json:"q,omitempty"`
	Kinds    []string `json:"kinds,omitempty"`
	Formats  []string `json:"formats,omitempty"`
	SourceID int64    `json:"source_id,omitempty"`
	Dir      string   `json:"dir,omitempty"`
}

// Empty is true when the rules would match every file, which a smart collection must not.
func (r Rules) Empty() bool {
	return len(r.Tags) == 0 && len(r.NotTags) == 0 && strings.TrimSpace(r.Query) == "" &&
		len(r.Kinds) == 0 && len(r.Formats) == 0 && r.SourceID == 0
}

func (r Rules) filter() FileFilter {
	f := FileFilter{
		SourceID: r.SourceID, Kinds: r.Kinds, Formats: r.Formats, Query: r.Query,
		Tags: r.Tags, TagAny: r.TagAny, NotTags: r.NotTags,
	}
	if r.SourceID > 0 {
		f.Dir = r.Dir
	}
	return f
}

type queryBuilder struct {
	where []string
	args  []any
}

func (q *queryBuilder) arg(v any) string {
	q.args = append(q.args, v)
	return fmt.Sprintf("$%d", len(q.args))
}

func (q *queryBuilder) add(cond string) { q.where = append(q.where, cond) }

func (q *queryBuilder) sql() string { return strings.Join(q.where, " AND ") }

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// conds returns the conditions for everything but a file's presence.
func (f FileFilter) conds(q *queryBuilder, withKinds bool) []string {
	var c []string
	if f.SourceID > 0 {
		c = append(c, "f.source_id = "+q.arg(f.SourceID))
		if f.Dir != "" {
			c = append(c, fmt.Sprintf("(f.dir = %s OR f.dir LIKE %s)", q.arg(f.Dir), q.arg(escapeLike(f.Dir)+"/%")))
		}
	}
	if withKinds {
		if len(f.Kinds) > 0 {
			c = append(c, "f.kind = ANY("+q.arg(f.Kinds)+")")
		}
		if len(f.Formats) > 0 {
			c = append(c, "f.format = ANY("+q.arg(f.Formats)+")")
		}
	}
	for _, term := range strings.Fields(f.Query) {
		like := q.arg("%" + escapeLike(term) + "%")
		c = append(c, fmt.Sprintf(`(f.rel_path ILIKE %[1]s OR EXISTS (
			SELECT 1 FROM file_tags ft JOIN tags t ON t.id = ft.tag_id
			WHERE ft.file_id = f.id AND t.name ILIKE %[1]s))`, like))
	}
	if len(f.Tags) > 0 {
		ids := q.arg(f.Tags)
		if f.TagAny {
			c = append(c, "EXISTS (SELECT 1 FROM file_tags ft WHERE ft.file_id = f.id AND ft.tag_id = ANY("+ids+"))")
		} else {
			c = append(c, fmt.Sprintf(
				"(SELECT count(*) FROM file_tags ft WHERE ft.file_id = f.id AND ft.tag_id = ANY(%s)) = %d",
				ids, len(uniqueIDs(f.Tags))))
		}
	}
	if len(f.NotTags) > 0 {
		c = append(c, "NOT EXISTS (SELECT 1 FROM file_tags ft WHERE ft.file_id = f.id AND ft.tag_id = ANY("+q.arg(f.NotTags)+"))")
	}
	if f.Collection > 0 && f.scope != nil {
		parts := []string{}
		if len(f.scope.ids) > 0 {
			parts = append(parts, "EXISTS (SELECT 1 FROM collection_files cf WHERE cf.file_id = f.id AND cf.collection_id = ANY("+q.arg(f.scope.ids)+"))")
		}
		for _, r := range f.scope.smart {
			parts = append(parts, ruleCond(q, r))
		}
		if len(parts) == 0 {
			parts = append(parts, "FALSE")
		}
		c = append(c, "("+strings.Join(parts, " OR ")+")")
	}
	if f.Unfiled {
		c = append(c, "NOT EXISTS (SELECT 1 FROM collection_files cf WHERE cf.file_id = f.id)")
		if f.scope != nil {
			for _, r := range f.scope.all {
				c = append(c, "NOT "+ruleCond(q, r))
			}
		}
	}
	return c
}

// ruleCond is a smart collection's rules as one parenthesised condition.
func ruleCond(q *queryBuilder, r Rules) string {
	if r.Empty() {
		return "(FALSE)"
	}
	return "(" + strings.Join(r.filter().conds(q, true), " AND ") + ")"
}

func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := ids[:0:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// filters builds the WHERE clause shared by listing, counting and facets. The filter
// must have been through resolve first when it names a collection or asks for unfiled.
func (f FileFilter) filters(withKinds bool) *queryBuilder {
	q := &queryBuilder{}
	q.add("f.status = 'present'")
	for _, c := range f.conds(q, withKinds) {
		q.add(c)
	}
	return q
}

// resolve looks up what the filter's collection scope needs before SQL can be built.
func (s *Store) resolve(ctx context.Context, f FileFilter) (FileFilter, error) {
	if f.Collection > 0 {
		scope, err := s.collectionScopeOf(ctx, f.Collection)
		if err != nil {
			return f, err
		}
		f.scope = scope
	}
	if f.Unfiled {
		if f.scope == nil {
			f.scope = &collectionScope{}
		}
		all, err := s.smartRules(ctx)
		if err != nil {
			return f, err
		}
		f.scope.all = all
	}
	return f, nil
}

func (s *Store) smartRules(ctx context.Context) ([]Rules, error) {
	rows, err := s.db.Query(ctx, "SELECT rules FROM collections WHERE rules IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rules
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r Rules
		if json.Unmarshal(raw, &r) == nil {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

// FileIDs returns the ids of up to limit+1 files matching the filter, so callers can tell
// when a bulk action would touch more than they allow.
func (s *Store) FileIDs(ctx context.Context, f FileFilter, limit int) ([]int64, error) {
	f, err := s.resolve(ctx, f)
	if err != nil {
		return nil, err
	}
	q := f.filters(true)
	rows, err := s.db.Query(ctx, "SELECT f.id FROM files f WHERE "+q.sql()+" ORDER BY f.id LIMIT "+q.arg(limit+1), q.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
