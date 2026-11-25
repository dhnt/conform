package contract_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform/contract"
)

func fakePageServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	page, err := os.ReadFile(filepath.Join("testdata", "fakecli", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(page)
	}))
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	return server, port
}

func uiContract(definition string) string {
	// CLI output is normalized from 127.0.0.1 to localhost for matching;
	// the UI GET keeps the literal httptest address.
	return `### F-01
Create a form and bind its URL.
` + "```console\n" + `$ form add --id form1 --json
{"id":"{ID}","inviteURL":"http://localhost:{PORT}/?id={ID}&invite=1","opened":true,"openedURL":"http://localhost:{PORT}/?id={ID}","ownerKey":"{KEY}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}
` + "```\n" + `exit: 0

### U-01
Inspect the page.
` + "```ui\n" + `GET http://127.0.0.1:{PORT}/?id={ID}
` + definition + "\n```\n"
}

func TestUIAssertionFromFakeCLI(t *testing.T) {
	_, port := fakePageServer(t)
	definition := `{"fields":[{"id":"name","label":"Full name","kind":"text","required":true}],"buttons":["Save","Submit"]}`
	for _, tc := range []struct {
		name, definition string
		want             contract.Status
	}{
		{"pass", definition, contract.PASS},
		{"missing_field", `{"fields":[{"id":"name","label":"Full name","required":true},{"id":"email","label":"Email"}],"buttons":["Save"]}`, contract.FAIL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertions, err := contract.Parse(strings.NewReader(uiContract(tc.definition)))
			if err != nil {
				t.Fatal(err)
			}
			if assertions[0].Kind != "cli" || assertions[1].Kind != "ui" || assertions[1].URL == "" {
				t.Fatalf("kinds: %+v", assertions)
			}
			results := contract.Run(context.Background(), assertions, contract.Env{Dir: t.TempDir(), PathPrefix: fakeCLIBinDir(t), Extra: []string{"FAKE_HOST=127.0.0.1", "FAKE_PORT=" + port}})
			if results[0].Status != contract.PASS || results[1].Status != tc.want {
				t.Fatalf("results: %+v", results)
			}
		})
	}
}

func TestUIURLFailures(t *testing.T) {
	def := `{"fields":[],"buttons":[]}`
	for _, tc := range []struct {
		name, url string
		want      contract.Status
	}{
		{"unbound", "http://localhost:{PORT}/", contract.FAIL},
		{"nonloopback", "http://example.com/", contract.INCONCLUSIVE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := fmt.Sprintf("### U-01\nInspect.\n```ui\nGET %s\n%s\n```\n", tc.url, def)
			assertions, err := contract.Parse(strings.NewReader(md))
			if err != nil {
				t.Fatal(err)
			}
			got := contract.Run(context.Background(), assertions, contract.Env{})
			if len(got) != 1 || got[0].Status != tc.want {
				t.Fatalf("results: %+v", got)
			}
		})
	}
}

func TestUINon200Fails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "gone", http.StatusNotFound) }))
	defer server.Close()
	md := fmt.Sprintf("### U-01\nInspect.\n```ui\nGET %s/\n{\"fields\":[],\"buttons\":[]}\n```\n", server.URL)
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatal(err)
	}
	got := contract.Run(context.Background(), assertions, contract.Env{})
	if got[0].Status != contract.FAIL || !strings.Contains(got[0].Detail, "404") {
		t.Fatalf("result: %+v", got[0])
	}
}

func TestFreshHomeDoesNotLeakBetweenRuns(t *testing.T) {
	md := "### H-01\nCheck home.\n```console\n$ form home-state\nnew\n```\nexit: 0\n"
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatal(err)
	}
	env := contract.Env{Dir: t.TempDir(), PathPrefix: fakeCLIBinDir(t), FreshHome: true}
	for i := 0; i < 2; i++ {
		got := contract.Run(context.Background(), assertions, env)
		if got[0].Status != contract.PASS {
			t.Fatalf("run %d: %+v", i, got)
		}
	}
}

func TestResetCmdBeforeEachRun(t *testing.T) {
	work := t.TempDir()
	md := "### H-01\nCheck state.\n```console\n$ test ! -e state && touch state && echo clean\nclean\n```\nexit: 0\n"
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatal(err)
	}
	env := contract.Env{Dir: work, ResetCmd: "rm -f state"}
	for i := 0; i < 2; i++ {
		got := contract.Run(context.Background(), assertions, env)
		if got[0].Status != contract.PASS {
			t.Fatalf("run %d: %+v", i, got)
		}
	}
}

func TestUIPageNotServedFails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close() // nothing listens on the advertised port any more
	md := fmt.Sprintf("### U-01\nInspect.\n```ui\nGET %s/\n{\"fields\":[],\"buttons\":[]}\n```\n", url)
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatal(err)
	}
	got := contract.Run(context.Background(), assertions, contract.Env{})
	if got[0].Status != contract.FAIL || !strings.Contains(got[0].Detail, "not served") {
		t.Fatalf("result: %+v", got[0])
	}
}
