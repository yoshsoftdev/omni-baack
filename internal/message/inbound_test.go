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
		"ig_post":       ContentOther,
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
					"url":   "https://cdn/post.jpg",
				},
			},
		},
	})
	if got := ExtractTextContent("ig_post", igPostRaw); got != "Shared a post\nAI that works with humans" {
		t.Fatalf("ig_post got=%q", got)
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
