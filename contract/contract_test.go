package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform/contract"
)

type recordedFixture struct {
	PassSequence []struct {
		Name       string          `json:"name"`
		Exit       int             `json:"exit"`
		Stdout     string          `json:"stdout"`
		WantStatus contract.Status `json:"want_status"`
	} `json:"pass_sequence"`
	NegativeCases []struct {
		Case       string            `json:"case"`
		Name       string            `json:"name"`
		Bindings   map[string]string `json:"bindings"`
		Exit       int               `json:"exit"`
		Stdout     string            `json:"stdout"`
		WantStatus contract.Status   `json:"want_status"`
	} `json:"negative_cases"`
}

func loadFormV1Contract(t *testing.T) []contract.Assertion {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "form-v1", "CONTRACT.md"))
	if err != nil {
		t.Fatalf("open fixture CONTRACT.md: %v", err)
	}
	defer f.Close()
	assertions, err := contract.Parse(f)
	if err != nil {
		t.Fatalf("Parse form-v1 CONTRACT.md: %v", err)
	}
	return assertions
}

func TestFormV1FixtureParseAndReplay(t *testing.T) {
	assertions := loadFormV1Contract(t)
	if len(assertions) != 9 {
		t.Fatalf("got %d assertions, want 9", len(assertions))
	}

	wantNames := []string{"F-01", "F-02", "F-03", "F-04", "F-05", "F-06", "F-07", "F-08", "F-09"}
	wantJSON := []bool{true, true, true, true, true, false, true, true, false}
	wantExit := []int{0, 0, 0, 0, 0, 1, 0, 0, 0}

	byName := make(map[string]contract.Assertion, len(assertions))
	for i, a := range assertions {
		if a.Name != wantNames[i] {
			t.Errorf("assertion[%d].Name = %q, want %q", i, a.Name, wantNames[i])
		}
		if a.Prose == "" {
			t.Errorf("%s: empty prose", a.Name)
		}
		if len(a.Commands) != 1 || a.Commands[0] == "" {
			t.Errorf("%s: unexpected commands %v", a.Name, a.Commands)
		}
		if a.Exit != wantExit[i] {
			t.Errorf("%s: Exit = %d, want %d", a.Name, a.Exit, wantExit[i])
		}
		if a.IsJSON() != wantJSON[i] {
			t.Errorf("%s: IsJSON() = %v, want %v", a.Name, a.IsJSON(), wantJSON[i])
		}
		byName[a.Name] = a
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "form-v1", "outputs.json"))
	if err != nil {
		t.Fatalf("read outputs.json: %v", err)
	}
	var fix recordedFixture
	if err := json.Unmarshal(raw, &fix); err != nil {
		t.Fatalf("unmarshal outputs.json: %v", err)
	}

	// Sequential replay across F-01..F-09 with shared bindings and command substitution.
	bindings := contract.Bindings{}
	for _, step := range fix.PassSequence {
		a, ok := byName[step.Name]
		if !ok {
			t.Fatalf("unknown assertion %q in fixture", step.Name)
		}
		cmd, err := contract.Substitute(a.Commands[0], bindings)
		if err != nil {
			t.Fatalf("%s: Substitute failed before check: %v", step.Name, err)
		}
		if strings.Contains(cmd, "{ID}") || strings.Contains(cmd, "{PORT}") {
			t.Fatalf("%s: unresolved placeholder in command %q", step.Name, cmd)
		}
		res := contract.Check(a, step.Stdout, step.Exit, bindings)
		if res.Name != step.Name {
			t.Errorf("%s: Result.Name = %q", step.Name, res.Name)
		}
		if res.Status != step.WantStatus {
			t.Errorf("%s: got %s (%s), want %s", step.Name, res.Status, res.Detail, step.WantStatus)
		}
	}

	wantBindings := map[string]string{
		"ID":   "intake",
		"PORT": "43210",
		"KEY":  "ownerKey123456",
		"TIME": "2099-12-31T23:59:59Z",
	}
	for k, want := range wantBindings {
		if got := bindings[k]; got != want {
			t.Errorf("binding[%q] = %q, want %q", k, got, want)
		}
	}

	// Negative recorded cases against form-v1 CONTRACT.md.
	for _, tc := range fix.NegativeCases {
		t.Run(tc.Case, func(t *testing.T) {
			a, ok := byName[tc.Name]
			if !ok {
				t.Fatalf("unknown assertion %q", tc.Name)
			}
			b := contract.Bindings(tc.Bindings).Clone()
			before := b.Clone()
			res := contract.Check(a, tc.Stdout, tc.Exit, b)
			if res.Status != tc.WantStatus {
				t.Errorf("got %s (%s), want %s", res.Status, res.Detail, tc.WantStatus)
			}
			// Failed check must not mutate bindings.
			if len(b) != len(before) {
				t.Fatalf("bindings mutated on FAIL: before=%v after=%v", before, b)
			}
			for k, v := range before {
				if b[k] != v {
					t.Errorf("binding[%q] mutated from %q to %q", k, v, b[k])
				}
			}
		})
	}
}

func TestPlaceholders(t *testing.T) {
	tests := []struct {
		name         string
		expected     string
		initial      contract.Bindings
		stdout       string
		wantStatus   contract.Status
		wantBindings map[string]string
	}{
		{
			name:         "bind_ID_valid",
			expected:     `{"id":"{ID}"}`,
			stdout:       `{"id":"form_01-abc"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"ID": "form_01-abc"},
		},
		{
			name:       "reject_ID_leading_hyphen",
			expected:   `{"id":"{ID}"}`,
			stdout:     `{"id":"-bad"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:       "reject_ID_too_long",
			expected:   `{"id":"{ID}"}`,
			stdout:     `{"id":"` + strings.Repeat("a", 65) + `"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:         "ID_consistent_later_match",
			expected:     `{"id":"{ID}","url":"http://localhost:8080/?id={ID}"}`,
			initial:      contract.Bindings{"ID": "intake"},
			stdout:       `{"id":"intake","url":"http://localhost:8080/?id=intake"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"ID": "intake"},
		},
		{
			name:       "ID_inconsistent_later_mismatch",
			expected:   `{"id":"{ID}"}`,
			initial:    contract.Bindings{"ID": "intake"},
			stdout:     `{"id":"different"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:         "bind_PORT_valid",
			expected:     `{"url":"http://localhost:{PORT}/"}`,
			stdout:       `{"url":"http://localhost:8080/"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"PORT": "8080"},
		},
		{
			name:       "reject_PORT_single_digit",
			expected:   `{"url":"http://localhost:{PORT}/"}`,
			stdout:     `{"url":"http://localhost:8/"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:       "reject_PORT_six_digits",
			expected:   `{"url":"http://localhost:{PORT}/"}`,
			stdout:     `{"url":"http://localhost:123456/"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:         "bind_TIME_rfc3339_utc",
			expected:     `{"close_at":"{TIME}"}`,
			stdout:       `{"close_at":"2099-12-31T23:59:59Z"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"TIME": "2099-12-31T23:59:59Z"},
		},
		{
			name:         "bind_TIME_rfc3339_offset_and_frac",
			expected:     `{"close_at":"{TIME}"}`,
			stdout:       `{"close_at":"2026-11-25T13:21:24.123456-08:00"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"TIME": "2026-11-25T13:21:24.123456-08:00"},
		},
		{
			name:       "reject_TIME_invalid",
			expected:   `{"close_at":"{TIME}"}`,
			stdout:     `{"close_at":"tomorrow"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:         "bind_KEY_valid",
			expected:     `{"ownerKey":"{KEY}"}`,
			stdout:       `{"ownerKey":"abcd_1234-WXYZ"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"KEY": "abcd_1234-WXYZ"},
		},
		{
			name:       "reject_KEY_too_short",
			expected:   `{"ownerKey":"{KEY}"}`,
			stdout:     `{"ownerKey":"short77"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:         "repeated_unbound_placeholder_same_string_pass",
			expected:     `{"pair":"{ID}:{ID}"}`,
			stdout:       `{"pair":"alpha-1:alpha-1"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"ID": "alpha-1"},
		},
		{
			name:       "repeated_unbound_placeholder_same_string_fail",
			expected:   `{"pair":"{ID}:{ID}"}`,
			stdout:     `{"pair":"alpha-1:beta-2"}`,
			wantStatus: contract.FAIL,
		},
		{
			name:         "id_bound_before_url_within_same_object",
			expected:     `{"url":"http://localhost:{PORT}/?id={ID}","id":"{ID}"}`,
			stdout:       `{"url":"http://localhost:9000/?id=myform","id":"myform"}`,
			wantStatus:   contract.PASS,
			wantBindings: map[string]string{"ID": "myform", "PORT": "9000"},
		},
		{
			name:       "id_mismatch_against_url_within_same_object",
			expected:   `{"url":"http://localhost:{PORT}/?id={ID}","id":"{ID}"}`,
			stdout:     `{"url":"http://localhost:9000/?id=other","id":"myform"}`,
			wantStatus: contract.FAIL,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := contract.Assertion{
				Name:     tc.name,
				Commands: []string{"form test --json"},
				Expected: tc.expected,
				Exit:     0,
			}
			b := tc.initial.Clone()
			res := contract.Check(a, tc.stdout, 0, b)
			if res.Status != tc.wantStatus {
				t.Fatalf("got %s (%s), want %s", res.Status, res.Detail, tc.wantStatus)
			}
			if tc.wantStatus == contract.PASS {
				for k, want := range tc.wantBindings {
					if b[k] != want {
						t.Errorf("binding[%q] = %q, want %q", k, b[k], want)
					}
				}
			}
		})
	}
}

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		b       contract.Bindings
		want    string
		wantErr string
	}{
		{
			name: "all_placeholders_bound",
			cmd:  "form open {ID} --port {PORT} --key {KEY} --at {TIME}",
			b:    contract.Bindings{"ID": "f1", "PORT": "8080", "KEY": "secretKey1", "TIME": "2099-01-01T00:00:00Z"},
			want: "form open f1 --port 8080 --key secretKey1 --at 2099-01-01T00:00:00Z",
		},
		{
			name: "unknown_braces_untouched",
			cmd:  `form add --definition '{"id":"x"}' {ID}`,
			b:    contract.Bindings{"ID": "f1"},
			want: `form add --definition '{"id":"x"}' f1`,
		},
		{
			name:    "missing_sorted_deduplicated",
			cmd:     "form open {PORT} {ID} {PORT}",
			b:       contract.Bindings{},
			want:    "form open {PORT} {ID} {PORT}",
			wantErr: "unbound placeholder(s) in command: ID, PORT",
		},
		{
			name:    "partial_substitution_on_error",
			cmd:     "form open {ID} --key {KEY}",
			b:       contract.Bindings{"ID": "f1"},
			want:    "form open f1 --key {KEY}",
			wantErr: "unbound placeholder(s) in command: KEY",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := contract.Substitute(tc.cmd, tc.b)
			if got != tc.want {
				t.Errorf("rendered = %q, want %q", got, tc.want)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != tc.wantErr {
					t.Errorf("err = %v, want %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestOpenFieldsEllipsis(t *testing.T) {
	tests := []struct {
		name       string
		expected   string
		stdout     string
		wantStatus contract.Status
	}{
		{
			name:       "open_object_quoted_key_allows_extras",
			expected:   `{"id":"x","...":"..."}`,
			stdout:     `{"id":"x","extra1":1,"extra2":{"nested":true}}`,
			wantStatus: contract.PASS,
		},
		{
			name:       "open_object_quoted_key_still_requires_named_keys",
			expected:   `{"id":"x","...":"..."}`,
			stdout:     `{"extra1":1}`,
			wantStatus: contract.FAIL,
		},
		{
			name:       "open_object_cert_trailing_ellipsis",
			expected:   `{"id":"x", ...}`,
			stdout:     `{"id":"x","extra":"ok"}`,
			wantStatus: contract.PASS,
		},
		{
			name:       "open_object_empty_ellipsis",
			expected:   `{...}`,
			stdout:     `{"anything":123}`,
			wantStatus: contract.PASS,
		},
		{
			name:       "closed_object_rejects_extra_key",
			expected:   `{"id":"x"}`,
			stdout:     `{"id":"x","extra":1}`,
			wantStatus: contract.FAIL,
		},
		{
			name:       "scalar_ellipsis_matches_string",
			expected:   `{"v":"..."}`,
			stdout:     `{"v":"any string"}`,
			wantStatus: contract.PASS,
		},
		{
			name:       "scalar_ellipsis_matches_number_bool_null_object_array",
			expected:   `{"a":"...","b":"...","c":"...","d":"...","e":"..."}`,
			stdout:     `{"a":42,"b":false,"c":null,"d":{"k":1},"e":[1,2]}`,
			wantStatus: contract.PASS,
		},
		{
			name:       "array_open_tail_quoted_allows_more_items",
			expected:   `[{"id":"a"},"..."]`,
			stdout:     `[{"id":"a"},{"id":"b"},{"id":"c"}]`,
			wantStatus: contract.PASS,
		},
		{
			name:       "array_open_tail_cert_syntax",
			expected:   `[{"id":"a"}, ...]`,
			stdout:     `[{"id":"a"},{"id":"b"}]`,
			wantStatus: contract.PASS,
		},
		{
			name:       "array_open_tail_rejects_too_few",
			expected:   `[{"id":"a"},{"id":"b"},"..."]`,
			stdout:     `[{"id":"a"}]`,
			wantStatus: contract.FAIL,
		},
		{
			name:       "closed_array_rejects_extra_item",
			expected:   `[1, 2]`,
			stdout:     `[1, 2, 3]`,
			wantStatus: contract.FAIL,
		},
		{
			name:       "number_int_float_equivalence",
			expected:   `{"n":1}`,
			stdout:     `{"n":1.0}`,
			wantStatus: contract.PASS,
		},
		{
			name:       "bool_does_not_match_number",
			expected:   `{"b":true}`,
			stdout:     `{"b":1}`,
			wantStatus: contract.FAIL,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := contract.Assertion{
				Name:     tc.name,
				Commands: []string{"form show --json"},
				Expected: tc.expected,
				Exit:     0,
			}
			res := contract.Check(a, tc.stdout, 0, nil)
			if res.Status != tc.wantStatus {
				t.Errorf("got %s (%s), want %s", res.Status, res.Detail, tc.wantStatus)
			}
		})
	}
}

func TestNormalizeAndLoopback(t *testing.T) {
	loopbackV4 := strings.Join([]string{"127", "0", "0", "1"}, ".")

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "ipv4_loopback_to_localhost",
			raw:  `{"url":"http://` + loopbackV4 + `:8123/?id=intake"}`,
			want: `{"url":"http://localhost:8123/?id=intake"}`,
		},
		{
			name: "ipv6_loopback_to_localhost",
			raw:  `{"url":"http://[::1]:8123/?id=intake"}`,
			want: `{"url":"http://localhost:8123/?id=intake"}`,
		},
		{
			name: "strip_ansi_csi_osc_apc",
			raw:  "\x1b[1;32mhello\x1b[0m \x1b]0;title\x07\x1b_apc\x1b\\world\r\n",
			want: "hello world",
		},
		{
			name: "strip_bashy_hint_json_line",
			raw:  "{\"schema_version\": \"bashy-hint-v1\", \"kind\": \"hint\"}\n{\"ok\":true}\n",
			want: `{"ok":true}`,
		},
		{
			name: "strip_bashy_skill_advisor_line",
			raw:  "bashy: claude detected — run `bashy skill show bashy`\n{\"ok\":true}\n",
			want: `{"ok":true}`,
		},
		{
			name: "crlf_and_cr_normalized",
			raw:  "  line1\r\nline2\rline3  \r\n",
			want: "line1\nline2\nline3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := contract.Normalize(tc.raw); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}

	// End-to-end Check with IPv4 loopback URL matching a localhost contract template.
	a := contract.Assertion{
		Name:     "L-01",
		Commands: []string{"form open intake --json"},
		Expected: `{"id":"{ID}","url":"http://localhost:{PORT}/?id={ID}"}`,
		Exit:     0,
	}
	b := contract.Bindings{}
	rawOut := `{"id":"intake","url":"http://` + loopbackV4 + `:54321/?id=intake"}`
	res := contract.Check(a, rawOut, 0, b)
	if res.Status != contract.PASS {
		t.Fatalf("loopback Check failed: %s", res.Detail)
	}
	if b["ID"] != "intake" || b["PORT"] != "54321" {
		t.Errorf("unexpected bindings after loopback Check: %v", b)
	}
}

func TestPlainTextOnlyWhereGiven(t *testing.T) {
	tests := []struct {
		name       string
		assertion  contract.Assertion
		stdout     string
		exit       int
		wantStatus contract.Status
	}{
		{
			name: "omitted_expected_accepts_any_plain_text",
			assertion: contract.Assertion{
				Name:     "P-01",
				Commands: []string{"form list"},
				Expected: "",
				Exit:     0,
			},
			stdout:     "any human table layout\nrow 1\nrow 2\n",
			exit:       0,
			wantStatus: contract.PASS,
		},
		{
			name: "omitted_expected_accepts_empty_stdout",
			assertion: contract.Assertion{
				Name:     "P-02",
				Commands: []string{"form list"},
				Expected: "",
				Exit:     0,
			},
			stdout:     "",
			exit:       0,
			wantStatus: contract.PASS,
		},
		{
			name: "omitted_expected_still_checks_exit",
			assertion: contract.Assertion{
				Name:     "P-03",
				Commands: []string{"form list"},
				Expected: "",
				Exit:     0,
			},
			stdout:     "some output",
			exit:       1,
			wantStatus: contract.FAIL,
		},
		{
			name: "given_plain_text_with_wildcard_matches",
			assertion: contract.Assertion{
				Name:     "P-04",
				Commands: []string{"form add --json"},
				Expected: "error: Secret fields are refused here...",
				Exit:     1,
			},
			stdout:     "error: Secret fields are refused here (api_key)\nmore details",
			exit:       1,
			wantStatus: contract.PASS,
		},
		{
			name: "given_plain_text_mismatch_fails",
			assertion: contract.Assertion{
				Name:     "P-05",
				Commands: []string{"form add --json"},
				Expected: "error: Secret fields are refused here...",
				Exit:     1,
			},
			stdout:     "error: different message",
			exit:       1,
			wantStatus: contract.FAIL,
		},
		{
			name: "given_plain_text_with_placeholder_binds",
			assertion: contract.Assertion{
				Name:     "P-06",
				Commands: []string{"form status"},
				Expected: "form {ID} listening on :{PORT}",
				Exit:     0,
			},
			stdout:     "form alpha listening on :9090",
			exit:       0,
			wantStatus: contract.PASS,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := contract.Bindings{}
			res := contract.Check(tc.assertion, tc.stdout, tc.exit, b)
			if res.Status != tc.wantStatus {
				t.Errorf("got %s (%s), want %s", res.Status, res.Detail, tc.wantStatus)
			}
			if tc.name == "given_plain_text_with_placeholder_binds" {
				if b["ID"] != "alpha" || b["PORT"] != "9090" {
					t.Errorf("bindings = %v, want ID=alpha PORT=9090", b)
				}
			}
		})
	}
}

func TestExitMismatch(t *testing.T) {
	tests := []struct {
		name     string
		wantExit int
		gotExit  int
		expected string
		stdout   string
	}{
		{
			name:     "expected_0_got_1",
			wantExit: 0,
			gotExit:  1,
			expected: `{"id":"{ID}"}`,
			stdout:   `{"id":"intake"}`,
		},
		{
			name:     "expected_1_got_0",
			wantExit: 1,
			gotExit:  0,
			expected: "error: refused...",
			stdout:   "error: refused",
		},
		{
			name:     "expected_0_got_127",
			wantExit: 0,
			gotExit:  127,
			expected: "",
			stdout:   "command not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := contract.Assertion{
				Name:     tc.name,
				Commands: []string{"form cmd --json"},
				Expected: tc.expected,
				Exit:     tc.wantExit,
			}
			b := contract.Bindings{}
			res := contract.Check(a, tc.stdout, tc.gotExit, b)
			if res.Status != contract.FAIL {
				t.Fatalf("got %s, want FAIL", res.Status)
			}
			if !strings.Contains(res.Detail, "exit") {
				t.Errorf("expected exit detail, got %q", res.Detail)
			}
			if len(b) != 0 {
				t.Errorf("bindings mutated on exit mismatch: %v", b)
			}
		})
	}
}

func TestMalformedContracts(t *testing.T) {
	tests := []struct {
		name    string
		md      string
		wantErr string
	}{
		{
			name: "missing_console_block_eof",
			md: `### F-01
Some prose without a console block.
exit: 0
`,
			wantErr: "F-01: missing ```console block",
		},
		{
			name: "missing_console_block_before_next_heading",
			md: `### F-01
Prose only.

### F-02
Valid second assertion.
` + "```console\n$ form list\n```\nexit: 0\n",
			wantErr: "F-01: missing ```console block",
		},
		{
			name: "unterminated_console_block",
			md: "### F-01\nProse.\n```console\n$ form list\n",
			wantErr: "F-01: unterminated ```console block",
		},
		{
			name:    "missing_exit_line_eof",
			md:      "### F-01\nProse.\n```console\n$ form list\n```\n",
			wantErr: "F-01: missing 'exit: N' line",
		},
		{
			name: "missing_exit_line_before_next_heading",
			md: "### F-01\nProse.\n```console\n$ form list\n```\n\n### F-02\nSecond.\n```console\n$ form list\n```\nexit: 0\n",
			wantErr: "F-01: missing 'exit: N' line",
		},
		{
			name:    "missing_dollar_command",
			md:      "### F-01\nProse.\n```console\n{\"id\":\"x\"}\n```\nexit: 0\n",
			wantErr: "F-01: console block must begin with '$ <command>'",
		},
		{
			name:    "empty_console_block",
			md:      "### F-01\nProse.\n```console\n```\nexit: 0\n",
			wantErr: "F-01: console block must begin with '$ <command>'",
		},
		{
			name:    "empty_command_after_dollar",
			md:      "### F-01\nProse.\n```console\n$   \n```\nexit: 0\n",
			wantErr: "F-01: empty command in console block",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := contract.Parse(strings.NewReader(tc.md))
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}

	// Malformed expected JSON inside a parsed assertion yields FAIL in Check.
	t.Run("invalid_contract_json", func(t *testing.T) {
		a := contract.Assertion{
			Name:     "BAD-01",
			Commands: []string{"form show --json"},
			Expected: `{"id": broken}`,
			Exit:     0,
		}
		res := contract.Check(a, `{"id":"x"}`, 0, nil)
		if res.Status != contract.FAIL || !strings.Contains(res.Detail, "invalid contract JSON") {
			t.Errorf("got %+v, want FAIL with invalid contract JSON", res)
		}
	})
}

func TestParseMultiCommandBlock(t *testing.T) {
	md := `### M-01
Run two setup commands then check JSON output.
` + "```console" + `
$ form add --id one --title One
$ form show one --json
{"id":"one","status":"pending"}
` + "```" + `
exit: 0
`
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(assertions) != 1 {
		t.Fatalf("got %d assertions, want 1", len(assertions))
	}
	a := assertions[0]
	if len(a.Commands) != 2 || a.Commands[0] != "form add --id one --title One" || a.Commands[1] != "form show one --json" {
		t.Fatalf("unexpected commands: %#v", a.Commands)
	}
	if !a.IsJSON() {
		t.Errorf("expected IsJSON() = true")
	}
	res := contract.Check(a, `{"id":"one","status":"pending"}`, 0, nil)
	if res.Status != contract.PASS {
		t.Errorf("Check failed: %s", res.Detail)
	}
}
