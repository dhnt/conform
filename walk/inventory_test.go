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
func dom(n *html.Node) map[string]any {
	o := map[string]any{"childNodes": []any{}}
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
		children = append(children, dom(c))
	}
	o["childNodes"] = children
	return o
}

func TestParity(t *testing.T) {
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
			if path == "testdata/desk_form.html" {
				definition := []Field{
					{ID: "name", Label: "Full name", Kind: "text", Required: true},
					{ID: "bio", Label: "Short bio", Kind: "textarea"},
					{ID: "contact", Label: "Preferred contact", Kind: "choice", Required: true, Options: []string{"email", "phone", "post"}},
					{ID: "topics", Label: "Topics", Kind: "multi", Options: []string{"news", "events", "offers"}},
					{ID: "subscribe", Label: "Subscribe?", Kind: "yes-no", Required: true},
					{ID: "avatar", Label: "Avatar file", Kind: "file"},
				}
				if ok, why := CompareWithDefinition(got, definition); !ok {
					t.Fatal(why)
				}
				got.HasSaveButton = false
				if ok, _ := CompareWithDefinition(got, definition); ok {
					t.Fatal("missing Save button accepted")
				}
			}
			if path == "testdata/edges.html" {
				if len(got.Fields) != 6 || got.Fields[0].Label != "External" || !got.Fields[0].Required ||
					got.Fields[1].Label != "Wrapped" || got.Fields[3].Kind != "yes-no" ||
					!reflect.DeepEqual(got.Fields[4].Options, []string{"a", "Fallback"}) || got.Fields[5].Kind != "textarea" {
					t.Fatalf("edge inventory: %#v", got)
				}
			}
		})
	}
}
