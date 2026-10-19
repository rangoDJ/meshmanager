// Package formats maps file extensions to the kinds of files the library indexes.
package formats

import (
	"path"
	"strings"
)

type Kind string

const (
	Model    Kind = "model"
	Archive  Kind = "archive"
	Image    Kind = "image"
	Document Kind = "document"
	Video    Kind = "video"
	Laser    Kind = "laser"
)

type Format struct {
	Name string
	Kind Kind
	// Thumbnail is true when this build can produce a thumbnail for the format.
	Thumbnail bool
}

var byExt = map[string]Format{
	// 3D models
	".stl":  {"stl", Model, true},
	".3mf":  {"3mf", Model, true},
	".obj":  {"obj", Model, true},
	".ply":  {"ply", Model, true},
	".glb":  {"glb", Model, true},
	".gltf": {"gltf", Model, false},
	".fbx":  {"fbx", Model, false},
	".step": {"step", Model, false},
	".stp":  {"step", Model, false},
	".3dm":  {"3dm", Model, false},
	".f3d":  {"f3d", Model, false},
	".f3z":  {"f3z", Model, false},
	".lys":  {"lys", Model, false},

	// archives
	".zip":    {"zip", Archive, false},
	".rar":    {"rar", Archive, false},
	".7z":     {"7z", Archive, false},
	".tar":    {"tar", Archive, false},
	".tgz":    {"tgz", Archive, false},
	".tar.gz": {"tgz", Archive, false},

	// images
	".jpg":  {"jpg", Image, true},
	".jpeg": {"jpg", Image, true},
	".png":  {"png", Image, true},
	".gif":  {"gif", Image, true},
	".webp": {"webp", Image, true},
	".bmp":  {"bmp", Image, true},
	".heic": {"heic", Image, false},
	".heif": {"heic", Image, false},
	".avif": {"avif", Image, false},
	".svg":  {"svg", Image, false},

	// documents
	".pdf":      {"pdf", Document, false},
	".txt":      {"txt", Document, false},
	".md":       {"md", Document, false},
	".markdown": {"md", Document, false},

	// video
	".mp4":  {"mp4", Video, false},
	".m4v":  {"mp4", Video, false},
	".mov":  {"mov", Video, false},
	".webm": {"webm", Video, false},

	// laser & CNC
	".xcs":   {"xcs", Laser, false},
	".xs":    {"xs", Laser, false},
	".lbrn":  {"lbrn", Laser, false},
	".lbrn2": {"lbrn", Laser, false},
	".dxf":   {"dxf", Laser, false},
	".dwg":   {"dwg", Laser, false},
}

// Detect returns the format for a file name, or false if the library doesn't index it.
func Detect(name string) (Format, bool) {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") {
		return byExt[".tar.gz"], true
	}
	f, ok := byExt[path.Ext(lower)]
	return f, ok
}

// Hashed reports whether files of this kind get a content hash during scans.
// Models and archives are hashed for duplicate detection and to track renames.
func Hashed(k Kind) bool {
	return k == Model || k == Archive
}

// ThumbnailFormats lists format names that can get a thumbnail.
func ThumbnailFormats() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range byExt {
		if f.Thumbnail && !seen[f.Name] {
			seen[f.Name] = true
			out = append(out, f.Name)
		}
	}
	return out
}

// CanView reports whether a format opens in the 3D viewer.
func CanView(format string) bool {
	switch format {
	case "stl", "3mf", "obj", "ply", "glb":
		return true
	}
	return false
}

// CanThumbnail reports whether a format name can get a thumbnail.
func CanThumbnail(format string) bool {
	for _, f := range byExt {
		if f.Name == format {
			return f.Thumbnail
		}
	}
	return false
}
