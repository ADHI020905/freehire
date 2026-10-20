package mcpapp

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The page keeps wire data in three variables only — `result`, `job` and `req` — so every
// field it reads is visible to this pattern. A field it reads that wire.go does not publish
// renders as nothing, silently, in a frame nobody here is watching.
var widgetFieldRead = regexp.MustCompile(`\b(result|job|req)\.([A-Za-z0-9_]+)`)

// jsonFields is every JSON field name of the given types, embedded structs included.
func jsonFields(types ...reflect.Type) map[string]bool {
	fields := map[string]bool{}
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := range t.NumField() {
			f := t.Field(i)
			if f.Anonymous {
				walk(f.Type)
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name != "" && name != "-" {
				fields[name] = true
			}
		}
	}
	for _, t := range types {
		walk(t)
	}
	return fields
}

func widgetFieldsRead(page string) []string {
	var read []string
	for _, m := range widgetFieldRead.FindAllStringSubmatch(page, -1) {
		if !slices.Contains(read, m[2]) {
			read = append(read, m[2])
		}
	}
	slices.Sort(read)
	return read
}

func TestTheWidgetReadsOnlyFieldsTheWireShapeHas(t *testing.T) {
	published := jsonFields(
		reflect.TypeFor[SearchJobsResult](),
		reflect.TypeFor[JobResult](),
		reflect.TypeFor[Requirement](),
	)
	read := widgetFieldsRead(string(widgetPage))
	if len(read) < 10 {
		t.Fatalf("found only %d field reads (%v) — the page no longer keeps wire data in result/job/req, and this guard sees nothing", len(read), read)
	}
	for _, field := range read {
		if !published[field] {
			t.Errorf("the widget reads %q, which no wire.go type publishes", field)
		}
	}
}

// The guard must be able to fail: a page reading a field the wire shape lacks is caught.
func TestTheFieldGuardCatchesAnUnpublishedField(t *testing.T) {
	published := jsonFields(reflect.TypeFor[JobResult]())
	read := widgetFieldsRead(`const x = job.salary_minimum; const y = job.title;`)
	if !slices.Equal(read, []string{"salary_minimum", "title"}) {
		t.Fatalf("read = %v", read)
	}
	if published["salary_minimum"] || !published["title"] {
		t.Fatal("jsonFields does not reflect JobResult")
	}
}

// Results come from postings we crawled, so the page must never turn one into markup, and it
// must never talk to anyone: the CSP admits no connection, and a request the host blocks is
// a feature that silently does nothing.
func TestTheWidgetCannotInjectMarkupOrMakeRequests(t *testing.T) {
	page := string(widgetPage)
	for _, banned := range []string{
		"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write",
		"fetch(", "XMLHttpRequest", "WebSocket", "EventSource", "sendBeacon",
		"eval(", "new Function",
	} {
		if strings.Contains(page, banned) {
			t.Errorf("the widget uses %s", banned)
		}
	}
}
