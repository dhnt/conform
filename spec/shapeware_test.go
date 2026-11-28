package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pingContract = "# Contract\n\nObjects without `\"...\":\"...\"` are closed.\n\n### A-01\nRun.\n```console\n$ app ping\nok\n```\nexit: 0\n"

func skillMD(meta, issues string) string {
	return "---\nname: demo\ndescription: Demo.\nmetadata:\n  type: shapeware\n" + meta +
		"---\n# demo\n\nType: shapeware (v0.1)\n\n## Known issues\n" + issues +
		"\n## Later\n- KI-99 [fixed] feature:none outside the section -> ignored\n"
}

func checkWith(t *testing.T, contractMD, featuresYAML, skill string, files map[string]string) []Finding {
	t.Helper()
	dir := writeSpecDir(t, contractMD, featuresYAML, files)
	if skill != "" {
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	return findings
}

func feat(id, status, resolves string, acceptance ...string) string {
	s := "- id: " + id + "\n  status: " + status + "\n"
	if resolves != "" {
		s += "  resolves: " + resolves + "\n"
	}
	if len(acceptance) > 0 {
		s += "  acceptance:\n"
		for _, a := range acceptance {
			s += "    - " + a + "\n"
		}
	}
	return s
}

const kiOpen = "- KI-3 [open] feature:view the date line shows undefined -> report it\n  retry: 1\n"

func TestKnownIssueLinkRules(t *testing.T) {
	kiFixed := func(by string) string {
		s := "- KI-3 [fixed] feature:view the date line shows undefined -> nothing\n  retry: 1\n"
		if by != "" {
			s += "  fixed-by: " + by + "\n"
		}
		return s + "  todo: 59dea9db\n"
	}
	for _, tc := range []struct {
		name, features, issues, moved string
		rule, subject, message        string // rule "" = clean
	}{
		{name: "tbd resolves open issue", features: feat("date-line", "tbd", "KI-3", "app ping => ok"), issues: kiOpen},
		{name: "resolves unknown issue", features: feat("date-line", "tbd", "KI-4", "app ping => ok"), issues: kiOpen,
			rule: "resolves-unknown-ki", subject: "date-line", message: "resolves: KI-4, which is not a known issue"},
		{name: "resolves issue outside the section", features: feat("date-line", "tbd", "KI-99", "app ping => ok"), issues: kiOpen,
			rule: "resolves-unknown-ki", subject: "date-line"},
		{name: "resolves issue moved to references", features: feat("date-line", "verified", "KI-7", "app ping => ok"), issues: kiOpen,
			moved: "# Fixed\n- KI-7 [fixed] feature:view old -> none\n  fixed-by: date-line\n"},
		{name: "resolves with no acceptance", features: feat("date-line", "tbd", "KI-3"), issues: kiOpen,
			rule: "resolves-no-acceptance", subject: "date-line", message: "no acceptance line"},
		{name: "verified resolves open issue", features: feat("date-line", "verified", "KI-3", "app ping => ok"), issues: kiOpen,
			rule: "resolves-ki-not-fixed", subject: "date-line", message: "still open; mark KI-3 fixed with fixed-by: date-line"},
		{name: "verified resolves expired issue", features: feat("date-line", "verified", "KI-3", "app ping => ok"),
			issues: strings.Replace(kiOpen, "[open]", "[expired]", 1), rule: "resolves-ki-not-fixed", subject: "date-line", message: "still expired"},
		{name: "fixed by its verified feature", features: feat("date-line", "verified", "KI-3", "app ping => ok"), issues: kiFixed("date-line")},
		{name: "fixed without fixed-by", features: feat("date-line", "verified", "KI-3", "app ping => ok"), issues: kiFixed(""),
			rule: "ki-fixed-without-fixed-by", subject: "KI-3", message: "needs fixed-by"},
		{name: "fixed by a missing feature", features: feat("date-line", "verified", "KI-3", "app ping => ok"), issues: kiFixed("nope"),
			rule: "ki-fixed-by-unknown", subject: "KI-3", message: "fixed-by: nope names no feature"},
		{name: "fixed by a tbd feature", features: feat("date-line", "tbd", "KI-3", "app ping => ok"), issues: kiFixed("date-line"),
			rule: "ki-fixed-by-not-verified", subject: "KI-3", message: "fixed-by: date-line is tbd"},
		{name: "fixed by a feature resolving nothing", features: feat("date-line", "verified", "", "app ping => ok"), issues: kiFixed("date-line"),
			rule: "ki-fixed-by-mismatch", subject: "KI-3", message: "resolves nothing, not KI-3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var files map[string]string
			if tc.moved != "" {
				files = map[string]string{"references/known-issues.md": tc.moved}
			}
			got := checkWith(t, pingContract, tc.features, skillMD("", tc.issues), files)
			assertOne(t, got, tc.rule, tc.subject, tc.message)
		})
	}
}

func TestResolvesWithoutSkillMD(t *testing.T) {
	got := checkWith(t, pingContract, feat("date-line", "tbd", "KI-1", "app ping => ok"), "", nil)
	assertOne(t, got, "resolves-unknown-ki", "date-line", "KI-1")
}

func TestCandidateFilesRules(t *testing.T) {
	impl := "  implementation: \"https://example.invalid/app.git\"\n"
	for _, tc := range []struct {
		name, meta, rule, message string
	}{
		{name: "valid globs", meta: impl + "  candidate-files: \"internal/backup/*.go internal/backup/testdata/*\"\n  build-outputs: \"bin/app\"\n"},
		{name: "no candidate files", meta: impl},
		{name: "absolute glob", meta: impl + "  candidate-files: \"/etc/*\"\n", rule: "candidate-files-invalid", message: "not a repo-relative"},
		{name: "dot-dot glob", meta: impl + "  candidate-files: \"internal/../main.go\"\n", rule: "candidate-files-invalid", message: `".."`},
		{name: "double star", meta: impl + "  candidate-files: \"internal/**/*.go\"\n", rule: "candidate-files-invalid", message: "**"},
		{name: "bad pattern", meta: impl + "  candidate-files: \"internal/[a.go\"\n", rule: "candidate-files-invalid", message: "not a valid glob"},
		{name: "empty", meta: impl + "  candidate-files: \"\"\n", rule: "candidate-files-invalid", message: "is empty"},
		{name: "bad build output", meta: impl + "  candidate-files: \"a/*.go\"\n  build-outputs: \"../bin\"\n", rule: "candidate-files-invalid", message: `".."`},
		{name: "not a lifted surface", meta: "  candidate-files: \"a/*.go\"\n", rule: "candidate-files-not-lifted", message: "metadata.implementation is not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkWith(t, pingContract, feat("ping", "verified", "", "app ping => ok"), skillMD(tc.meta, ""), nil)
			subject := ""
			if tc.rule != "" {
				subject = "metadata.candidate-files"
				if tc.name == "bad build output" {
					subject = "metadata.build-outputs"
				}
			}
			assertOne(t, got, tc.rule, subject, tc.message)
		})
	}
}

func TestGoTestRules(t *testing.T) {
	gotestContract := func(cmd, out, exit string) string {
		return "# Contract\n\nObjects without `\"...\":\"...\"` are closed.\n\n### B-01\nPinned.\n```console\n$ " + cmd + "\n" + out + "\n```\nexit: " + exit + "\n"
	}
	for _, tc := range []struct {
		name, contract, line, rule, subject, message string
	}{
		{name: "acceptance pinned test", contract: pingContract, line: "classgo-backup gotest ./internal/backup TestRestore => PASS TestRestore"},
		{name: "not a gotest line", contract: pingContract, line: "app ping => ok"},
		{name: "acceptance wrong expected", contract: pingContract, line: "classgo-backup gotest ./internal/backup TestRestore => ok",
			rule: "gotest-form", subject: "pin#1", message: `exactly "PASS TestRestore"`},
		{name: "acceptance other name", contract: pingContract, line: "classgo-backup gotest ./internal/backup TestRestore => PASS TestOther",
			rule: "gotest-form", subject: "pin#1", message: "PASS TestRestore"},
		{name: "acceptance package path", contract: pingContract, line: "classgo-backup gotest internal/backup TestRestore => PASS TestRestore",
			rule: "gotest-form", subject: "pin#1", message: "./ path"},
		{name: "acceptance root package", contract: pingContract, line: "classgo-backup gotest . TestRestore => PASS TestRestore"},
		{name: "acceptance package above the implementation", contract: pingContract, line: "classgo-backup gotest ./../sibling TestRestore => PASS TestRestore",
			rule: "gotest-form", subject: "pin#1", message: `".." segment`},
		{name: "acceptance package with dot-dot inside", contract: pingContract, line: "classgo-backup gotest ./internal/../cmd TestRestore => PASS TestRestore",
			rule: "gotest-form", subject: "pin#1", message: `".." segment`},
		{name: "acceptance package with empty segment", contract: pingContract, line: "classgo-backup gotest ./internal//backup TestRestore => PASS TestRestore",
			rule: "gotest-form", subject: "pin#1", message: "empty"},
		{name: "acceptance package pattern", contract: pingContract, line: "classgo-backup gotest ./... TestRestore => PASS TestRestore",
			rule: "gotest-form", subject: "pin#1", message: "package pattern"},
		{name: "acceptance bare dot-slash", contract: pingContract, line: "classgo-backup gotest ./ TestRestore => PASS TestRestore",
			rule: "gotest-form", subject: "pin#1", message: "./ path"},
		{name: "acceptance subtest", contract: pingContract, line: "classgo-backup gotest ./internal/backup TestRestore/sub => PASS TestRestore/sub",
			rule: "gotest-form", subject: "pin#1", message: "top-level Go test name"},
		{name: "acceptance bare Test name", contract: pingContract, line: "classgo-backup gotest ./internal/backup Test => PASS Test",
			rule: "gotest-form", subject: "pin#1", message: "top-level Go test name"},
		{name: "acceptance name pattern", contract: pingContract, line: "classgo-backup gotest ./internal/backup Test.* => PASS Test.*",
			rule: "gotest-form", subject: "pin#1", message: "top-level Go test name"},
		{name: "acceptance arity", contract: pingContract, line: "classgo-backup gotest ./internal/backup => PASS",
			rule: "gotest-form", subject: "pin#1", message: "exactly PKG NAME"},
		{name: "acceptance nested module path", contract: pingContract, line: "bashy-agent gotest ./yoke/mb TestMB => PASS TestMB"},
		{name: "acceptance dependency import path standing alone", contract: pingContract, line: "bashy-agent gotest github.com/qiangli/yoke/pkg/chat TestJoin => PASS TestJoin"},
		{name: "acceptance import path pattern", contract: pingContract, line: "bashy-agent gotest github.com/qiangli/yoke/... TestJoin => PASS TestJoin",
			rule: "gotest-form", subject: "pin#1", message: "package pattern"},
		{name: "acceptance import path with dot-dot", contract: pingContract, line: "bashy-agent gotest github.com/qiangli/../yoke/pkg TestJoin => PASS TestJoin",
			rule: "gotest-form", subject: "pin#1", message: `".." segment`},
		{name: "acceptance parent path", contract: pingContract, line: "bashy-agent gotest ../yoke/pkg TestJoin => PASS TestJoin",
			rule: "gotest-form", subject: "pin#1", message: `".." segment`},
		{name: "acceptance absolute path", contract: pingContract, line: "bashy-agent gotest /src/yoke/pkg TestJoin => PASS TestJoin",
			rule: "gotest-form", subject: "pin#1", message: "dependency import path"},
		{name: "acceptance hidden first element", contract: pingContract, line: "bashy-agent gotest .hidden/pkg TestJoin => PASS TestJoin",
			rule: "gotest-form", subject: "pin#1", message: "dependency import path"},
		{name: "acceptance standard library", contract: pingContract, line: "bashy-agent gotest strings TestJoin => PASS TestJoin",
			rule: "gotest-form", subject: "pin#1", message: "dependency import path"},
		{name: "acceptance import path bad character", contract: pingContract, line: "bashy-agent gotest github.com/q$x/pkg TestJoin => PASS TestJoin",
			rule: "gotest-form", subject: "pin#1", message: "letters, digits"},
		{name: "contract dependency import path", contract: gotestContract("bashy-agent gotest github.com/qiangli/yoke/pkg/bus TestAdHoc", "PASS TestAdHoc", "0"), line: "app ping => ok"},
		{name: "contract pinned test", contract: gotestContract("classgo-backup gotest ./internal/backup TestRestore", "PASS TestRestore", "0"), line: "app ping => ok"},
		{name: "contract wrong expected", contract: gotestContract("classgo-backup gotest ./internal/backup TestRestore", "FAIL TestRestore", "1"), line: "app ping => ok",
			rule: "gotest-form", subject: "B-01", message: `exactly "PASS TestRestore"`},
		{name: "contract wrong exit", contract: gotestContract("classgo-backup gotest ./internal/backup TestRestore", "PASS TestRestore", "1"), line: "app ping => ok",
			rule: "gotest-form", subject: "B-01", message: "exit 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkWith(t, tc.contract, feat("pin", "verified", "", tc.line), "", nil)
			assertOne(t, got, tc.rule, tc.subject, tc.message)
		})
	}
}

// TestGoTestRulesInsideTheImplementation lints a folder that sits in its
// implementation (a repository with a go.mod at its root): an import path
// must then be in a module that go.mod requires.
func TestGoTestRulesInsideTheImplementation(t *testing.T) {
	gomod := "module github.com/qiangli/bashy\n\ngo 1.27.0\n\n" +
		"require github.com/qiangli/yoke v0.0.0-20261128093718-d5e2b26750e8 // pinned\n\n" +
		"require (\n\t\"github.com/qiangli/yoke/pkg/oci\" v0.0.0-20261128093718-d5e2b26750e8 // indirect\n\tgolang.org/x/net v0.63.0\n)\n\n" +
		"replace github.com/example/replaced => ../replaced\n"
	for _, tc := range []struct {
		name, gomod, line, rule, message string
		noGit                            bool
	}{
		{name: "required module root", gomod: gomod, line: "bashy-agent gotest github.com/qiangli/yoke TestA => PASS TestA"},
		{name: "required module package", gomod: gomod, line: "bashy-agent gotest github.com/qiangli/yoke/pkg/chat TestA => PASS TestA"},
		{name: "required in a block, quoted", gomod: gomod, line: "bashy-agent gotest github.com/qiangli/yoke/pkg/oci/sub TestA => PASS TestA"},
		{name: "nested module path", gomod: gomod, line: "bashy-agent gotest ./yoke/inbox TestA => PASS TestA"},
		{name: "not required", gomod: gomod, line: "bashy-agent gotest github.com/qiangli/other/pkg TestA => PASS TestA",
			rule: "gotest-form", message: "not in a module required by the implementation's go.mod"},
		{name: "prefix is not a module", gomod: gomod, line: "bashy-agent gotest github.com/qiangli/yokeextra/pkg TestA => PASS TestA",
			rule: "gotest-form", message: "not in a module required"},
		{name: "replace only", gomod: gomod, line: "bashy-agent gotest github.com/example/replaced TestA => PASS TestA",
			rule: "gotest-form", message: "not in a module required"},
		{name: "own module", gomod: gomod, line: "bashy-agent gotest github.com/qiangli/bashy/yoke/mb TestA => PASS TestA",
			rule: "gotest-form", message: "own module github.com/qiangli/bashy; name it as a ./ path"},
		{name: "repository without go.mod stands alone", line: "bashy-agent gotest github.com/qiangli/other/pkg TestA => PASS TestA"},
		{name: "no repository stands alone", gomod: gomod, noGit: true, line: "bashy-agent gotest github.com/qiangli/other/pkg TestA => PASS TestA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if !tc.noGit {
				if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if tc.gomod != "" {
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(tc.gomod), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(root, "shapeware", "agent")
			if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(writeSpecDir(t, pingContract, feat("pin", "verified", "", tc.line), nil), dir); err != nil {
				t.Fatal(err)
			}
			got, err := Check(dir)
			if err != nil {
				t.Fatal(err)
			}
			subject := ""
			if tc.rule != "" {
				subject = "pin#1"
			}
			assertOne(t, got, tc.rule, subject, tc.message)
		})
	}
}

// TestGoTestPathSymlinks: inside its implementation the lint resolves a
// ./path through symlinks, as the launcher does, and rejects one that
// leaves the implementation. Standing alone the check stays lexical.
func TestGoTestPathSymlinks(t *testing.T) {
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, line, rule, message string
		noGit                     bool
	}{
		{name: "plain directory", line: "app gotest ./internal/real TestA => PASS TestA"},
		{name: "symlink inside", line: "app gotest ./inside TestA => PASS TestA"},
		{name: "missing path", line: "app gotest ./internal/nope TestA => PASS TestA"},
		{name: "symlink out", line: "app gotest ./escape/pkg TestA => PASS TestA",
			rule: "gotest-form", message: "resolves outside the implementation"},
		{name: "symlink out standing alone is lexical", noGit: true, line: "app gotest ./escape/pkg TestA => PASS TestA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if !tc.noGit {
				if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.invalid/app\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "internal", "real"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "internal", "real"), filepath.Join(root, "inside")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "shapeware", "app")
			if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(writeSpecDir(t, pingContract, feat("pin", "verified", "", tc.line), nil), dir); err != nil {
				t.Fatal(err)
			}
			got, err := Check(dir)
			if err != nil {
				t.Fatal(err)
			}
			subject := ""
			if tc.rule != "" {
				subject = "pin#1"
			}
			assertOne(t, got, tc.rule, subject, tc.message)
		})
	}
}

func TestParseSkillPieces(t *testing.T) {
	meta, body := splitSkill("---\nname: x\nmetadata:\n  version: \"1.2.3\"\n  candidate-files: \"a/*.go b/*\"\nlicense: MIT\n---\n# x\n")
	if meta["version"] != "1.2.3" || meta["candidate-files"] != "a/*.go b/*" || meta["license"] != "" || body != "# x\n" {
		t.Fatalf("meta=%v body=%q", meta, body)
	}
	issues := parseKnownIssues(knownIssuesSection("## Known issues\n\n- KI-1 feature:a no bracket -> x\n  fixed-by: f\n```\n- KI-5 [fixed] fenced\n```\n- KI-2 [Workaround] feature:b s -> a\n## Next\n- KI-3 [open] x\n"))
	if len(issues) != 2 || issues[0] != (knownIssue{id: "KI-1", status: "open", fixedBy: "f"}) || issues[1] != (knownIssue{id: "KI-2", status: "workaround"}) {
		t.Fatalf("issues = %+v", issues)
	}
}

// assertOne checks for no findings (rule "") or exactly one matching finding.
func assertOne(t *testing.T, got []Finding, rule, subject, message string) {
	t.Helper()
	if rule == "" {
		if len(got) != 0 {
			t.Fatalf("want no findings, got %+v", got)
		}
		return
	}
	if len(got) != 1 || got[0].Rule != rule || got[0].Subject != subject || got[0].Severity != "error" || !strings.Contains(got[0].Message, message) {
		t.Fatalf("got %+v, want one error %s %s containing %q", got, rule, subject, message)
	}
}
