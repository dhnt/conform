package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dhnt/conform"
)

func TestRun(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != conform.Version {
		t.Fatalf("version: code=%d out=%q", code, out.String())
	}
	if code := run(nil, &out, &errb); code != 2 {
		t.Fatalf("no args: code=%d", code)
	}
	if code := run([]string{"nope"}, &out, &errb); code != 2 {
		t.Fatalf("unknown: code=%d", code)
	}
}
