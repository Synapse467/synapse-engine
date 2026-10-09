package textutil

import (
	"reflect"
	"testing"
)

func TestTermsLowercaseStemAndDropStopwords(t *testing.T) {
	got := Terms("How do I handle the Retries, and retrying failed requests?")
	want := []string{"handl", "retry", "retry", "fail", "request"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestTermsKeepNumbersAndUnicode(t *testing.T) {
	got := Terms("Café 2026 über")
	if !reflect.DeepEqual(got, []string{"café", "2026", "über"}) {
		t.Fatalf("got %v", got)
	}
}

func TestTermsAreDeterministic(t *testing.T) {
	a := Terms("Rotate keys every 90 days; never share keys.")
	b := Terms("Rotate keys every 90 days; never share keys.")
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same text produced different terms")
	}
}

func TestStemIsConservative(t *testing.T) {
	for word, want := range map[string]string{
		"class": "class", "status": "status", "analysis": "analysis", "boxes": "box", "policies": "policy",
		"running": "run", "stopped": "stop", "cats": "cat", "use": "use", "rotate": "rotat", "rotated": "rotat", "rotating": "rotat", "rotation": "rotat", "protection": "protect", "action": "action",
	} {
		if got := Stem(word); got != want {
			t.Errorf("Stem(%q) = %q, want %q", word, got, want)
		}
	}
}

func TestUniqueKeepsFirstOccurrenceOrder(t *testing.T) {
	if got := Unique("beta alpha beta gamma alpha"); !reflect.DeepEqual(got, []string{"beta", "alpha", "gamma"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSentencesCarryByteOffsets(t *testing.T) {
	text := "First one. Second, e.g. this one! Third?"
	spans := Sentences(text)
	if len(spans) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %+v", len(spans), spans)
	}
	for _, s := range spans {
		if text[s.Start:s.End] != s.Text {
			t.Fatalf("offsets do not match the text: %+v", s)
		}
	}
	if spans[1].Text != "Second, e.g. this one!" {
		t.Fatalf("abbreviation split the sentence: %q", spans[1].Text)
	}
}

func TestSentencesOfEmptyText(t *testing.T) {
	if len(Sentences("   \n ")) != 0 {
		t.Fatal("whitespace produced a sentence")
	}
}
