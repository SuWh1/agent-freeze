package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DataMata2/agent-freeze/internal/auto"
)

// pinHome points the auto config at a fresh directory for one test.
func pinHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func pinnedKeys(t *testing.T) []string {
	t.Helper()
	c, err := auto.LoadConfig(auto.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	return c.Pins
}

func TestPinJSON(t *testing.T) {
	pinHome(t)
	code, out, errs := run(newFake(t), "pin", "acme", "--json")
	if code != 0 || errs != "" {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
	want := `{"results":[{"id":"102","key":"claude-102-1000","name":"Acme web frontend builder","ok":true,"pinned":true}]}` + "\n"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if !slices.Equal(pinnedKeys(t), []string{"claude-102-1000"}) {
		t.Errorf("pins = %v", pinnedKeys(t))
	}
}

func TestPinIsIdempotent(t *testing.T) {
	pinHome(t)
	for range 2 {
		code, out, _ := run(newFake(t), "pin", "102", "--json")
		if code != 0 || !strings.Contains(out, `"ok":true,"pinned":true`) {
			t.Fatalf("code=%d out=%q", code, out)
		}
	}
	if got := pinnedKeys(t); len(got) != 1 {
		t.Errorf("pins = %v, want one entry", got)
	}
}

func TestUnpinJSON(t *testing.T) {
	pinHome(t)
	run(newFake(t), "pin", "102")
	code, out, _ := run(newFake(t), "unpin", "102", "--json")
	want := `{"results":[{"id":"102","key":"claude-102-1000","name":"Acme web frontend builder","ok":true,"pinned":false}]}` + "\n"
	if code != 0 || out != want {
		t.Errorf("code=%d out=%q, want %q", code, out, want)
	}
	if len(pinnedKeys(t)) != 0 {
		t.Errorf("pins = %v, want none", pinnedKeys(t))
	}
	code, out, _ = run(newFake(t), "unpin", "102", "--json")
	if code != 0 || !strings.Contains(out, `"ok":true,"pinned":false`) {
		t.Errorf("unpinning twice: code=%d out=%q", code, out)
	}
}

func TestPinTextOutput(t *testing.T) {
	pinHome(t)
	code, out, errs := run(newFake(t), "pin", "acme")
	if code != 0 || errs != "" || out != "pinned Acme web frontend builder (102)\n" {
		t.Errorf("pin: code=%d out=%q stderr=%q", code, out, errs)
	}
	code, out, _ = run(newFake(t), "unpin", "acme")
	if code != 0 || out != "unpinned Acme web frontend builder (102)\n" {
		t.Errorf("unpin: code=%d out=%q", code, out)
	}
}

func TestPinSelectorsDeduplicateInListOrder(t *testing.T) {
	pinHome(t)
	code, out, _ := run(newFake(t), "pin", "201", "acme", "102", "--json")
	if code != 0 || strings.Count(out, `"id"`) != 2 || !strings.Contains(out, `{"id":"102"`) {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if want := []string{"claude-102-1000", "claude-201-1000"}; !slices.Equal(pinnedKeys(t), want) {
		t.Errorf("pins = %v, want %v", pinnedKeys(t), want)
	}
}

func TestPinSelectorErrorsMatchFreeze(t *testing.T) {
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"pin", "nope"}, `no agent matches "nope"`},
		{[]string{"unpin", "claude"}, `"claude" matches several agents`},
		{[]string{"pin", "acme", "nope"}, `no agent matches "nope"`},
	}
	for _, c := range cases {
		pinHome(t)
		code, _, errs := run(newFake(t), c.args...)
		if code != 2 || !strings.Contains(errs, c.msg) {
			t.Errorf("%v: code=%d stderr=%q", c.args, code, errs)
		}
		if len(pinnedKeys(t)) != 0 {
			t.Errorf("%v: wrote pins %v", c.args, pinnedKeys(t))
		}
		jargs := append(slices.Clone(c.args), "--json")
		code, out, _ := run(newFake(t), jargs...)
		var v map[string]string
		if code != 2 || json.Unmarshal([]byte(out), &v) != nil || !strings.Contains(v["error"], c.msg) {
			t.Errorf("%v: code=%d stdout=%q", jargs, code, out)
		}
	}
}

func TestPinUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"pin"}, {"unpin"}, {"pin", "--all"}, {"unpin", "--yes", "102"}, {"pin", "--all-procs", "102"}} {
		pinHome(t)
		if code, _, _ := run(newFake(t), args...); code != 2 {
			t.Errorf("%v: code=%d, want 2", args, code)
		}
	}
}

func TestPinFailureExitsOneAndKeepsConfig(t *testing.T) {
	pinHome(t)
	path := auto.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	const broken = "{not json"
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(newFake(t), "pin", "102", "--json")
	var v struct {
		Results []struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("out=%q: %v", out, err)
	}
	if code != 1 || len(v.Results) != 1 || v.Results[0].OK || v.Results[0].Error == "" {
		t.Errorf("code=%d out=%q", code, out)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != broken {
		t.Errorf("config overwritten: %q", raw)
	}
}

func TestListJSONPinnedFlag(t *testing.T) {
	pinHome(t)
	run(newFake(t), "pin", "201")
	_, out, _ := run(newFake(t), "list", "--json")
	var v struct {
		Groups []struct {
			ID     string `json:"id"`
			Pinned bool   `json:"pinned"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, g := range v.Groups {
		got[g.ID] = g.Pinned
	}
	want := map[string]bool{"102": false, "201": true, "200": false, "300": false}
	if len(got) != len(want) {
		t.Fatalf("groups = %v", got)
	}
	for id, p := range want {
		if got[id] != p {
			t.Errorf("group %s pinned = %v, want %v", id, got[id], p)
		}
	}
}

func TestListJSONSurvivesBrokenAutoConfig(t *testing.T) {
	pinHome(t)
	path := auto.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errs := run(newFake(t), "list", "--json")
	if code != 0 || !strings.Contains(out, `"pinned":false`) || !strings.Contains(errs, "auto config") {
		t.Errorf("code=%d out=%q stderr=%q", code, out, errs)
	}
}
