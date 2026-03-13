package main

import "testing"

func TestBuildCaption(t *testing.T) {
	draft := Draft{
		Caption:     "my meme",
		SourceLabel: "https://example.com",
	}
	got := buildCaption(draft)
	want := "my meme\n\nИсточник: https://example.com"
	if got != want {
		t.Fatalf("buildCaption() = %q, want %q", got, want)
	}
}

func TestDetectMediaKind(t *testing.T) {
	cases := []struct {
		name string
		mime string
		want MediaKind
	}{
		{"cat.jpg", "", MediaPhoto},
		{"clip.mp4", "", MediaVideo},
		{"", "video/mp4", MediaVideo},
	}
	for _, tc := range cases {
		if got := DetectMediaKind(tc.name, tc.mime); got != tc.want {
			t.Fatalf("DetectMediaKind(%q,%q)=%q want %q", tc.name, tc.mime, got, tc.want)
		}
	}
}
