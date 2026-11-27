package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform/cert"
)

func TestMutateFakeCLIAndPage(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "contract", "testdata", "fakecli", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(page) }))
	defer server.Close()
	md := fmt.Sprintf("### F-01\nList forms.\n```console\n$ form list\nID\tSTATUS\n```\nexit: 0\n\n### U-01\nInspect form.\n```ui\nGET %s/?id=form1\n{\"fields\":[{\"id\":\"name\",\"label\":\"Full name\",\"kind\":\"text\",\"required\":true}],\"buttons\":[\"Save\",\"Submit\"]}\n```\n", server.URL)
	contractFile := filepath.Join(t.TempDir(), "CONTRACT.md")
	if err := os.WriteFile(contractFile, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	_, bin := fixturePaths(t)
	var out, errb bytes.Buffer
	code := Run([]string{"mutate", contractFile, "--path", bin, "--dir", t.TempDir(), "--fresh-home", "--json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, out.String(), errb.String())
	}
	var report cert.MutationReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Total == 0 || report.Killed != report.Total || report.Strength != 1 {
		t.Fatalf("report: %+v", report)
	}
	var cli, ui bool
	for _, m := range report.Mutants {
		if m.Layer == cert.LayerCLI {
			cli = true
		}
		if m.Layer == cert.LayerUI {
			ui = true
		}
	}
	if !cli || !ui {
		t.Fatalf("missing mutation layer: %+v", report)
	}
	out.Reset()
	errb.Reset()
	code = Run([]string{"mutate", contractFile, "--path", bin, "--dir", t.TempDir(), "--fresh-home"}, &out, &errb)
	if code != 0 || !strings.Contains(out.String(), "strength ") || !strings.Contains(out.String(), "KILLED ui") {
		t.Fatalf("text exit %d: %s\n%s", code, out.String(), errb.String())
	}
}

func TestMutateReportsFailingBaselineNames(t *testing.T) {
	contractFile := filepath.Join(t.TempDir(), "CONTRACT.md")
	md := "### F-01\nMismatch.\n```console\n$ form list\nmissing\n```\nexit: 0\n"
	if err := os.WriteFile(contractFile, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	_, bin := fixturePaths(t)
	var out, errb bytes.Buffer
	code := Run([]string{"mutate", contractFile, "--path", bin, "--dir", t.TempDir()}, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "F-01") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}

// Shards partition the mutant set: N runs with K = 0..N-1 are disjoint and
// together equal one unsharded run, so a large contract mutates in parallel
// with conform alone (Sprint 737 needed a bespoke shard tool for 559 mutants).
func TestMutateShardsPartitionTheMutantSet(t *testing.T) {
	md := "### F-01\nList forms.\n```console\n$ form list\nID\tSTATUS\n```\nexit: 0\n\n### F-02\nList forms again.\n```console\n$ form list\nID\tSTATUS\n```\nexit: 0\n"
	contractFile := filepath.Join(t.TempDir(), "CONTRACT.md")
	if err := os.WriteFile(contractFile, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	_, bin := fixturePaths(t)
	run := func(extra ...string) cert.MutationReport {
		t.Helper()
		var out, errb bytes.Buffer
		args := append([]string{"mutate", contractFile, "--path", bin, "--dir", t.TempDir(), "--fresh-home", "--json"}, extra...)
		if code := Run(args, &out, &errb); code != 0 {
			t.Fatalf("%v: exit %d: %s\n%s", extra, code, out.String(), errb.String())
		}
		var r cert.MutationReport
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	full := run()
	a, b := run("--shards", "2", "--shard", "0"), run("--shards=2", "--shard=1")
	if a.Total+b.Total != full.Total || a.Total == 0 || b.Total == 0 {
		t.Fatalf("shard totals %d + %d != %d", a.Total, b.Total, full.Total)
	}
	seen := map[string]bool{}
	for _, m := range append(a.Mutants, b.Mutants...) {
		if seen[m.Name] {
			t.Fatalf("mutant %s in both shards", m.Name)
		}
		seen[m.Name] = true
	}
	for _, m := range full.Mutants {
		if !seen[m.Name] {
			t.Fatalf("mutant %s in no shard", m.Name)
		}
	}
	for _, bad := range [][]string{{"--shards", "0"}, {"--shards", "2", "--shard", "2"}, {"--shard", "-1"}, {"--shards", "x"}, {"--shards"}} {
		var out, errb bytes.Buffer
		if code := Run(append([]string{"mutate", contractFile}, bad...), &out, &errb); code != 2 {
			t.Fatalf("%v: exit %d, want 2", bad, code)
		}
	}
}
