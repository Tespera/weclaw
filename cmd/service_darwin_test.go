//go:build darwin

package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLaunchctl records launchctl invocations; print succeeds only when loaded.
type fakeLaunchctl struct {
	loaded bool
	pid    string
	calls  []string
}

func (f *fakeLaunchctl) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch args[0] {
	case "print":
		if !f.loaded {
			return []byte("Could not find service"), errors.New("exit status 113")
		}
		return []byte("com.weclaw.bridge = {\n\tstate = running\n\tpid = " + f.pid + "\n}"), nil
	case "bootstrap":
		f.loaded = true
	case "bootout":
		f.loaded = false
	}
	return nil, nil
}

func withFakeLaunchctl(t *testing.T, loaded bool) *fakeLaunchctl {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	f := &fakeLaunchctl{loaded: loaded, pid: "4242"}
	orig := launchctl
	launchctl = f.run
	t.Cleanup(func() { launchctl = orig })
	return f
}

// mutations returns non-"print" calls as verb plus flags, dropping the
// domain/target/plist path arguments.
func (f *fakeLaunchctl) mutations() []string {
	var out []string
	for _, c := range f.calls {
		fields := strings.Fields(c)
		if fields[0] == "print" {
			continue
		}
		var kept []string
		for _, a := range fields {
			if !strings.Contains(a, "/") {
				kept = append(kept, a)
			}
		}
		out = append(out, strings.Join(kept, " "))
	}
	return out
}

func TestServiceLifecycle(t *testing.T) {
	tests := []struct {
		name   string
		loaded bool
		op     func() error
		want   string // first mutating launchctl verb and flags
	}{
		{"start when loaded kicks", true, serviceStart, "kickstart"},
		{"start when unloaded bootstraps", false, serviceStart, "bootstrap"},
		{"restart when loaded kills and restarts", true, serviceRestart, "kickstart -k"},
		{"restart when unloaded bootstraps", false, serviceRestart, "bootstrap"},
		{"stop when loaded boots out", true, serviceStop, "bootout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := withFakeLaunchctl(t, tt.loaded)
			if err := tt.op(); err != nil {
				t.Fatal(err)
			}
			m := f.mutations()
			if len(m) != 1 || m[0] != tt.want {
				t.Fatalf("launchctl mutations = %q, want [%q]", m, tt.want)
			}
		})
	}

	t.Run("stop when unloaded is a no-op", func(t *testing.T) {
		f := withFakeLaunchctl(t, false)
		if err := serviceStop(); err != nil {
			t.Fatal(err)
		}
		if m := f.mutations(); len(m) != 0 {
			t.Fatalf("launchctl mutations = %q, want none", m)
		}
	})
}

func TestServicePid(t *testing.T) {
	f := withFakeLaunchctl(t, true)
	if got := servicePid(); got != 4242 {
		t.Fatalf("servicePid = %d, want 4242", got)
	}
	f.loaded = false
	if got := servicePid(); got != 0 {
		t.Fatalf("servicePid when unloaded = %d, want 0", got)
	}
}

func TestServiceInstallUninstall(t *testing.T) {
	f := withFakeLaunchctl(t, false)
	if serviceInstalled() {
		t.Fatal("fresh HOME should have no service")
	}
	if err := serviceInstall(); err != nil {
		t.Fatal(err)
	}
	if !serviceInstalled() || !f.loaded {
		t.Fatalf("after install: installed=%v loaded=%v", serviceInstalled(), f.loaded)
	}
	data, err := os.ReadFile(plistPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{launchdLabel, "<string>start</string>", "<string>--foreground</string>", "<key>KeepAlive</key>"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("plist missing %q", want)
		}
	}
	if err := serviceUninstall(); err != nil {
		t.Fatal(err)
	}
	if serviceInstalled() || f.loaded {
		t.Fatalf("after uninstall: installed=%v loaded=%v", serviceInstalled(), f.loaded)
	}
}

func TestRenderPlistIsValidAndEscaped(t *testing.T) {
	content, err := renderPlist(plistParams{
		Label:   launchdLabel,
		Exe:     "/Users/a&b/bin/weclaw",
		Path:    "/opt/<x>/bin:/usr/bin",
		Home:    "/Users/a&b",
		WorkDir: "/Users/a&b/.weclaw",
		Log:     "/Users/a&b/.weclaw/weclaw.log",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "/Users/a&amp;b/bin/weclaw") || !strings.Contains(content, "/opt/&lt;x&gt;/bin") {
		t.Fatalf("special characters not escaped:\n%s", content)
	}
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil not available")
	}
	p := filepath.Join(t.TempDir(), "test.plist")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("plutil", "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint: %v: %s", err, out)
	}
}
