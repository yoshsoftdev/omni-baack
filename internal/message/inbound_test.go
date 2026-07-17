package message

import (
	"encoding/json"
	"testing"
)

func TestShouldApplyStatus_Monotonic(t *testing.T) {
	sent := StatusSent
	delivered := StatusDelivered
	read := StatusRead

	cases := []struct {
		current *Status
		next    Status
		want    bool
	}{
		{nil, StatusSent, true},
		{&sent, StatusDelivered, true},
		{&delivered, StatusRead, true},
		{&read, StatusDelivered, false},
		{&delivered, StatusSent, false},
		{&sent, StatusSent, false},
	}
	for _, tc := range cases {
		if got := shouldApplyStatus(tc.current, tc.next); got != tc.want {
			t.Fatalf("current=%v next=%v got=%v want=%v", tc.current, tc.next, got, tc.want)
		}
	}
}

func TestContentTypeFromMeta_InstagramVariants(t *testing.T) {
	cases := map[string]ContentType{
		"story_mention": ContentOther,
		"story_reply":   ContentOther,
		"ig_reel":       ContentOther,
		"reel":          ContentOther,
		"ig_post":       ContentOther,
		"post":          ContentOther,
		"share":         ContentOther,
		"fallback":      ContentOther,
		"ig_story":      ContentOther,
		"file":          ContentDocument,
	}
	for input, want := range cases {
		if got := ContentTypeFromMeta(input); got != want {
			t.Fatalf("input=%s got=%s want=%s", input, got, want)
		}
	}
}

func TestExtractTextContent_StorySignals(t *testing.T) {
	storyMentionRaw, _ := json.Marshal(map[string]any{
		"attachments": []map[string]any{
			{
				"type": "story_mention",
				"payload": map[string]any{
					"url": "https://cdn/story.jpg",
				},
			},
		},
	})
	if got := ExtractTextContent("story_mention", storyMentionRaw); got != "Mentioned your story" {
		t.Fatalf("story mention got=%q", got)
	}

	storyReplyRaw, _ := json.Marshal(map[string]any{
		"reply_to": map[string]any{
			"story": map[string]any{
				"id":  "story-1",
				"url": "https://cdn/story.jpg",
			},
		},
	})
	if got := ExtractTextContent("story_reply", storyReplyRaw); got != "Replied to a story" {
		t.Fatalf("story reply got=%q", got)
	}

	igPostRaw, _ := json.Marshal(map[string]any{
		"attachments": []map[string]any{
			{
				"type": "ig_post",
				"payload": map[string]any{
					"title": "AI that works with humans",
					"url":   "https://www.instagram.com/p/ABC123/",
				},
			},
		},
	})
	wantPost := "Shared a post\nAI that works with humans\nhttps://www.instagram.com/p/ABC123/"
	if got := ExtractTextContent("ig_post", igPostRaw); got != wantPost {
		t.Fatalf("ig_post got=%q want=%q", got, wantPost)
	}

	igReelRaw, _ := json.Marshal(map[string]any{
		"attachments": []map[string]any{
			{
				"type": "ig_reel",
				"payload": map[string]any{
					"title": "Sunset reel",
					"url":   "https://www.instagram.com/reel/XYZ789/",
				},
			},
		},
	})
	wantReel := "Shared a reel\nSunset reel\nhttps://www.instagram.com/reel/XYZ789/"
	if got := ExtractTextContent("ig_reel", igReelRaw); got != wantReel {
		t.Fatalf("ig_reel got=%q want=%q", got, wantReel)
	}

	igStoryRaw, _ := json.Marshal(map[string]any{
		"attachments": []map[string]any{
			{
				"type": "ig_story",
				"payload": map[string]any{
					"story_media_id":  "123",
					"story_media_url": "https://cdn/story-media.jpg",
				},
			},
		},
	})
	if got := ExtractTextContent("ig_story", igStoryRaw); got != "Shared a story" {
		t.Fatalf("ig_story got=%q", got)
	}
}

func TestIsInstagramOGShareType(t *testing.T) {
	for _, input := range []string{"ig_post", "post", "ig_reel", "reel", "share", "fallback"} {
		if !IsInstagramOGShareType(input) {
			t.Fatalf("expected %s to be OG share", input)
		}
	}
	if IsInstagramOGShareType("image") {
		t.Fatal("image should not be OG share")
	}
}

func TestExtractInstagramShare_PrefersIgPostPermalinkOverShareCDN(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"attachments": []map[string]any{
			{
				"type": "share",
				"payload": map[string]any{
					"ig_post_media_id": "18139494541428835",
					"title":            "Workspace post",
					"url":              "https://lookaside.fbsbx.com/ig_messaging_cdn/?asset_id=18139494541428835&signature=abc",
				},
			},
			{
				"type": "ig_post",
				"payload": map[string]any{
					"ig_post_media_id": "18139494541428835",
					"title":            "Workspace post",
					"url":              "https://www.instagram.com/p/ABC123/",
				},
			},
		},
	})
	share := ExtractInstagramShare(raw)
	if share == nil {
		t.Fatal("expected share")
	}
	if share.Type != "ig_post" {
		t.Fatalf("type=%q", share.Type)
	}
	if share.URL != "https://www.instagram.com/p/ABC123/" {
		t.Fatalf("url=%q", share.URL)
	}
	if share.MediaID != "18139494541428835" {
		t.Fatalf("media_id=%q", share.MediaID)
	}
	got := ExtractTextContent("ig_post", raw)
	want := "Shared a post\nWorkspace post\nhttps://www.instagram.com/p/ABC123/"
	if got != want {
		t.Fatalf("content=%q want=%q", got, want)
	}
}

func TestExtractInstagramShare_PrefersIgPostOverShareWhenBothCDN(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"attachments": []map[string]any{
			{
				"type": "share",
				"payload": map[string]any{
					"url":   "https://lookaside.fbsbx.com/ig_messaging_cdn/?asset_id=1&signature=a",
					"title": "Post",
				},
			},
			{
				"type": "ig_post",
				"payload": map[string]any{
					"ig_post_media_id": "18139494541428835",
					"url":              "https://lookaside.fbsbx.com/ig_messaging_cdn/?asset_id=18139494541428835&signature=b",
					"title":            "Post",
				},
			},
		},
	})
	share := ExtractInstagramShare(raw)
	if share == nil {
		t.Fatal("expected share")
	}
	if share.Type != "ig_post" {
		t.Fatalf("type=%q", share.Type)
	}
	if share.MediaID != "18139494541428835" {
		t.Fatalf("media_id=%q", share.MediaID)
	}
}

func TestIsInstagramPublicURL(t *testing.T) {
	if !IsInstagramPublicURL("https://www.instagram.com/p/ABC/") {
		t.Fatal("expected post permalink")
	}
	if !IsInstagramPublicURL("https://instagram.com/reel/XYZ/") {
		t.Fatal("expected reel permalink")
	}
	if IsInstagramPublicURL("https://lookaside.fbsbx.com/ig_messaging_cdn/?asset_id=1") {
		t.Fatal("cdn should not count as public")
	}
}
