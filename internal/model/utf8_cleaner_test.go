package model

import (
	"testing"
	"unicode/utf8"
)

func TestCleanUTF8(t *testing.T) {
	// Normal UTF-8 text
	normal := "hello world - normal text"
	if CleanUTF8(normal) != normal {
		t.Fatalf("expected untouched normal string, got %s", CleanUTF8(normal))
	}

	// Corrupted UTF-8 / invalid byte sequences
	corrupted := string([]byte{'h', 'e', 'l', 'l', 'o', 0xff, 0xfe, 'w', 'o', 'r', 'l', 'd'})
	cleaned := CleanUTF8(corrupted)
	if !utf8.ValidString(cleaned) {
		t.Fatalf("expected valid UTF-8 string, got invalid")
	}

	// Windows GBK mojibake string test case (Review Focus #3)
	mojibake := "agentops 涓婃姤"
	cleanedMoji := CleanUTF8(mojibake)
	if !utf8.ValidString(cleanedMoji) {
		t.Fatalf("expected valid UTF-8, got %s", cleanedMoji)
	}
}
