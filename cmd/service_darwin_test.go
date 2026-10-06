//go:build darwin

package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeLaunchctl records launchctl invocations; print succeeds only when loaded.
type fakeLaunchctl struct {
	loaded bool
	pid    string
	calls  []string
	// lingering makes "print" keep finding the job for this many probes after
	// bootout, like launchd while a job is still being torn down.
	lingering int
	dying     int
}

func (f *fakeLaunchctl) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch args[0] {
	case "print":
		if f.dying > 0 {
			f.dying--
			return []byte("com.weclaw.bridge = {\n\tstate = SIGTERMed\n}"), nil
		}
		if !f.loaded {
			return []byte("Could not find service"), errors.New("exit status 113")
		}
		return []byte("com.weclaw.bridge = {\n\tstate = running\n\tpid = " + f.pid + "\n}"), nil
	case "bootstrap":
		f.loaded = true
	case "bootout":
		f.loaded = false
		f.dying = f.lingering
	}
	return nil, nil
}

func withFakeLaunchctl(t *testing.T, loaded bool) *fakeLaunchctl {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	f := &fakeLaunchctl{loaded: loaded, pid: "4242"}
	orig, origTimeout, origPoll, origInside := launchctl, serviceStopTimeout, servicePollInterval, insideService
	launchctl = f.run
	serviceStopTimeout, servicePollInterval = 200*time.Millisecond, time.Millisecond
	insideService = func() bool { return false }
	t.Cleanup(func() {
		launchctl, serviceStopTimeout, servicePollInterval, insideService = orig, origTimeout, origPoll, origInside
	})
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
		{"restart when unloaded bootstraps", false, func() error { _, err := serviceRestart(); return err }, "bootstrap"},
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

func TestServicePath(t *testing.T) {
	tmp := t.TempDir() // exists, but is a per-session temp dir
	raw := strings.Join([]string{
		tmp,
		"/opt/homebrew/bin",
		"relative/bin",
		"/usr/local/bin",
		"/nonexistent/weclaw-test",
		"/opt/homebrew/bin/", // duplicate after Clean
		"",
		"/var/folders/xx/T/otty-shell-1/bin",
		"/bin",
	}, ":")
	got := strings.Split(servicePath(raw), ":")

	for _, bad := range []string{tmp, "relative/bin", "/nonexistent/weclaw-test", "/var/folders/xx/T/otty-shell-1/bin"} {
		for _, d := range got {
			if d == bad {
				t.Errorf("servicePath kept %q: %v", bad, got)
			}
		}
	}
	count := map[string]int{}
	for _, d := range got {
		count[d]++
		if count[d] > 1 {
			t.Errorf("duplicate %q in %v", d, got)
		}
	}
	for _, want := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		if count[want] != 1 {
			t.Errorf("system dir %q missing from %v", want, got)
		}
	}
	if _, err := os.Stat("/opt/homebrew/bin"); err == nil && got[0] != "/opt/homebrew/bin" {
		t.Errorf("order not preserved, got %v", got)
	}
}

// Reinstalling a running service must end with it loaded again, even though
// launchd keeps reporting the old job for a while after bootout.
func TestServiceInstallWhileRunningWaitsForBootout(t *testing.T) {
	f := withFakeLaunchctl(t, true)
	f.lingering = 3
	if err := serviceInstall(); err != nil {
		t.Fatal(err)
	}
	if !f.loaded {
		t.Fatalf("service not loaded after reinstall; calls = %q", f.calls)
	}
	if m := f.mutations(); len(m) != 2 || m[0] != "bootout" || m[1] != "bootstrap" {
		t.Fatalf("launchctl mutations = %q, want [bootout bootstrap]", m)
	}
}

func TestServiceStopTimesOut(t *testing.T) {
	f := withFakeLaunchctl(t, true)
	f.lingering = 1 << 30
	if err := serviceStop(); err == nil || !strings.Contains(err.Error(), "did not unload") {
		t.Fatalf("serviceStop error = %v, want unload timeout", err)
	}
}

// A rebuilt binary violates the launch constraint launchd pinned to the old one,
// so `kickstart -k` gets its first spawn killed (OS_REASON_CODESIGNING) and the
// bridge stays down for ThrottleInterval. Restart must reload the job instead,
// and must wait for the old job to unload before bootstrapping.
func TestServiceRestartReloadsJob(t *testing.T) {
	f := withFakeLaunchctl(t, true)
	f.lingering = 3
	detached, err := serviceRestart()
	if err != nil {
		t.Fatal(err)
	}
	if detached {
		t.Fatal("restart from outside the service should not detach")
	}
	if !f.loaded {
		t.Fatalf("service not loaded after restart; calls = %q", f.calls)
	}
	if m := f.mutations(); len(m) != 2 || m[0] != "bootout" || m[1] != "bootstrap" {
		t.Fatalf("launchctl mutations = %q, want [bootout bootstrap]", m)
	}
}

// From inside the service, bootout would kill the caller before it could
// bootstrap again, so the reload must be handed to a detached process.
func TestServiceRestartInsideServiceDetaches(t *testing.T) {
	f := withFakeLaunchctl(t, true)
	insideService = func() bool { return true }
	spawned := 0
	origSpawn := spawnDetachedReload
	spawnDetachedReload = func() error { spawned++; return nil }
	t.Cleanup(func() { spawnDetachedReload = origSpawn })

	detached, err := serviceRestart()
	if err != nil {
		t.Fatal(err)
	}
	if !detached || spawned != 1 {
		t.Fatalf("detached = %v, spawned = %d; want true, 1", detached, spawned)
	}
	if m := f.mutations(); len(m) != 0 {
		t.Fatalf("caller ran launchctl mutations %q itself, want none", m)
	}
}

func TestIsDescendantOf(t *testing.T) {
	self, parent := os.Getpid(), os.Getppid()
	if !isDescendantOf(self, self) {
		t.Error("a process should count as descending from itself")
	}
	if !isDescendantOf(self, parent) {
		t.Error("this process should descend from its parent")
	}
	if isDescendantOf(parent, self) {
		t.Error("the parent should not descend from this process")
	}
	if isDescendantOf(self, 0) || isDescendantOf(self, 1) {
		t.Error("pid 0/1 must not count as the service")
	}
}
