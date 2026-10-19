package search

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseQuery(t *testing.T) {
	title := []string{"title"}
	cases := []struct {
		name string
		q    string
		want []Alternative
	}{
		{"empty stays one empty alternative", "", []Alternative{{Text: ""}}},
		{"no operator returns q untouched, spacing included", "  golang   rust ", []Alternative{{Text: "  golang   rust "}}},
		{"lowercase or is a word", "golang or rust", []Alternative{{Text: "golang or rust"}}},
		{"OR splits", "produktchef OR \"product owner\"", []Alternative{{Text: "produktchef"}, {Text: "\"product owner\""}}},
		{"OR inside quotes is text", "\"a OR b\"", []Alternative{{Text: "\"a OR b\""}}},
		{"minus survives per alternative", "golang -senior OR rust", []Alternative{{Text: "golang -senior"}, {Text: "rust"}}},
		{"title: scopes its alternative", "title:\"product owner\" -junior", []Alternative{{Text: "\"product owner\" -junior", Fields: title}}},
		{"title: per alternative", "title:produktchef OR golang", []Alternative{{Text: "produktchef", Fields: title}, {Text: "golang"}}},
		{"detached title: prefix", "title: produktchef", []Alternative{{Text: "produktchef", Fields: title}}},
		{"title: mid-alternative is text", "golang title:lead", []Alternative{{Text: "golang title:lead"}}},
		{"empty alternatives dropped", "OR golang OR OR rust OR", []Alternative{{Text: "golang"}, {Text: "rust"}}},
		{"only operators falls back to q", "OR", []Alternative{{Text: "OR"}}},
		{"bare title: falls back to q", "title:", []Alternative{{Text: "title:"}}},
		{"five alternatives allowed", "a OR b OR c OR d OR e", []Alternative{{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}, {Text: "e"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseQuery(tc.q)
			if err != nil {
				t.Fatalf("ParseQuery(%q) error: %v", tc.q, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseQuery(%q) = %#v, want %#v", tc.q, got, tc.want)
			}
		})
	}
}

func TestParseQueryRefusesMoreThanFiveAlternatives(t *testing.T) {
	_, err := ParseQuery("a OR b OR c OR d OR e OR f")
	if !errors.Is(err, ErrTooManyAlternatives) {
		t.Fatalf("err = %v, want ErrTooManyAlternatives", err)
	}
}
