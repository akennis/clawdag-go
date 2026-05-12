package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func runParse(t *testing.T, raw string) *ParseCitationsOp {
	t.Helper()
	op := &ParseCitationsOp{Raw: &raw}
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return op
}

func TestParseCitations_StandardTrailer(t *testing.T) {
	op := runParse(t, "To return an item, sign in to your account.\n\nSources: returns.txt")
	if op.Body != "To return an item, sign in to your account." {
		t.Fatalf("Body = %q", op.Body)
	}
	if !reflect.DeepEqual(op.Sources, []string{"returns.txt"}) {
		t.Fatalf("Sources = %v, want [returns.txt]", op.Sources)
	}
}

func TestParseCitations_MultipleSources(t *testing.T) {
	op := runParse(t, "Shipping takes 3-5 days. Returns are accepted within 30 days.\nSources: shipping.txt, returns.txt")
	if !reflect.DeepEqual(op.Sources, []string{"shipping.txt", "returns.txt"}) {
		t.Fatalf("Sources = %v, want [shipping.txt returns.txt]", op.Sources)
	}
}

func TestParseCitations_None(t *testing.T) {
	op := runParse(t, "I don't know based on the provided context.\n\nSources: none")
	if op.Body != "I don't know based on the provided context." {
		t.Fatalf("Body = %q", op.Body)
	}
	if op.Sources != nil {
		t.Fatalf("Sources = %v, want nil", op.Sources)
	}
}

func TestParseCitations_NoTrailer(t *testing.T) {
	op := runParse(t, "This response forgot to cite anything.")
	if op.Body != "This response forgot to cite anything." {
		t.Fatalf("Body = %q", op.Body)
	}
	if op.Sources != nil {
		t.Fatalf("Sources = %v, want nil when no trailer present", op.Sources)
	}
}

func TestParseCitations_CaseInsensitiveMarker(t *testing.T) {
	op := runParse(t, "Answer body.\nSOURCES: a.txt, b.txt")
	if !reflect.DeepEqual(op.Sources, []string{"a.txt", "b.txt"}) {
		t.Fatalf("Sources = %v", op.Sources)
	}
}

func TestParseCitations_WhitespaceAroundCommas(t *testing.T) {
	op := runParse(t, "Answer.\nSources:   returns.txt ,   shipping.txt  ,warranty.txt")
	if !reflect.DeepEqual(op.Sources, []string{"returns.txt", "shipping.txt", "warranty.txt"}) {
		t.Fatalf("Sources = %v", op.Sources)
	}
}

func TestParseCitations_LastTrailerWins(t *testing.T) {
	// If the body itself contains the word "Sources:" earlier, the LAST occurrence
	// is the canonical citation line.
	op := runParse(t, "I'll list the Sources: section below.\n\nThe answer is X.\n\nSources: kb1.txt")
	if !reflect.DeepEqual(op.Sources, []string{"kb1.txt"}) {
		t.Fatalf("Sources = %v, want [kb1.txt] (last trailer wins)", op.Sources)
	}
	if op.Body == "" || !strings.Contains(op.Body, "The answer is X.") {
		t.Fatalf("Body = %q, expected to contain answer text", op.Body)
	}
}

func TestParseCitations_EmptyTrailer(t *testing.T) {
	op := runParse(t, "Some answer.\nSources:")
	if op.Body != "Some answer." {
		t.Fatalf("Body = %q", op.Body)
	}
	if op.Sources != nil {
		t.Fatalf("Sources = %v, want nil for empty trailer", op.Sources)
	}
}
