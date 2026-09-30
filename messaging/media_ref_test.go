package messaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weclaw/ilink"
)

func TestResolveMediaRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	withSpace := filepath.Join(dir, "my report.pdf")
	img := filepath.Join(home, "pic.png")
	for _, p := range []string{withSpace, img, filepath.Join(dir, "rel.bin")} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// macOS temp dirs live behind the /var -> /private/var symlink.
	realDir, _ := filepath.EvalSymlinks(dir)

	ok := []struct{ ref, wantURL, wantPath string }{
		{"https://example.com/a.png", "https://example.com/a.png", ""},
		{"HTTP://example.com/a", "HTTP://example.com/a", ""},
		{withSpace, "", withSpace},
		{"file://" + strings.ReplaceAll(withSpace, " ", "%20"), "", withSpace},
		{"~/pic.png", "", img},
		{"rel.bin", "", filepath.Join(realDir, "rel.bin")},
		{"  " + withSpace + "  ", "", withSpace},
	}
	for _, tt := range ok {
		m, err := ResolveMediaRef(tt.ref)
		if err != nil {
			t.Errorf("ResolveMediaRef(%q) error: %v", tt.ref, err)
			continue
		}
		gotPath := m.Path
		if gotPath != "" {
			gotPath, _ = filepath.EvalSymlinks(gotPath)
		}
		wantPath := tt.wantPath
		if wantPath != "" {
			wantPath, _ = filepath.EvalSymlinks(wantPath)
		}
		if m.URL != tt.wantURL || gotPath != wantPath {
			t.Errorf("ResolveMediaRef(%q) = %+v, want URL=%q Path=%q", tt.ref, m, tt.wantURL, tt.wantPath)
		}
	}

	bad := map[string]string{
		"":                                "empty",
		filepath.Join(dir, "missing.txt"): "no such file",
		dir:                               "not a regular file",
	}
	for ref, want := range bad {
		if _, err := ResolveMediaRef(ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ResolveMediaRef(%q) error = %v, want %q", ref, err, want)
		}
	}
}

func TestLocalFileMediaClassification(t *testing.T) {
	cases := map[string]int{
		"/tmp/a.png":        ilink.ItemTypeImage,
		"/tmp/b.MOV":        ilink.ItemTypeVideo,
		"/tmp/c.zip":        ilink.ItemTypeFile,
		"/tmp/d.key":        ilink.ItemTypeFile,
		"/tmp/no-extension": ilink.ItemTypeFile,
	}
	for p, want := range cases {
		if _, got := classifyMedia(inferContentType(p), p); got != want {
			t.Errorf("classifyMedia(%s) item type = %d, want %d", p, got, want)
		}
	}
}
