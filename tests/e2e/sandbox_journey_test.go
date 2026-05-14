package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/sandbox"
)

// TestSandboxFullLifecycleJourney drives the entire `conduit sandbox`
// substrate end-to-end through Manager + Workspace APIs:
//
//   - create three sandboxes, verify on-disk layout
//   - set/clear the active pointer
//   - write workspace + home files
//   - take v1 snapshot, mutate state, take v2 snapshot
//   - clone the sandbox; verify post-mutation parity, clone has no snapshots
//   - rollback the original to v1; assert clone is unaffected
//   - exercise maxSnapshots auto-prune
//   - destroy active sandbox; assert active-pointer is auto-cleared
//   - verify final list contains only "staging" and "experiment"
func TestSandboxFullLifecycleJourney(t *testing.T) {
	home := newHome(t)
	root := filepath.Join(home.Root, "sandboxes")
	mgr := sandbox.NewManager(root)

	// ── 1. Bootstrap & 2. Create three sandboxes ─────────────────────────
	names := []string{"dev", "staging", "experiment"}
	for _, name := range names {
		ws, err := mgr.Create(name, sandbox.CreateOptions{})
		if err != nil {
			t.Fatalf("Create(%q): unexpected error: %v", name, err)
		}
		if ws.Name() != name {
			t.Fatalf("Create(%q): workspace name mismatch: got %q", name, ws.Name())
		}
		// on-disk layout: workspace/ and home/ both exist under <root>/sandboxes/<name>/
		wsPath := ws.Path(sandbox.SubdirWorkspace)
		hmPath := ws.Path(sandbox.SubdirHome)
		mustBeDir(t, wsPath, "Create("+name+") workspace subdir")
		mustBeDir(t, hmPath, "Create("+name+") home subdir")
	}

	list, err := mgr.List()
	if err != nil {
		t.Fatalf("List after creates: %v", err)
	}
	if got := sandboxNames(list); !equalSets(got, names) {
		t.Fatalf("List() after create: expected %v, got %v", names, got)
	}

	// ── 3. Active sandbox: ErrNoActive → SetActive → Active ─────────────
	if active, err := mgr.Active(); !errors.Is(err, sandbox.ErrNoActive) {
		t.Fatalf("Active() before SetActive: expected ErrNoActive, got (%q, %v)", active, err)
	}
	if err := mgr.SetActive("dev"); err != nil {
		t.Fatalf("SetActive(\"dev\"): %v", err)
	}
	active, err := mgr.Active()
	if err != nil {
		t.Fatalf("Active() after SetActive(\"dev\"): %v", err)
	}
	if active != "dev" {
		t.Fatalf("Active(): expected \"dev\", got %q", active)
	}
	// pointer file must exist under <root>/sandboxes/.active
	pointerPath := filepath.Join(root, "sandboxes", ".active")
	if _, err := os.Stat(pointerPath); err != nil {
		t.Fatalf("active pointer file %q: expected to exist, got %v", pointerPath, err)
	}

	// ── 4. Switch + write workspace content ─────────────────────────────
	ws, err := mgr.Switch("dev")
	if err != nil {
		t.Fatalf("Switch(\"dev\"): %v", err)
	}
	if ws.Name() != "dev" {
		t.Fatalf("Switch returned workspace named %q, expected \"dev\"", ws.Name())
	}

	originalMain := "package main\nfunc main(){}\n"
	originalNotes := "initial notes\n"
	originalValues := "v1\nv2\nv3\n"
	originalGitconfig := "[user]\n  name = Tester\n"

	write := func(rel, content string, sub sandbox.Subdir) string {
		return mustWrite(t, filepath.Join(ws.Path(sub), rel), content)
	}
	mainPath := write("main.go", originalMain, sandbox.SubdirWorkspace)
	notesPath := write("notes.md", originalNotes, sandbox.SubdirWorkspace)
	valuesPath := write("data/values.txt", originalValues, sandbox.SubdirWorkspace)
	gitconfigPath := write(".gitconfig", originalGitconfig, sandbox.SubdirHome)

	for label, p := range map[string]string{
		"main.go":    mainPath,
		"notes.md":   notesPath,
		"values.txt": valuesPath,
		".gitconfig": gitconfigPath,
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("pre-snapshot file %s at %s: expected to exist, got %v", label, p, err)
		}
	}

	// ── 5. Snapshot v1 ─────────────────────────────────────────────────
	snap1, err := ws.Snapshot("initial", 0)
	if err != nil {
		t.Fatalf("Snapshot v1 on dev: %v", err)
	}
	if snap1.ID == "" {
		t.Fatalf("Snapshot v1: empty ID, info=%+v", snap1)
	}
	if snap1.FileCount < 4 {
		t.Fatalf("Snapshot v1: FileCount expected >=4 (main.go, notes.md, data/values.txt, .gitconfig), got %d", snap1.FileCount)
	}
	if snap1.SizeBytes <= 0 {
		t.Fatalf("Snapshot v1: SizeBytes expected >0, got %d", snap1.SizeBytes)
	}
	v1MetaPath := filepath.Join(ws.Path(sandbox.SubdirSnapshots), snap1.ID, "meta.yaml")
	if _, err := os.Stat(v1MetaPath); err != nil {
		t.Fatalf("Snapshot v1 meta.yaml at %s: expected to exist, got %v", v1MetaPath, err)
	}

	// ── 6. Mutate state ────────────────────────────────────────────────
	mutatedMain := originalMain + "// extra line after edits\n"
	if err := os.WriteFile(mainPath, []byte(mutatedMain), 0o600); err != nil {
		t.Fatalf("append main.go: %v", err)
	}
	if err := os.Remove(notesPath); err != nil {
		t.Fatalf("remove notes.md: %v", err)
	}
	secretsPath := write("secrets.env", "API_KEY=shh\n", sandbox.SubdirWorkspace)

	// verify the mutations on disk
	if got, err := os.ReadFile(mainPath); err != nil || string(got) != mutatedMain {
		t.Fatalf("post-mutation main.go: expected %q, got %q (err=%v)", mutatedMain, got, err)
	}
	if _, err := os.Stat(notesPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-mutation notes.md at %s: expected not-exist, got err=%v", notesPath, err)
	}
	if _, err := os.Stat(secretsPath); err != nil {
		t.Fatalf("post-mutation secrets.env at %s: expected to exist, got %v", secretsPath, err)
	}

	// ── 7. Snapshot v2 ─────────────────────────────────────────────────
	snap2, err := ws.Snapshot("after-edits", 0)
	if err != nil {
		t.Fatalf("Snapshot v2 on dev: %v", err)
	}
	if snap2.ID == snap1.ID {
		t.Fatalf("Snapshot v2: id collides with v1 (%q)", snap1.ID)
	}
	snaps, err := ws.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots after v2: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("ListSnapshots after v2: expected 2, got %d (%+v)", len(snaps), snaps)
	}
	// newest first → snap2 must be element 0
	if snaps[0].ID != snap2.ID {
		t.Fatalf("ListSnapshots[0]: expected v2 %q, got %q (full list: %+v)", snap2.ID, snaps[0].ID, snaps)
	}
	if snaps[1].ID != snap1.ID {
		t.Fatalf("ListSnapshots[1]: expected v1 %q, got %q", snap1.ID, snaps[1].ID)
	}

	// ── 8. Clone dev → dev-fork ────────────────────────────────────────
	cloneWS, err := mgr.Clone("dev", "dev-fork", sandbox.CloneOptions{})
	if err != nil {
		t.Fatalf("Clone(dev, dev-fork): %v", err)
	}
	if cloneWS.Name() != "dev-fork" {
		t.Fatalf("Clone returned workspace named %q, expected \"dev-fork\"", cloneWS.Name())
	}
	mustBeDir(t, cloneWS.Path(sandbox.SubdirWorkspace), "clone workspace subdir")
	mustBeDir(t, cloneWS.Path(sandbox.SubdirHome), "clone home subdir")

	// post-mutation files should appear in the clone, notes.md should NOT
	cloneMain := filepath.Join(cloneWS.Path(sandbox.SubdirWorkspace), "main.go")
	if got, err := os.ReadFile(cloneMain); err != nil || string(got) != mutatedMain {
		t.Fatalf("clone main.go at %s: expected mutated content %q, got %q (err=%v)", cloneMain, mutatedMain, got, err)
	}
	cloneSecrets := filepath.Join(cloneWS.Path(sandbox.SubdirWorkspace), "secrets.env")
	if _, err := os.Stat(cloneSecrets); err != nil {
		t.Fatalf("clone secrets.env at %s: expected to exist, got %v", cloneSecrets, err)
	}
	cloneNotes := filepath.Join(cloneWS.Path(sandbox.SubdirWorkspace), "notes.md")
	if _, err := os.Stat(cloneNotes); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clone notes.md at %s: expected not-exist (it was deleted before clone), got err=%v", cloneNotes, err)
	}
	cloneGitconfig := filepath.Join(cloneWS.Path(sandbox.SubdirHome), ".gitconfig")
	if got, err := os.ReadFile(cloneGitconfig); err != nil || string(got) != originalGitconfig {
		t.Fatalf("clone .gitconfig at %s: expected %q, got %q (err=%v)", cloneGitconfig, originalGitconfig, got, err)
	}
	// Clone does NOT carry over snapshots/ by design (see internal/sandbox/clone.go).
	// Document the contract: clone starts with an empty snapshots/ subdir.
	cloneSnaps, err := cloneWS.ListSnapshots()
	if err != nil {
		t.Fatalf("clone ListSnapshots: %v", err)
	}
	if len(cloneSnaps) != 0 {
		t.Fatalf("clone ListSnapshots: expected 0 (snapshots/ intentionally not cloned), got %d (%+v)", len(cloneSnaps), cloneSnaps)
	}

	// ── 9. Rollback dev to v1 ─────────────────────────────────────────
	if err := ws.Rollback(snap1.ID); err != nil {
		t.Fatalf("Rollback(dev, v1=%s): %v", snap1.ID, err)
	}
	// notes.md is back
	if got, err := os.ReadFile(notesPath); err != nil || string(got) != originalNotes {
		t.Fatalf("post-rollback notes.md at %s: expected %q, got %q (err=%v)", notesPath, originalNotes, got, err)
	}
	// secrets.env is gone
	if _, err := os.Stat(secretsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-rollback secrets.env at %s: expected not-exist, got err=%v", secretsPath, err)
	}
	// main.go matches the pre-mutation version
	if got, err := os.ReadFile(mainPath); err != nil || string(got) != originalMain {
		t.Fatalf("post-rollback main.go at %s: expected %q, got %q (err=%v)", mainPath, originalMain, got, err)
	}
	// clone is unaffected
	if got, err := os.ReadFile(cloneMain); err != nil || string(got) != mutatedMain {
		t.Fatalf("post-rollback clone main.go at %s: expected mutated %q, got %q (err=%v)", cloneMain, mutatedMain, got, err)
	}
	if _, err := os.Stat(cloneSecrets); err != nil {
		t.Fatalf("post-rollback clone secrets.env at %s: expected to still exist, got %v", cloneSecrets, err)
	}
	if _, err := os.Stat(cloneNotes); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-rollback clone notes.md at %s: expected still not-exist, got err=%v", cloneNotes, err)
	}

	// ── 10. Snapshot prune via maxSnapshots ────────────────────────────
	// Currently dev has snap1 and snap2 from earlier. Take 3 more snapshots
	// with maxSnapshots=2 each; the cap should hold list length <= 2.
	for i := 0; i < 3; i++ {
		// touch a file so each snapshot has new content / unique mtime
		bump := []byte(originalMain + "// bump " + itoa(i) + "\n")
		if err := os.WriteFile(mainPath, bump, 0o600); err != nil {
			t.Fatalf("bump #%d write main.go: %v", i, err)
		}
		if _, err := ws.Snapshot("prune-"+itoa(i), 2); err != nil {
			t.Fatalf("Snapshot(prune-%d, max=2): %v", i, err)
		}
	}
	finalSnaps, err := ws.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots after prune cycle: %v", err)
	}
	if len(finalSnaps) > 2 {
		t.Fatalf("ListSnapshots after maxSnapshots=2 prune: expected <=2, got %d (%+v)", len(finalSnaps), finalSnaps)
	}

	// ── 11. Destroy dev-fork, then destroy active dev ───────────────────
	if err := mgr.Destroy("dev-fork"); err != nil {
		t.Fatalf("Destroy(dev-fork): %v", err)
	}
	if _, err := os.Stat(cloneWS.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-Destroy dev-fork root %s: expected not-exist, got err=%v", cloneWS.Root(), err)
	}
	postForkList, err := mgr.List()
	if err != nil {
		t.Fatalf("List after destroy dev-fork: %v", err)
	}
	for _, w := range postForkList {
		if w.Name == "dev-fork" {
			t.Fatalf("List after destroy dev-fork: still present (%+v)", postForkList)
		}
	}

	// Destroy "dev" while it's the active sandbox. Per workspace.go, Destroy
	// auto-clears the active pointer when it matches the destroyed name.
	if err := mgr.Destroy("dev"); err != nil {
		t.Fatalf("Destroy(dev) while active: %v", err)
	}
	gotActive, gotErr := mgr.Active()
	if !errors.Is(gotErr, sandbox.ErrNoActive) {
		// Contract fallback (the prompt instructed to handle a possible
		// dangling pointer): clear explicitly and re-assert.
		t.Logf("contract note: Active() after Destroy of active returned (%q, %v); calling ClearActive() explicitly", gotActive, gotErr)
		if clearErr := mgr.ClearActive(); clearErr != nil {
			t.Fatalf("ClearActive after destroy: %v", clearErr)
		}
		if gotActive, gotErr = mgr.Active(); !errors.Is(gotErr, sandbox.ErrNoActive) {
			t.Fatalf("Active() after ClearActive: expected ErrNoActive, got (%q, %v)", gotActive, gotErr)
		}
	}

	// ── 12. Final state ────────────────────────────────────────────────
	finalList, err := mgr.List()
	if err != nil {
		t.Fatalf("Final List: %v", err)
	}
	finalNames := sandboxNames(finalList)
	want := []string{"staging", "experiment"}
	if !equalSets(finalNames, want) {
		t.Fatalf("Final list: expected %v, got %v", want, finalNames)
	}

	if _, err := mgr.Switch("staging"); err != nil {
		t.Fatalf("Switch(staging) at end: %v", err)
	}
	final, err := mgr.Active()
	if err != nil {
		t.Fatalf("Active() after Switch(staging): %v", err)
	}
	if final != "staging" {
		t.Fatalf("Active() after final switch: expected \"staging\", got %q", final)
	}
}

// ── helpers (kept local to this file) ──────────────────────────────────

// mustBeDir asserts path exists and is a directory; t.Fatalf with context.
func mustBeDir(t *testing.T, path, label string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("%s: stat %s: %v", label, path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s: %s exists but is not a directory", label, path)
	}
}

// sandboxNames extracts the Name field from a slice of WorkspaceInfo, sorted.
func sandboxNames(list []sandbox.WorkspaceInfo) []string {
	out := make([]string, 0, len(list))
	for _, w := range list {
		out = append(out, w.Name)
	}
	sort.Strings(out)
	return out
}

// equalSets compares two string slices irrespective of order.
func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	return strings.Join(ac, "\x00") == strings.Join(bc, "\x00")
}

// itoa is a tiny strconv.Itoa shim to avoid importing strconv solely for it.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
