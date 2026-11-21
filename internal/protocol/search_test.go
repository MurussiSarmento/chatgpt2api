package protocol

import "testing"

func TestCleanSearchText(t *testing.T) {
	input := "Answer urlSourcehttps://example.com ."
	got := CleanSearchText(input)
	if got != "Answer Source (https://example.com)." {
		t.Fatalf("clean text = %q", got)
	}
	annotations := SearchAnnotations(got)
	if len(annotations) != 1 || annotations[0]["url"] != "https://example.com" || annotations[0]["start_index"] != 15 {
		t.Fatalf("annotations = %#v", annotations)
	}
}
