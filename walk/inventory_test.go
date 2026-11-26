package walk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"golang.org/x/net/html"
)

// dom exposes only the browser primitives used by walker.js. In particular,
// it does not provide querySelector or form-specific conveniences.
func dom(n *html.Node) map[string]any { id := 0; return domChild(n, nil, &id) }

func domChild(n *html.Node, parent map[string]any, id *int) map[string]any {
	*id = *id + 1
	o := map[string]any{"childNodes": []any{}, "parentNode": parent, "walkID": *id}
	if n.Type == html.ElementNode {
		o["tagName"] = strings.ToUpper(n.Data)
		o["getAttribute"] = func(key string) any {
			for _, a := range n.Attr {
				if a.Key == key {
					return a.Val
				}
			}
			return nil
		}
	}
	if n.Type == html.TextNode {
		o["textContent"] = n.Data
	}
	children := []any{}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, domChild(c, o, id))
	}
	o["childNodes"] = children
	return o
}

func TestParity(t *testing.T) {
	var deskFields []Field
	var plainFields []Field
	fixtures, err := filepath.Glob("testdata/*.html")
	if err != nil || len(fixtures) < 2 {
		t.Fatalf("fixtures: %v, %v", fixtures, err)
	}
	for _, path := range fixtures {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Inventory(strings.NewReader(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			root, err := html.Parse(strings.NewReader(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			vm := goja.New()
			if err := vm.Set("document", dom(root)); err != nil {
				t.Fatal(err)
			}
			v, err := vm.RunString(JS + "\nJSON.stringify(walkInventory(document))")
			if err != nil {
				t.Fatal(err)
			}
			var want InventoryResult
			if err := json.Unmarshal([]byte(v.String()), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Go/JS mismatch\nGo: %#v\nJS: %#v", got, want)
			}
			if path == "testdata/desk_form.html" || path == "testdata/plain.html" {
				if path == "testdata/desk_form.html" {
					deskFields = got.Fields
				} else {
					plainFields = got.Fields
				}
				definition := []Field{
					{ID: "name", Label: "Full name", Kind: "text", Required: true},
					{ID: "bio", Label: "Short bio", Kind: "textarea"},
					{ID: "contact", Label: "Preferred contact", Kind: "choice", Required: true, Options: []string{"email", "phone", "post"}},
					{ID: "topics", Label: "Topics", Kind: "multi", Options: []string{"news", "events", "offers"}},
					{ID: "subscribe", Label: "Subscribe?", Kind: "yes-no", Required: true},
					{ID: "avatar", Label: "Avatar file", Kind: "file"},
				}
				if ok, why := CompareWithDefinition(got, definition, []string{"Save (partial)", "Submit answers"}); !ok {
					t.Fatal(why)
				}
				got.Buttons = got.Buttons[1:]
				if ok, _ := CompareWithDefinition(got, definition, []string{"Save (partial)", "Submit answers"}); ok {
					t.Fatal("missing Save button accepted")
				}
			}
			if path == "testdata/accessibility.html" {
				want := []Field{
					{ID: "code", Label: "Code", Kind: "text", Required: true, Options: []string{}},
					{ID: "description", Label: "Description", Kind: "textarea", Options: []string{}},
					{ID: "contact", Label: "Contact method", Kind: "yes-no", Required: true, Options: []string{"Yes", "No"}},
					{ID: "agree", Label: "Agree", Kind: "yes-no", Options: []string{"Agree"}},
					{ID: "colors", Label: "Colors", Kind: "multi", Options: []string{"Red", "Blue"}},
				}
				if !reflect.DeepEqual(got.Fields, want) {
					t.Fatalf("accessible fields: %#v", got.Fields)
				}
				if ok, why := CompareWithDefinition(got, want, []string{"send"}); !ok {
					t.Fatal(why)
				}
			}
			if path == "testdata/single-checkbox.html" {
				def := []Field{{ID: "name", Label: "Full name", Kind: "text", Required: true}, {ID: "subscribe", Label: "Subscribe?", Kind: "yes-no", Required: true}}
				if ok, why := CompareWithDefinition(got, def, []string{"Submit answers"}); !ok {
					t.Fatalf("single checkbox yes-no: %s (%#v)", why, got.Fields)
				}
			}
			if path == "testdata/edges.html" {
				if len(got.Fields) != 6 || got.Fields[0].Label != "External" || !got.Fields[0].Required ||
					got.Fields[1].Label != "Wrapped" || got.Fields[3].Kind != "yes-no" ||
					!reflect.DeepEqual(got.Fields[4].Options, []string{"A", "Fallback"}) || got.Fields[5].Kind != "textarea" {
					t.Fatalf("edge inventory: %#v", got)
				}
			}
		})
	}
	if !reflect.DeepEqual(deskFields, plainFields) {
		t.Fatalf("plain fields differ from desk form\ndesk: %#v\nplain: %#v", deskFields, plainFields)
	}
}

func TestYesNoFieldNeedsBothOptions(t *testing.T) {
	def := []Field{{ID: "subscribe", Label: "Subscribe?", Kind: "yes-no", Required: true}}
	field := func(opts ...string) InventoryResult {
		return InventoryResult{Fields: []Field{{ID: "subscribe", Label: "Subscribe?", Kind: "yes-no", Required: true, Options: opts}}}
	}
	for _, tc := range []struct {
		opts []string
		ok   bool
	}{
		{nil, true}, // single checkbox
		{[]string{"Yes", "No"}, true},
		{[]string{"no", "yes"}, true},
		{[]string{"yes"}, false},
		{[]string{"no"}, false},
		{[]string{"yes", "maybe"}, false},
	} {
		if ok, why := CompareWithDefinition(field(tc.opts...), def, nil); ok != tc.ok {
			t.Errorf("options %q: ok=%t (%s), want %t", tc.opts, ok, why, tc.ok)
		}
	}
}
