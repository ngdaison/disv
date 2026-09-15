package feed

import (
	"testing"
)

func TestResolveYouTubeChannel(t *testing.T) {
	// Test with a standard YouTube channel ID
	chID, title, _, _, err := ResolveYouTubeChannel("UC_x5XG1OV2P6uZZ5FSM9Ttw")
	if err != nil {
		t.Fatalf("Lỗi phân giải YouTube Channel ID: %v", err)
	}
	if chID != "UC_x5XG1OV2P6uZZ5FSM9Ttw" {
		t.Errorf("Kỳ vọng channel ID UC_x5XG1OV2P6uZZ5FSM9Ttw, nhận được: %s", chID)
	}
	if title == "" {
		t.Errorf("Kỳ vọng có tiêu đề kênh, nhận được chuỗi rỗng")
	}

	// Test with a handle URL
	chID2, title2, _, _, err2 := ResolveYouTubeChannel("https://www.youtube.com/@Google")
	if err2 != nil {
		t.Fatalf("Lỗi phân giải YouTube handle: %v", err2)
	}
	if chID2 == "" || title2 == "" {
		t.Errorf("Kỳ vọng lấy được channel ID và title từ handle, nhận được: ID=%s, Title=%s", chID2, title2)
	}
}

func TestResolveTikTokChannel(t *testing.T) {
	username, displayName, _, _, err := ResolveTikTokChannel("https://www.tiktok.com/@shrimcangu1")
	if err != nil {
		t.Fatalf("Lỗi phân giải TikTok channel: %v", err)
	}
	if username != "shrimcangu1" {
		t.Errorf("Kỳ vọng username shrimcangu1, nhận được: %s", username)
	}
	if displayName == "" {
		t.Errorf("Kỳ vọng có tên hiển thị, nhận được chuỗi rỗng")
	}
}

func TestResolveFakeChannels(t *testing.T) {
	// Fake YouTube channel ID
	_, _, _, _, ytErr := ResolveYouTubeChannel("UC0000000000000000000000")
	if ytErr == nil {
		t.Errorf("Kỳ vọng kênh YouTube giả mạo phải báo lỗi, nhưng lại thành công")
	}

	// Fake TikTok username
	_, _, _, _, ttErr := ResolveTikTokChannel("abcdefghijklmn998877665544332211")
	if ttErr == nil {
		t.Errorf("Kỳ vọng kênh TikTok giả mạo phải báo lỗi, nhưng lại thành công")
	}
}
