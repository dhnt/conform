package walk

import (
	_ "embed"
	"io"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// JS is the browser version of Inventory. Call walkInventory(document).
//
//go:embed walker.js
var JS string

type Field struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Required bool     `json:"required"`
	Options  []string `json:"options"`
}

type InventoryResult struct {
	Fields          []Field  `json:"fields"`
	HasSaveButton   bool     `json:"has_save_button"`
	HasSubmitButton bool     `json:"has_submit_button"`
	Buttons         []string `json:"buttons"`
}

// Inventory extracts the visible form inventory from parsed HTML.
func Inventory(r io.Reader) (InventoryResult, error) {
	root, err := html.Parse(r)
	if err != nil {
		return InventoryResult{}, err
	}
	out := InventoryResult{Fields: []Field{}, Buttons: []string{}}
	walk(root, false, func(form *html.Node) {
		if tag(form) != "form" || hidden(form) {
			return
		}
		visit(form, func(n *html.Node) {
			if n != form && tag(n) == "form" {
				return
			}
			if hidden(n) {
				return
			}
			if tag(n) == "button" {
				label := clean(visibleText(n, false))
				out.Buttons = append(out.Buttons, label)
				if has(n, "data-desk-save") || strings.Contains(strings.ToLower(label), "save") {
					out.HasSaveButton = true
				}
				if has(n, "data-desk-submit") || strings.EqualFold(attr(n, "type"), "submit") || strings.Contains(strings.ToLower(label), "submit") {
					out.HasSubmitButton = true
				}
			}
			if tag(n) == "div" && has(n, "data-field") && !insideField(n, form) {
				if f, ok := field(n, form); ok {
					out.Fields = append(out.Fields, f)
				}
			}
		})
	})
	return out, nil
}

func tag(n *html.Node) string {
	if n != nil && n.Type == html.ElementNode {
		return n.Data
	}
	return ""
}
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func has(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
func class(n *html.Node, name string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == name {
			return true
		}
	}
	return false
}
func hidden(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if has(n, "hidden") || strings.EqualFold(attr(n, "aria-hidden"), "true") || tag(n) == "input" && strings.EqualFold(attr(n, "type"), "hidden") {
		return true
	}
	for _, d := range strings.Split(strings.ToLower(attr(n, "style")), ";") {
		parts := strings.SplitN(d, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == "display" && strings.HasPrefix(strings.TrimSpace(parts[1]), "none") {
			return true
		}
	}
	return false
}
func visit(n *html.Node, fn func(*html.Node)) {
	fn(n)
	if hidden(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		visit(c, fn)
	}
}
func walk(n *html.Node, ancestorHidden bool, fn func(*html.Node)) {
	if tag(n) == "form" && !ancestorHidden {
		fn(n)
	}
	ancestorHidden = ancestorHidden || hidden(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, ancestorHidden, fn)
	}
}
func insideField(n, form *html.Node) bool {
	for p := n.Parent; p != nil && p != form; p = p.Parent {
		if has(p, "data-field") {
			return true
		}
	}
	return false
}
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
func visibleText(n *html.Node, option bool) string {
	if hidden(n) || option && class(n, "desk-recommended") {
		return ""
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(visibleText(c, option))
	}
	return b.String()
}
func allText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(allText(c))
	}
	return b.String()
}
func field(n, form *html.Node) (Field, bool) {
	f := Field{ID: attr(n, "data-field"), Options: []string{}, Required: strings.EqualFold(attr(n, "aria-required"), "true")}
	dk := attr(n, "data-kind")
	var kinds []string
	var label *html.Node
	visit(n, func(c *html.Node) {
		if c == n || hidden(c) {
			return
		}
		switch tag(c) {
		case "label":
			if label == nil && !class(c, "desk-option") {
				label = c
			}
		case "textarea", "select", "input":
			k := tag(c)
			if k == "input" {
				k = strings.ToLower(attr(c, "type"))
				if k == "" {
					k = "text"
				}
			}
			kinds = append(kinds, k)
			if has(c, "required") || strings.EqualFold(attr(c, "aria-required"), "true") {
				f.Required = true
			}
			if k == "radio" || k == "checkbox" {
				v := attr(c, "value")
				if v == "" {
					for p := c.Parent; p != nil && p != n; p = p.Parent {
						if tag(p) == "label" {
							v = clean(visibleText(p, true))
							break
						}
					}
				}
				if v != "" {
					f.Options = append(f.Options, v)
				}
			}
		case "option":
			if v := strings.TrimSpace(attr(c, "value")); v != "" {
				f.Options = append(f.Options, v)
			}
		}
	})
	if label == nil && f.ID != "" {
		visit(form, func(c *html.Node) {
			if label == nil && tag(c) == "label" && attr(c, "for") == f.ID {
				label = c
			}
		})
	}
	if label != nil {
		raw := visibleText(label, false)
		f.Label = strings.TrimSpace(strings.TrimRightFunc(clean(raw), func(r rune) bool { return r == '*' || unicode.IsSpace(r) }))
		if strings.Contains(allText(label), "*") {
			f.Required = true
		}
	}
	if len(kinds) == 0 {
		return Field{}, false
	}
	contains := func(k string) bool {
		for _, x := range kinds {
			if x == k {
				return true
			}
		}
		return false
	}
	switch {
	case contains("textarea"):
		f.Kind = "textarea"
	case contains("file"):
		f.Kind = "file"
	case contains("checkbox"):
		f.Kind = "multi"
	case contains("radio") || contains("select"):
		f.Kind = "choice"
		if dk == "yes-no" || dk != "choice" && len(f.Options) == 2 && f.Options[0] == "yes" && f.Options[1] == "no" {
			f.Kind = "yes-no"
		}
	default:
		f.Kind = dk
		if f.Kind == "" {
			f.Kind = "text"
		}
	}
	return f, true
}
