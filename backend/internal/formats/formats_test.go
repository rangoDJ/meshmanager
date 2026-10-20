package formats

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name   string
		format string
		kind   Kind
		ok     bool
	}{
		{"cube.stl", "stl", Model, true},
		{"project.3mf", "3mf", Model, true},
		{"box.gcode.3mf", GCode3MF, GCode, true},
		{"Box.GCODE.3MF", GCode3MF, GCode, true},
		{"part.gcode", "gcode", GCode, true},
		{"gcode.3mf.txt", "txt", Document, true},
		{"gcode.3mf", "3mf", Model, true}, // no dot before gcode: just a 3MF
		{"models.tar.gz", "tgz", Archive, true},
		{"notes.unknown", "", "", false},
	}
	for _, tt := range tests {
		f, ok := Detect(tt.name)
		if ok != tt.ok || f.Name != tt.format || f.Kind != tt.kind {
			t.Errorf("Detect(%q) = %+v, %v; want %s %s %v", tt.name, f, ok, tt.format, tt.kind, tt.ok)
		}
	}
}

func TestGCode3MFOpensAsToolpath(t *testing.T) {
	if CanView(GCode3MF) {
		t.Error("a sliced .gcode.3mf must not open in the 3D viewer")
	}
	if !CanThumbnail(GCode3MF) {
		t.Error("a sliced .gcode.3mf has an embedded plate preview")
	}
	if Hashed(GCode) {
		t.Error("G-code isn't hashed")
	}
}
