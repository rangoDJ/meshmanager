package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// organizeEnv is a small scanned library plus a signed-in client.
type organizeEnv struct {
	t       *testing.T
	c       *client
	lib     string
	srcID   any
	sid     string
	dataDir string
}

func newOrganizeEnv(t *testing.T) *organizeEnv {
	t.Helper()
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)

	lib := filepath.Join(cfg.LibraryRoot, "lib")
	writeLibFile(t, lib, "Toys/dragon.stl", cubeSTL(10))
	writeLibFile(t, lib, "Toys/knight.stl", cubeSTL(11))
	writeLibFile(t, lib, "Functional/bracket.obj", []byte(cubeOBJ))
	writeLibFile(t, lib, "Functional/hinge.stl", cubeSTL(12))
	writeLibFile(t, lib, "Photos/print.png", pngBytes(64, 64))

	_, src := c.do("POST", "/api/sources", map[string]any{"path": "lib"})
	waitForScan(t, c, src["id"])
	return &organizeEnv{t: t, c: c, lib: lib, srcID: src["id"], sid: fmt.Sprint(src["id"]), dataDir: cfg.DataDir}
}

func (e *organizeEnv) rescan() map[string]any {
	e.t.Helper()
	if status, body := e.c.do("POST", "/api/sources/"+e.sid+"/scan", nil); status != 202 {
		e.t.Fatalf("rescan: %d %v", status, body)
	}
	return waitForScan(e.t, e.c, e.srcID)
}

// id returns the id of the present file with this name.
func (e *organizeEnv) id(name string) int64 {
	e.t.Helper()
	items, _ := listFiles(e.t, e.c, "q="+name+"&limit=100")
	for _, it := range items {
		if it["name"] == name {
			return int64(it["id"].(float64))
		}
	}
	e.t.Fatalf("no file named %s", name)
	return 0
}

func (e *organizeEnv) ok(method, path string, body any) map[string]any {
	e.t.Helper()
	status, out := e.c.do(method, path, body)
	if status < 200 || status > 299 {
		e.t.Fatalf("%s %s: %d %v", method, path, status, out)
	}
	return out
}

func (e *organizeEnv) expect(want int, method, path string, body any) map[string]any {
	e.t.Helper()
	status, out := e.c.do(method, path, body)
	if status != want {
		e.t.Fatalf("%s %s: got %d, want %d (%v)", method, path, status, want, out)
	}
	return out
}

func (e *organizeEnv) tagID(name string) int64 {
	e.t.Helper()
	_, out := e.c.do("GET", "/api/tags", nil)
	for _, it := range out["items"].([]any) {
		tag := it.(map[string]any)
		if tag["name"] == name {
			return int64(tag["id"].(float64))
		}
	}
	e.t.Fatalf("no tag %q", name)
	return 0
}

func (e *organizeEnv) sortedNames(query string) string {
	e.t.Helper()
	items, _ := listFiles(e.t, e.c, query)
	var n []string
	for _, it := range items {
		n = append(n, it["name"].(string))
	}
	sort.Strings(n)
	return strings.Join(n, ",")
}

func TestTagsAndTagFilters(t *testing.T) {
	e := newOrganizeEnv(t)
	dragon, knight, bracket, hinge := e.id("dragon.stl"), e.id("knight.stl"), e.id("bracket.obj"), e.id("hinge.stl")

	e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{dragon, knight}, "add_names": []string{"Fantasy", "Painted"}})
	e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{bracket, hinge}, "add_names": []string{"Functional"}})
	fantasy, painted, functional := e.tagID("Fantasy"), e.tagID("Painted"), e.tagID("Functional")

	// Tags come with counts and a colour each, and the colours differ.
	_, list := e.c.do("GET", "/api/tags", nil)
	colors := map[string]bool{}
	for _, it := range list["items"].([]any) {
		tag := it.(map[string]any)
		colors[tag["color"].(string)] = true
		want := 2.0
		if tag["files"] != want {
			t.Errorf("tag %v has %v files, want 2", tag["name"], tag["files"])
		}
	}
	if len(colors) != 3 {
		t.Errorf("auto colours should differ, got %v", colors)
	}

	// Tags show up on files in the grid.
	items, _ := listFiles(t, e.c, "q=dragon")
	tags := items[0]["tags"].([]any)
	if len(tags) != 2 {
		t.Fatalf("dragon tags: %v", tags)
	}

	// Filtering: all (default), any, and exclusion.
	e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{knight}, "remove_ids": []int64{painted}})
	q := func(parts ...string) string { return strings.Join(parts, "&") }
	fid, pid := fmt.Sprint(fantasy), fmt.Sprint(painted)
	if got := e.sortedNames(q("tag=" + fid)); got != "dragon.stl,knight.stl" {
		t.Errorf("one tag: %s", got)
	}
	if got := e.sortedNames(q("tag="+fid, "tag="+pid)); got != "dragon.stl" {
		t.Errorf("all of two tags: %s", got)
	}
	if got := e.sortedNames(q("tag="+fid, "tag="+pid, "tags=any")); got != "dragon.stl,knight.stl" {
		t.Errorf("any of two tags: %s", got)
	}
	if got := e.sortedNames(q("tag="+fid, "notag="+pid)); got != "knight.stl" {
		t.Errorf("excluding a tag: %s", got)
	}
	if got := e.sortedNames(q("notag=" + fid + "&kind=model")); got != "bracket.obj,hinge.stl" {
		t.Errorf("excluding only: %s", got)
	}
	// Facet counts follow the tag filter too.
	_, facets := e.c.do("GET", "/api/files/facets?tag="+fid, nil)
	total := 0.0
	for _, f := range facets["items"].([]any) {
		total += f.(map[string]any)["count"].(float64)
	}
	if total != 2 {
		t.Errorf("facet total with tag filter: %v", total)
	}
	// The search box also finds tag names, not just paths.
	if got := e.sortedNames("q=fantasy"); got != "dragon.stl,knight.stl" {
		t.Errorf("search by tag name: %s", got)
	}

	// Managing tags.
	e.expect(409, "PATCH", fmt.Sprintf("/api/tags/%d", painted), map[string]any{"name": "fantasy"})
	e.expect(400, "PATCH", fmt.Sprintf("/api/tags/%d", painted), map[string]any{"color": "chartreuse"})
	e.expect(409, "POST", "/api/tags", map[string]any{"name": "FANTASY"})
	e.expect(400, "POST", "/api/tags", map[string]any{"name": "  "})
	out := e.ok("PATCH", fmt.Sprintf("/api/tags/%d", fantasy), map[string]any{"pinned": true, "color": "violet", "name": "Fantasy art"})
	if out["pinned"] != true || out["color"] != "violet" || out["name"] != "Fantasy art" {
		t.Errorf("updated tag: %v", out)
	}
	items, _ = listFiles(t, e.c, "q=dragon")
	if first := items[0]["tags"].([]any)[0].(map[string]any); first["pinned"] != true || first["name"] != "Fantasy art" {
		t.Errorf("pinned tag should lead the file's tags: %v", items[0]["tags"])
	}

	// Merging moves the files over and removes the tag.
	e.ok("POST", fmt.Sprintf("/api/tags/%d/merge", painted), map[string]any{"into": fantasy})
	e.expect(400, "POST", fmt.Sprintf("/api/tags/%d/merge", fantasy), map[string]any{"into": fantasy})
	if got := e.sortedNames("tag=" + fid); got != "dragon.stl,knight.stl" {
		t.Errorf("after merge: %s", got)
	}
	e.expect(404, "PATCH", fmt.Sprintf("/api/tags/%d", painted), map[string]any{"name": "gone"})

	// Bulk tagging by filter: every model in a folder.
	res := e.ok("POST", "/api/bulk/tags", map[string]any{
		"filter": map[string]any{"source": e.srcID, "dir": "Toys", "kinds": []string{"model"}}, "add_names": []string{"Toys"},
	})
	if res["files"] != 2.0 {
		t.Errorf("filter selection touched %v files, want 2", res["files"])
	}
	e.expect(400, "POST", "/api/bulk/tags", map[string]any{"add_names": []string{"X"}}) // nothing selected
	e.expect(400, "POST", "/api/bulk/tags", map[string]any{"ids": []int64{dragon}})     // nothing to change

	// Deleting a tag takes it off every file.
	e.expect(204, "DELETE", fmt.Sprintf("/api/tags/%d", functional), nil)
	items, _ = listFiles(t, e.c, "q=bracket")
	if tags := items[0]["tags"].([]any); len(tags) != 0 {
		t.Errorf("bracket kept a deleted tag: %v", tags)
	}
}

func TestNotesAndProvenance(t *testing.T) {
	e := newOrganizeEnv(t)
	id := e.id("dragon.stl")
	path := fmt.Sprintf("/api/files/%d", id)

	detail := e.ok("GET", path, nil)
	if m := detail["meta"].(map[string]any); m["notes"] != "" || m["designer"] != "" {
		t.Errorf("a fresh file has no meta: %v", m)
	}

	e.ok("PATCH", path+"/meta", map[string]any{
		"notes": "print at 0.12 with tree supports", "designer": " Jane Maker ", "license": "CC BY-NC", "source_url": "https://example.com/dragon",
	})
	m := e.ok("GET", path, nil)["meta"].(map[string]any)
	if m["notes"] != "print at 0.12 with tree supports" || m["designer"] != "Jane Maker" || m["license"] != "CC BY-NC" ||
		m["source_url"] != "https://example.com/dragon" {
		t.Errorf("saved meta: %v", m)
	}

	// Only the fields sent change.
	e.ok("PATCH", path+"/meta", map[string]any{"notes": "second thoughts"})
	m = e.ok("GET", path, nil)["meta"].(map[string]any)
	if m["notes"] != "second thoughts" || m["designer"] != "Jane Maker" {
		t.Errorf("partial update: %v", m)
	}

	// The source URL becomes a link, so anything but a web address is refused.
	for _, bad := range []string{"javascript:alert(1)", "ftp://example.com/x", "not a url", "https://"} {
		e.expect(400, "PATCH", path+"/meta", map[string]any{"source_url": bad})
	}
	e.expect(400, "PATCH", path+"/meta", map[string]any{"notes": strings.Repeat("x", 10001)})
	e.expect(404, "PATCH", "/api/files/999999/meta", map[string]any{"notes": "x"})

	// Clearing everything removes the record.
	e.ok("PATCH", path+"/meta", map[string]any{"notes": "", "designer": "", "license": "", "source_url": ""})
	var rows int
	if err := testPool.QueryRow(t.Context(), "SELECT count(*) FROM file_meta").Scan(&rows); err != nil || rows != 0 {
		t.Errorf("file_meta rows after clearing: %d %v", rows, err)
	}
}

func TestCollections(t *testing.T) {
	e := newOrganizeEnv(t)
	dragon, knight, bracket := e.id("dragon.stl"), e.id("knight.stl"), e.id("bracket.obj")

	games := e.ok("POST", "/api/collections", map[string]any{"name": "Tabletop"})
	gid := int64(games["id"].(float64))
	minis := e.ok("POST", "/api/collections", map[string]any{"name": "Minis", "parent_id": gid})
	mid := int64(minis["id"].(float64))
	e.expect(409, "POST", "/api/collections", map[string]any{"name": "minis", "parent_id": gid}) // sibling clash
	e.ok("POST", "/api/collections", map[string]any{"name": "Minis"})                            // same name elsewhere is fine
	e.expect(404, "POST", "/api/collections", map[string]any{"name": "Orphan", "parent_id": 99999})
	e.expect(400, "POST", "/api/collections", map[string]any{"name": ""})

	e.ok("POST", fmt.Sprintf("/api/collections/%d/files", mid), map[string]any{"ids": []int64{dragon, knight}})
	e.ok("POST", fmt.Sprintf("/api/collections/%d/files", gid), map[string]any{"ids": []int64{bracket}})

	// A collection includes everything nested in it.
	if got := e.sortedNames(fmt.Sprintf("collection=%d", mid)); got != "dragon.stl,knight.stl" {
		t.Errorf("minis: %s", got)
	}
	if got := e.sortedNames(fmt.Sprintf("collection=%d", gid)); got != "bracket.obj,dragon.stl,knight.stl" {
		t.Errorf("tabletop with nested: %s", got)
	}
	e.expect(404, "GET", "/api/files?collection=99999", nil)

	// Counts count a collection's own files; the file panel lists its collections.
	_, cols := e.c.do("GET", "/api/collections", nil)
	counts := map[string]float64{}
	for _, it := range cols["items"].([]any) {
		c := it.(map[string]any)
		if c["parent_id"] != nil {
			counts[c["name"].(string)] = c["files"].(float64)
		} else if c["name"] == "Tabletop" {
			counts["Tabletop"] = c["files"].(float64)
		}
	}
	if counts["Minis"] != 2 || counts["Tabletop"] != 1 {
		t.Errorf("counts: %v", counts)
	}
	detail := e.ok("GET", fmt.Sprintf("/api/files/%d", dragon), nil)
	if cs := detail["collections"].([]any); len(cs) != 1 || cs[0].(map[string]any)["name"] != "Minis" {
		t.Errorf("dragon's collections: %v", detail["collections"])
	}

	// Smart collection: whatever carries the tag, with nothing added by hand.
	e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{bracket, e.id("hinge.stl")}, "add_names": []string{"Functional"}})
	tag := e.tagID("Functional")
	smart := e.ok("POST", "/api/collections", map[string]any{"name": "Hardware", "rules": map[string]any{"tags": []int64{tag}}})
	sid := int64(smart["id"].(float64))
	if smart["files"] != 2.0 || smart["rules"] == nil {
		t.Errorf("smart collection: %v", smart)
	}
	if got := e.sortedNames(fmt.Sprintf("collection=%d", sid)); got != "bracket.obj,hinge.stl" {
		t.Errorf("smart members: %s", got)
	}
	e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{knight}, "add_ids": []int64{tag}})
	if got := e.sortedNames(fmt.Sprintf("collection=%d", sid)); got != "bracket.obj,hinge.stl,knight.stl" {
		t.Errorf("smart collection should fill itself: %s", got)
	}
	e.expect(409, "POST", fmt.Sprintf("/api/collections/%d/files", sid), map[string]any{"ids": []int64{dragon}})
	e.expect(400, "POST", "/api/collections", map[string]any{"name": "Empty rules", "rules": map[string]any{}})
	e.expect(400, "POST", "/api/collections", map[string]any{"name": "Blank search", "rules": map[string]any{"q": "  "}})
	detail = e.ok("GET", fmt.Sprintf("/api/files/%d", bracket), nil)
	var smartRef bool
	for _, c := range detail["collections"].([]any) {
		if m := c.(map[string]any); m["name"] == "Hardware" && m["smart"] == true {
			smartRef = true
		}
	}
	if !smartRef {
		t.Errorf("a file should list the smart collection it matches: %v", detail["collections"])
	}
	e.ok("PATCH", fmt.Sprintf("/api/collections/%d", sid), map[string]any{"rules": map[string]any{"tags": []int64{tag}, "kinds": []string{"model"}, "q": "hinge"}})
	if got := e.sortedNames(fmt.Sprintf("collection=%d", sid)); got != "hinge.stl" {
		t.Errorf("edited rules: %s", got)
	}

	// Not in a collection: files that no collection, hand-filled or smart, holds.
	if got := e.sortedNames("unfiled=1"); got != "print.png" {
		t.Errorf("unfiled (everything else is in a collection): %s", got)
	}
	e.ok("POST", fmt.Sprintf("/api/collections/%d/files/remove", mid), map[string]any{"ids": []int64{knight}})
	if got := e.sortedNames("unfiled=1"); got != "knight.stl,print.png" {
		t.Errorf("unfiled after removing knight from Minis: %s", got)
	}

	// Moving: no cycles, and the top level is reachable with null.
	e.expect(409, "PATCH", fmt.Sprintf("/api/collections/%d", gid), map[string]any{"parent_id": mid})
	e.expect(409, "PATCH", fmt.Sprintf("/api/collections/%d", gid), map[string]any{"parent_id": gid})
	// A top-level "Minis" already exists, so moving there clashes until it's renamed.
	e.expect(409, "PATCH", fmt.Sprintf("/api/collections/%d", mid), map[string]any{"parent_id": nil})
	moved := e.ok("PATCH", fmt.Sprintf("/api/collections/%d", mid), map[string]any{"name": "Miniatures", "parent_id": nil})
	if moved["parent_id"] != nil || moved["name"] != "Miniatures" {
		t.Errorf("moved to top level: %v", moved)
	}
	e.ok("PATCH", fmt.Sprintf("/api/collections/%d", mid), map[string]any{"parent_id": gid})

	// Deleting hands nested collections to the parent and leaves the files alone.
	e.expect(204, "DELETE", fmt.Sprintf("/api/collections/%d", gid), nil)
	_, cols = e.c.do("GET", "/api/collections", nil)
	for _, it := range cols["items"].([]any) {
		c := it.(map[string]any)
		if c["name"] == "Miniatures" && c["parent_id"] != nil {
			t.Errorf("nested collection should move up: %v", c)
		}
	}
	if got := e.sortedNames("q=dragon"); got != "dragon.stl" {
		t.Errorf("files must survive: %s", got)
	}

	// Deleting a tag that a smart collection uses drops it from the rules.
	e.expect(204, "DELETE", fmt.Sprintf("/api/tags/%d", tag), nil)
	_, cols = e.c.do("GET", "/api/collections", nil)
	for _, it := range cols["items"].([]any) {
		if c := it.(map[string]any); c["name"] == "Hardware" {
			if tags, _ := c["rules"].(map[string]any)["tags"].([]any); len(tags) != 0 {
				t.Errorf("deleted tag still in rules: %v", c["rules"])
			}
		}
	}
}

func TestPrintQueue(t *testing.T) {
	e := newOrganizeEnv(t)
	dragon, knight, hinge := e.id("dragon.stl"), e.id("knight.stl"), e.id("hinge.stl")

	list := e.ok("POST", "/api/print-lists", map[string]any{"name": "Weekend", "printer": "X1C"})
	lid := int64(list["id"].(float64))
	other := e.ok("POST", "/api/print-lists", map[string]any{"name": "Gifts"})
	oid := int64(other["id"].(float64))
	e.expect(409, "POST", "/api/print-lists", map[string]any{"name": "weekend"})
	e.expect(400, "POST", "/api/print-lists", map[string]any{"name": " "})

	e.ok("POST", fmt.Sprintf("/api/print-lists/%d/items", lid), map[string]any{"ids": []int64{knight, dragon, hinge}, "quantity": 2})
	view := e.ok("GET", fmt.Sprintf("/api/print-lists/%d/items", lid), nil)
	items := view["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items: %v", items)
	}
	// Queue order is the order of adding, sorted by name within one add.
	first := items[0].(map[string]any)
	if first["file"].(map[string]any)["name"] != "dragon.stl" || first["quantity"] != 2.0 || first["status"] != "todo" {
		t.Errorf("first item: %v", first)
	}
	if l := view["list"].(map[string]any); l["todo"] != 3.0 || l["printer"] != "X1C" {
		t.Errorf("list counts: %v", l)
	}

	dragonItem := int64(first["id"].(float64))
	e.ok("PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"status": "printing", "note": "0.2 draft first"})
	e.ok("PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"status": "done", "quantity": 3})
	view = e.ok("GET", fmt.Sprintf("/api/print-lists/%d/items", lid), nil)
	got := view["items"].([]any)[0].(map[string]any)
	if got["status"] != "done" || got["finished_at"] == nil || got["quantity"] != 3.0 || got["note"] != "0.2 draft first" {
		t.Errorf("updated item: %v", got)
	}
	e.ok("PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"status": "todo"})
	view = e.ok("GET", fmt.Sprintf("/api/print-lists/%d/items", lid), nil)
	if view["items"].([]any)[0].(map[string]any)["finished_at"] != nil {
		t.Errorf("finished_at should clear when an item goes back to todo")
	}
	e.expect(400, "PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"status": "burnt"})
	e.expect(400, "PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"quantity": 0})
	e.expect(404, "PATCH", "/api/print-items/99999", map[string]any{"status": "done"})

	// Adding a done model again puts it back in the queue; a waiting one is left alone.
	e.ok("PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"status": "done"})
	e.ok("POST", fmt.Sprintf("/api/print-lists/%d/items", lid), map[string]any{"ids": []int64{dragon, knight}})
	view = e.ok("GET", fmt.Sprintf("/api/print-lists/%d/items", lid), nil)
	if len(view["items"].([]any)) != 3 || view["items"].([]any)[0].(map[string]any)["status"] != "todo" {
		t.Errorf("re-adding a done item: %v", view["items"])
	}

	// The file panel says where a model is queued.
	detail := e.ok("GET", fmt.Sprintf("/api/files/%d", dragon), nil)
	if q := detail["queue"].([]any); len(q) != 1 || q[0].(map[string]any)["list_name"] != "Weekend" {
		t.Errorf("queue refs: %v", detail["queue"])
	}

	// Moving an item to another list, and the clash when it's already there.
	e.ok("PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"list_id": oid})
	e.ok("POST", fmt.Sprintf("/api/print-lists/%d/items", lid), map[string]any{"ids": []int64{dragon}})
	e.expect(409, "PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"list_id": lid})

	// Clearing finished items.
	e.ok("PATCH", fmt.Sprintf("/api/print-items/%d", dragonItem), map[string]any{"status": "done"})
	cleared := e.ok("POST", fmt.Sprintf("/api/print-lists/%d/clear-done", oid), nil)
	if cleared["removed"] != 1.0 {
		t.Errorf("clear done: %v", cleared)
	}

	// CSV export, with spreadsheet formulas defused.
	e.ok("PATCH", fmt.Sprintf("/api/print-items/%d", int64(view["items"].([]any)[1].(map[string]any)["id"].(float64))), map[string]any{"note": "=HYPERLINK(\"http://evil\")"})
	status, h, body := e.c.raw(fmt.Sprintf("/api/print-lists/%d/export.csv", lid))
	if status != 200 || !strings.HasPrefix(h.Get("Content-Type"), "text/csv") || !strings.Contains(h.Get("Content-Disposition"), "Weekend.csv") {
		t.Fatalf("export: %d %v", status, h)
	}
	csv := string(bytes.TrimPrefix(body, []byte("\xEF\xBB\xBF")))
	if !strings.HasPrefix(csv, "Order,File,Quantity,Status,Format,Size (mm),Location,Note") {
		t.Errorf("csv header: %q", csv)
	}
	if strings.Contains(csv, ",=HYPERLINK") || !strings.Contains(csv, "'=HYPERLINK") {
		t.Errorf("csv formula not defused: %q", csv)
	}

	// Renaming and deleting lists; deleting takes the items with it.
	renamed := e.ok("PATCH", fmt.Sprintf("/api/print-lists/%d", lid), map[string]any{"name": "Weekend run", "printer": "P1S"})
	if renamed["printer"] != "P1S" {
		t.Errorf("renamed: %v", renamed)
	}
	e.expect(204, "DELETE", fmt.Sprintf("/api/print-lists/%d", lid), nil)
	e.expect(404, "GET", fmt.Sprintf("/api/print-lists/%d/items", lid), nil)
	detail = e.ok("GET", fmt.Sprintf("/api/files/%d", knight), nil)
	if q := detail["queue"].([]any); len(q) != 0 {
		t.Errorf("deleted list left queue entries: %v", q)
	}
}

// TestRenamedFilesKeepTheirOrganization is the milestone's promise: tags, notes and the
// rest survive a rename or move on disk followed by a rescan.
func TestRenamedFilesKeepTheirOrganization(t *testing.T) {
	e := newOrganizeEnv(t)
	dragon, bracket, hinge, png := e.id("dragon.stl"), e.id("bracket.obj"), e.id("hinge.stl"), e.id("print.png")

	list := e.ok("POST", "/api/print-lists", map[string]any{"name": "Queue"})
	lid := int64(list["id"].(float64))
	col := e.ok("POST", "/api/collections", map[string]any{"name": "Keepers"})
	cid := int64(col["id"].(float64))
	for _, id := range []int64{dragon, bracket, hinge, png} {
		e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{id}, "add_names": []string{fmt.Sprintf("tag-%d", id)}})
	}
	e.ok("PATCH", fmt.Sprintf("/api/files/%d/meta", dragon), map[string]any{"notes": "needs supports", "designer": "Jane"})
	e.ok("POST", fmt.Sprintf("/api/collections/%d/files", cid), map[string]any{"ids": []int64{dragon, png}})
	e.ok("POST", fmt.Sprintf("/api/print-lists/%d/items", lid), map[string]any{"ids": []int64{dragon}, "quantity": 4})

	// A model moved into a new folder and renamed; a picture renamed in place;
	// a whole folder renamed.
	if err := os.MkdirAll(filepath.Join(e.lib, "Toys", "Fantasy"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustRename(t, filepath.Join(e.lib, "Toys", "dragon.stl"), filepath.Join(e.lib, "Toys", "Fantasy", "dragon_v2.stl"))
	mustRename(t, filepath.Join(e.lib, "Photos", "print.png"), filepath.Join(e.lib, "Photos", "benchy_photo.png"))
	mustRename(t, filepath.Join(e.lib, "Functional"), filepath.Join(e.lib, "Hardware"))

	src := e.rescan()
	if msg := fmt.Sprint(src["last_scan"].(map[string]any)["message"]); !strings.Contains(msg, "4 moved") {
		t.Errorf("scan summary should report the followed files: %s", msg)
	}

	newDragon := e.id("dragon_v2.stl")
	if newDragon == dragon {
		t.Fatal("the renamed file should be a new row")
	}
	detail := e.ok("GET", fmt.Sprintf("/api/files/%d", newDragon), nil)
	if tags := detail["tags"].([]any); len(tags) != 1 || tags[0].(map[string]any)["name"] != fmt.Sprintf("tag-%d", dragon) {
		t.Errorf("tags after the move: %v", detail["tags"])
	}
	if m := detail["meta"].(map[string]any); m["notes"] != "needs supports" || m["designer"] != "Jane" {
		t.Errorf("notes after the move: %v", m)
	}
	if cs := detail["collections"].([]any); len(cs) != 1 || cs[0].(map[string]any)["name"] != "Keepers" {
		t.Errorf("collections after the move: %v", detail["collections"])
	}
	if q := detail["queue"].([]any); len(q) != 1 || q[0].(map[string]any)["quantity"] != 4.0 {
		t.Errorf("queue after the move: %v", detail["queue"])
	}
	e.expect(404, "GET", fmt.Sprintf("/api/files/%d", dragon), nil) // the old row is gone

	// A picture isn't hashed, so it is followed by size and modification time.
	newPNG := e.id("benchy_photo.png")
	detail = e.ok("GET", fmt.Sprintf("/api/files/%d", newPNG), nil)
	if tags := detail["tags"].([]any); len(tags) != 1 {
		t.Errorf("picture tags after rename: %v", detail["tags"])
	}

	// Files in a renamed folder all follow.
	for _, name := range []string{"bracket.obj", "hinge.stl"} {
		items, _ := listFiles(t, e.c, "q="+name)
		if len(items) != 1 || !strings.HasPrefix(items[0]["rel_path"].(string), "Hardware/") {
			t.Fatalf("%s after the folder rename: %v", name, items)
		}
		if tags := items[0]["tags"].([]any); len(tags) != 1 {
			t.Errorf("%s lost its tag in the folder rename: %v", name, tags)
		}
	}

	// And tag filters find them at their new paths.
	tag := e.tagID(fmt.Sprintf("tag-%d", dragon))
	if got := e.sortedNames(fmt.Sprintf("tag=%d", tag)); got != "dragon_v2.stl" {
		t.Errorf("filter after the move: %s", got)
	}

	// A second scan finds nothing more to follow.
	src = e.rescan()
	if msg := fmt.Sprint(src["last_scan"].(map[string]any)["message"]); strings.Contains(msg, "moved") {
		t.Errorf("nothing moved the second time: %s", msg)
	}
}

func mustRename(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

// upload sends a picture to a file the way the browser does.
func (e *organizeEnv) upload(fileID int64, name string, data []byte) (int, map[string]any) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="photo"; filename="%s"`, name))
	part, _ := mw.CreatePart(hdr)
	_, _ = part.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/files/%d/photos", e.c.base, fileID), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "test")
	resp, err := e.c.http.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestPhotosAndCustomThumbnails(t *testing.T) {
	e := newOrganizeEnv(t)
	dragon := e.id("dragon.stl")

	status, first := e.upload(dragon, "finished.png", noisyPNG(1200, 800))
	if status != 201 || first["is_cover"] != true || first["width"] != 1200.0 {
		t.Fatalf("first upload: %d %v", status, first)
	}
	// The first photo becomes the thumbnail.
	items, _ := listFiles(t, e.c, "q=dragon")
	if items[0]["custom_thumb"] != true || items[0]["thumb"] != "ready" || !strings.Contains(items[0]["thumb_url"].(string), "/api/photos/") {
		t.Errorf("custom thumbnail: %v %v", items[0]["thumb"], items[0]["thumb_url"])
	}
	code, h, body := e.c.raw(items[0]["thumb_url"].(string))
	if code != 200 || !strings.HasPrefix(h.Get("Content-Type"), "image/") || len(body) == 0 {
		t.Errorf("thumb fetch: %d %s", code, h.Get("Content-Type"))
	}
	// The tile copy is smaller than the original.
	full := first["url"].(string)
	_, _, fullBody := e.c.raw(full)
	if len(body) >= len(fullBody) {
		t.Errorf("tile copy (%d bytes) should be smaller than the original (%d)", len(body), len(fullBody))
	}

	status, second := e.upload(dragon, "angle2.png", pngBytes(300, 300))
	if status != 201 || second["is_cover"] != false {
		t.Fatalf("second upload: %d %v", status, second)
	}
	secondID := int64(second["id"].(float64))
	e.ok("PATCH", fmt.Sprintf("/api/photos/%d", secondID), map[string]any{"cover": true})
	detail := e.ok("GET", fmt.Sprintf("/api/files/%d", dragon), nil)
	var covers int
	for _, p := range detail["photos"].([]any) {
		if p.(map[string]any)["is_cover"] == true {
			covers++
		}
	}
	if len(detail["photos"].([]any)) != 2 || covers != 1 {
		t.Errorf("photos: %v", detail["photos"])
	}

	// Refusals: not a picture, wrong kind of picture, no such file.
	if status, _ := e.upload(dragon, "notes.txt", []byte("hello")); status != 415 {
		t.Errorf("text upload: %d", status)
	}
	if status, _ := e.upload(dragon, "x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)); status != 415 {
		t.Errorf("svg upload: %d", status)
	}
	if status, _ := e.upload(99999, "x.png", pngBytes(8, 8)); status != 404 {
		t.Errorf("upload to a missing file: %d", status)
	}

	// Un-covering returns to the generated thumbnail; deleting removes the stored files.
	e.ok("PATCH", fmt.Sprintf("/api/photos/%d", secondID), map[string]any{"cover": false})
	items, _ = listFiles(t, e.c, "q=dragon")
	if items[0]["custom_thumb"] == true {
		t.Errorf("no cover should mean the generated thumbnail: %v", items[0]["thumb_url"])
	}
	before, _ := filepath.Glob(filepath.Join(e.dataDir, "photos", "*"))
	if len(before) != 4 {
		t.Errorf("stored photo files: %d, want 4 (two originals, two tile copies)", len(before))
	}
	e.expect(204, "DELETE", fmt.Sprintf("/api/photos/%d", secondID), nil)
	after, _ := filepath.Glob(filepath.Join(e.dataDir, "photos", "*"))
	if len(after) != 2 {
		t.Errorf("stored photo files after delete: %d, want 2", len(after))
	}
	e.expect(404, "GET", fmt.Sprintf("/api/photos/%d/image", secondID), nil)

	// Removing the source removes the photos stored for it.
	e.expect(204, "DELETE", "/api/sources/"+e.sid, nil)
	left, _ := filepath.Glob(filepath.Join(e.dataDir, "photos", "*"))
	if len(left) != 0 {
		t.Errorf("photos left behind after removing the source: %v", left)
	}
}

func TestFolderCards(t *testing.T) {
	e := newOrganizeEnv(t)
	type folder = map[string]any
	get := func(dir string) []folder {
		_, out := e.c.do("GET", "/api/sources/"+e.sid+"/folders?dir="+dir+"&kind=model&kind=image", nil)
		var fs []folder
		for _, it := range out["items"].([]any) {
			fs = append(fs, it.(map[string]any))
		}
		return fs
	}

	top := get("")
	var paths []string
	for _, f := range top {
		paths = append(paths, f["path"].(string))
	}
	if strings.Join(paths, ",") != "Functional,Photos,Toys" {
		t.Fatalf("top-level folders: %v", paths)
	}
	toys := top[2]
	if toys["files"] != 2.0 || toys["name"] != "Toys" || toys["custom"] != false {
		t.Errorf("Toys card: %v", toys)
	}
	cover := toys["cover"].(map[string]any)
	if cover["name"] != "dragon.stl" { // first model by name
		t.Errorf("automatic cover: %v", cover["name"])
	}

	// A chosen cover wins until it's cleared.
	knight := e.id("knight.stl")
	e.expect(204, "PUT", "/api/sources/"+e.sid+"/folder-cover", map[string]any{"dir": "Toys", "file_id": knight})
	toys = get("")[2]
	if toys["custom"] != true || toys["cover"].(map[string]any)["name"] != "knight.stl" {
		t.Errorf("chosen cover: %v", toys)
	}
	e.expect(404, "PUT", "/api/sources/"+e.sid+"/folder-cover", map[string]any{"dir": "Toys", "file_id": 99999})
	e.expect(204, "DELETE", "/api/sources/"+e.sid+"/folder-cover?dir=Toys", nil)
	if get("")[2]["custom"] != false {
		t.Errorf("cleared cover should return to automatic")
	}

	// Folders inside folders, and nothing for a folder with no subfolders.
	writeLibFile(t, e.lib, "Toys/Minis/orc.stl", cubeSTL(13))
	e.rescan()
	inner := get("Toys")
	if len(inner) != 1 || inner[0]["path"] != "Toys/Minis" || inner[0]["name"] != "Minis" || inner[0]["files"] != 1.0 {
		t.Errorf("subfolders of Toys: %v", inner)
	}
	if got := get("Toys/Minis"); len(got) != 0 {
		t.Errorf("a leaf folder has no subfolders: %v", got)
	}
	e.expect(404, "GET", "/api/sources/99999/folders", nil)
}

// noisyPNG is an opaque, photo-like picture that doesn't compress away.
func noisyPNG(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	seed := uint32(12345)
	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223
			img.SetNRGBA(x, y, color.NRGBA{uint8(seed >> 24), uint8(seed >> 16), uint8(seed >> 8), 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestMoveFollowingEdgeCases(t *testing.T) {
	e := newOrganizeEnv(t)
	dragon := e.id("dragon.stl")
	e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{dragon}, "add_names": []string{"Fantasy"}})

	// A file that is simply deleted keeps its row (and tags) but leaves the listing.
	data := cubeSTL(10)
	mustRemove(t, filepath.Join(e.lib, "Toys", "dragon.stl"))
	src := e.rescan()
	if msg := fmt.Sprint(src["last_scan"].(map[string]any)["message"]); strings.Contains(msg, "moved") {
		t.Errorf("a deletion is not a move: %s", msg)
	}
	if got := e.sortedNames("q=dragon"); got != "" {
		t.Errorf("deleted file still listed: %s", got)
	}

	// If the same content turns up later, even in another folder, the tag comes with it.
	writeLibFile(t, e.lib, "Restored/dragon_back.stl", data)
	src = e.rescan()
	if msg := fmt.Sprint(src["last_scan"].(map[string]any)["message"]); !strings.Contains(msg, "1 moved") {
		t.Errorf("returning content should be followed: %s", msg)
	}
	items, _ := listFiles(t, e.c, "q=dragon_back")
	if len(items) != 1 || len(items[0]["tags"].([]any)) != 1 {
		t.Errorf("restored file: %v", items)
	}

	// Pictures with identical size and time are too ambiguous to guess about.
	same := pngBytes(40, 40)
	stamp := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{"Photos/a.png", "Photos/b.png"} {
		p := writeLibFile(t, e.lib, name, same)
		_ = os.Chtimes(p, stamp, stamp)
	}
	e.rescan()
	a := e.id("a.png")
	e.ok("POST", "/api/bulk/tags", map[string]any{"ids": []int64{a}, "add_names": []string{"Test shot"}})
	mustRemove(t, filepath.Join(e.lib, "Photos", "a.png"))
	p := writeLibFile(t, e.lib, "Photos/c.png", same)
	_ = os.Chtimes(p, stamp, stamp)
	src = e.rescan()
	if msg := fmt.Sprint(src["last_scan"].(map[string]any)["message"]); strings.Contains(msg, "moved") {
		t.Errorf("ambiguous pictures must not be matched: %s", msg)
	}
	for _, name := range []string{"b.png", "c.png"} {
		items, _ := listFiles(t, e.c, "q="+name)
		if len(items) != 1 || len(items[0]["tags"].([]any)) != 0 {
			t.Errorf("%s must not inherit the tag: %v", name, items)
		}
	}
}

func mustRemove(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
}
