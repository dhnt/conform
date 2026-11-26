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
	Fields  []Field  `json:"fields"`
	Buttons []string `json:"buttons"`
}

func tag(n *html.Node) string {
	if n != nil && n.Type == html.ElementNode {
		return n.Data
	}
	return ""
}
func attr(n *html.Node, key string) string {
	if n != nil {
		for _, a := range n.Attr {
			if a.Key == key {
				return a.Val
			}
		}
	}
	return ""
}
func has(n *html.Node, key string) bool {
	if n != nil {
		for _, a := range n.Attr {
			if a.Key == key {
				return true
			}
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
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	if has(n, "hidden") || strings.EqualFold(attr(n, "aria-hidden"), "true") || tag(n) == "input" && strings.EqualFold(attr(n, "type"), "hidden") {
		return true
	}
	for _, d := range strings.Split(strings.ToLower(attr(n, "style")), ";") {
		p := strings.SplitN(d, ":", 2)
		if len(p) == 2 && ((strings.TrimSpace(p[0]) == "display" && strings.HasPrefix(strings.TrimSpace(p[1]), "none")) || (strings.TrimSpace(p[0]) == "visibility" && strings.HasPrefix(strings.TrimSpace(p[1]), "hidden"))) {
			return true
		}
	}
	return false
}
func visit(n *html.Node, fn func(*html.Node)) {
	if hidden(n) {
		return
	}
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		visit(c, fn)
	}
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
func trimLabel(s string) string {
	return strings.TrimSpace(strings.TrimRightFunc(clean(s), func(r rune) bool { return r == '*' || unicode.IsSpace(r) }))
}
func labelText(n *html.Node) string {
	return trimLabel(visibleText(n, false))
}
func required(n *html.Node) bool {
	return has(n, "required") || strings.EqualFold(attr(n, "aria-required"), "true")
}
func inputType(n *html.Node) string {
	if tag(n) != "input" {
		return tag(n)
	}
	if s := strings.ToLower(attr(n, "type")); s != "" {
		return s
	}
	return "text"
}
func control(n *html.Node) bool {
	switch tag(n) {
	case "textarea", "select":
		return true
	case "input":
		switch inputType(n) {
		case "hidden", "submit", "button", "reset", "image":
			return false
		}
		return true
	}
	return false
}
func ancestor(n, form *html.Node, match func(*html.Node) bool) *html.Node {
	for p := n.Parent; p != nil && p != form; p = p.Parent {
		if match(p) {
			return p
		}
	}
	return nil
}
func fieldWrap(n, form *html.Node) *html.Node {
	return ancestor(n, form, func(p *html.Node) bool { return has(p, "data-field") })
}
func groupWrap(n, form *html.Node) *html.Node {
	if p := fieldWrap(n, form); p != nil {
		return p
	}
	return ancestor(n, form, func(p *html.Node) bool {
		return tag(p) == "fieldset" || (attr(p, "role") == "radiogroup" || attr(p, "role") == "group")
	})
}
func first(n *html.Node, match func(*html.Node) bool) *html.Node {
	var found *html.Node
	visit(n, func(c *html.Node) {
		if found == nil && match(c) {
			found = c
		}
	})
	return found
}
func byID(form *html.Node, id string) *html.Node {
	if id == "" {
		return nil
	}
	return first(form, func(n *html.Node) bool { return attr(n, "id") == id })
}
func labelledBy(n, form *html.Node) string {
	var parts []string
	for _, id := range strings.Fields(attr(n, "aria-labelledby")) {
		if ref := byID(form, id); ref != nil {
			parts = append(parts, labelText(ref))
		}
	}
	return trimLabel(strings.Join(parts, " "))
}
func findLabel(form *html.Node, id string) *html.Node {
	if id == "" {
		return nil
	}
	return first(form, func(n *html.Node) bool { return tag(n) == "label" && attr(n, "for") == id })
}
func legend(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	return first(n, func(c *html.Node) bool { return tag(c) == "legend" })
}
func fieldLabel(n, group, form *html.Node, grouped bool) *html.Node {
	if grouped {
		if group != nil {
			if l := findLabel(form, attr(group, "id")); l != nil {
				return l
			}
		}
		return nil
	}
	if l := findLabel(form, attr(n, "id")); l != nil {
		return l
	}
	return ancestor(n, form, func(p *html.Node) bool { return tag(p) == "label" })
}
func groupLabel(group *html.Node) *html.Node {
	if group == nil {
		return nil
	}
	if l := legend(group); l != nil {
		return l
	}
	if has(group, "data-field") {
		return first(group, func(c *html.Node) bool {
			return tag(c) == "label" && !class(c, "desk-option") && first(c, control) == nil
		})
	}
	return nil
}
func optionLabel(n, form *html.Node) string {
	if l := findLabel(form, attr(n, "id")); l != nil {
		if s := labelText(l); s != "" {
			return s
		}
	}
	if l := ancestor(n, form, func(p *html.Node) bool { return tag(p) == "label" }); l != nil {
		if s := clean(visibleText(l, true)); s != "" {
			return s
		}
	}
	if s := labelledBy(n, form); s != "" {
		return s
	}
	if s := clean(attr(n, "aria-label")); s != "" {
		return s
	}
	return clean(attr(n, "value"))
}
func fieldID(n, group *html.Node) string {
	if group != nil && has(group, "data-field") {
		return attr(group, "data-field")
	}
	if has(n, "data-field") {
		return attr(n, "data-field")
	}
	if s := attr(n, "name"); s != "" {
		return s
	}
	return attr(n, "id")
}

// Inventory extracts visible controls and buttons in form order.
func Inventory(r io.Reader) (InventoryResult, error) {
	root, err := html.Parse(r)
	if err != nil {
		return InventoryResult{}, err
	}
	out := InventoryResult{Fields: []Field{}, Buttons: []string{}}
	visit(root, func(form *html.Node) {
		if tag(form) != "form" {
			return
		}
		seen := map[struct {
			group *html.Node
			kind  string
		}]bool{}
		named := map[string]bool{}
		visit(form, func(n *html.Node) {
			if tag(n) == "button" || tag(n) == "input" && (inputType(n) == "submit" || inputType(n) == "button" || inputType(n) == "reset") {
				label := clean(visibleText(n, false))
				if tag(n) == "input" {
					label = clean(attr(n, "value"))
					if label == "" {
						switch inputType(n) {
						case "submit":
							label = "Submit"
						case "reset":
							label = "Reset"
						}
					}
				}
				if label == "" {
					label = clean(attr(n, "aria-label"))
				}
				if label != "" {
					out.Buttons = append(out.Buttons, label)
				}
			}
			if !control(n) {
				return
			}
			kind := inputType(n)
			grouped := kind == "radio" || kind == "checkbox"
			var group *html.Node
			if grouped {
				group = groupWrap(n, form)
			} else {
				group = fieldWrap(n, form)
				if group == nil {
					group = ancestor(n, form, func(p *html.Node) bool { return tag(p) == "fieldset" })
				}
			}
			if grouped {
				if group != nil {
					key := struct {
						group *html.Node
						kind  string
					}{group, kind}
					if seen[key] {
						return
					}
					seen[key] = true
				} else if name := attr(n, "name"); name != "" {
					key := kind + ":" + name
					if named[key] {
						return
					}
					named[key] = true
				}
			}
			members := []*html.Node{n}
			if grouped {
				members = nil
				visit(form, func(c *html.Node) {
					if !control(c) || inputType(c) != kind {
						return
					}
					if group != nil {
						if groupWrap(c, form) == group {
							members = append(members, c)
						}
					} else if c == n || groupWrap(c, form) == nil && attr(c, "name") == attr(n, "name") && attr(n, "name") != "" {
						members = append(members, c)
					}
				})
			}
			f := Field{ID: fieldID(n, group), Options: []string{}}
			if group != nil {
				f.Required = required(group)
			}
			for _, c := range members {
				f.Required = f.Required || required(c)
			}
			label := fieldLabel(n, group, form, grouped)
			if label != nil {
				f.Label = labelText(label)
				if strings.Contains(allText(label), "*") {
					f.Required = true
				}
			}
			if f.Label == "" {
				for _, source := range []*html.Node{group, n} {
					if source == nil {
						continue
					}
					if s := labelledBy(source, form); s != "" {
						f.Label = s
						break
					}
					if s := trimLabel(attr(source, "aria-label")); s != "" {
						f.Label = s
						break
					}
				}
			}
			if f.Label == "" {
				if l := groupLabel(group); l != nil {
					f.Label = labelText(l)
					if strings.Contains(allText(l), "*") {
						f.Required = true
					}
				}
			}
			if f.Label == "" && grouped && len(members) == 1 {
				f.Label = optionLabel(n, form)
			}
			if kind == "select" {
				visit(n, func(c *html.Node) {
					if tag(c) == "option" && !(has(c, "value") && strings.TrimSpace(attr(c, "value")) == "") {
						s := clean(visibleText(c, false))
						if s == "" {
							s = clean(attr(c, "value"))
						}
						if s != "" {
							f.Options = append(f.Options, s)
						}
					}
				})
			}
			if grouped {
				for _, c := range members {
					if s := optionLabel(c, form); s != "" {
						f.Options = append(f.Options, s)
					}
				}
			}
			switch kind {
			case "textarea":
				f.Kind = "textarea"
			case "select":
				if has(n, "multiple") {
					f.Kind = "multi"
				} else {
					f.Kind = "choice"
				}
			case "radio":
				f.Kind = "choice"
				if len(f.Options) == 2 && ((strings.EqualFold(f.Options[0], "yes") && strings.EqualFold(f.Options[1], "no")) || (strings.EqualFold(f.Options[0], "no") && strings.EqualFold(f.Options[1], "yes"))) {
					f.Kind = "yes-no"
				}
			case "checkbox":
				if len(members) == 1 {
					// A lone checkbox is a yes-no field: checked is yes. Its label
					// names the field, not an option.
					f.Kind = "yes-no"
					f.Options = []string{}
				} else {
					f.Kind = "multi"
				}
			case "file":
				f.Kind = "file"
			default:
				f.Kind = "text"
			}
			if group != nil && attr(group, "data-kind") != "" {
				f.Kind = attr(group, "data-kind")
			} else if attr(n, "data-kind") != "" {
				f.Kind = attr(n, "data-kind")
			}
			out.Fields = append(out.Fields, f)
		})
	})
	return out, nil
}
