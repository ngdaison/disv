package feed

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

var (
	httpClient = &http.Client{Timeout: 30 * time.Second}

	ytChannelRegex = regexp.MustCompile(`(?:channel\/|channel_id=|\"channelId\":\"|itemprop=\"channelId\" content=\")(UC[\w-]+)`)
	ytHandleRegex  = regexp.MustCompile(`@([\w.-]+)`)
	ttUserRegex    = regexp.MustCompile(`(?:tiktok\.com\/@?|@)([\w.-]+)`)
)

type XMLFeed struct {
	XMLName xml.Name   `xml:"feed"`
	Title   string     `xml:"title"`
	Author  XMLAuthor  `xml:"author"`
	Entries []XMLEntry `xml:"entry"`
}

type XMLAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri"`
}

type XMLEntry struct {
	ID        string  `xml:"id"`
	VideoID   string  `xml:"videoId"`
	Title     string  `xml:"title"`
	Link      XMLLink `xml:"link"`
	Published string  `xml:"published"`
}

type XMLLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type Service struct {
	store    *storage.MySQLStore
	stopChan chan struct{}
	mu       sync.Mutex
}

func NewService(store *storage.MySQLStore) *Service {
	return &Service{
		store:    store,
		stopChan: make(chan struct{}),
	}
}

func (s *Service) Start(sess *discordgo.Session) {
	ticker := time.NewTicker(2 * time.Minute)
	go func() {
		// Chạy ngay lần đầu sau 10 giây khởi động bot
		select {
		case <-time.After(10 * time.Second):
			s.checkFeeds(sess)
		case <-s.stopChan:
			ticker.Stop()
			return
		}

		for {
			select {
			case <-ticker.C:
				s.checkFeeds(sess)
			case <-s.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stopChan:
	default:
		close(s.stopChan)
	}
}

func (s *Service) checkFeeds(sess *discordgo.Session) {
	subs, err := s.store.GetAllVideoSubscriptions()
	if err != nil || len(subs) == 0 {
		return
	}

	// Gom nhóm kiểm tra theo platform và target_id để tránh gửi trùng lặp request tới cùng một kênh
	type targetKey struct {
		platform string
		targetID string
	}
	cacheLatest := make(map[targetKey]struct {
		videoID  string
		videoURL string
		title    string
	})

	for _, sub := range subs {
		key := targetKey{platform: sub.Platform, targetID: sub.TargetID}
		data, exists := cacheLatest[key]
		if !exists {
			var vID, vURL, vTitle string
			var fetchErr error

			if sub.Platform == "youtube" {
				vID, vURL, vTitle, fetchErr = FetchLatestYouTubeVideo(sub.TargetID)
			} else if sub.Platform == "tiktok" {
				vID, vURL, vTitle, fetchErr = FetchLatestTikTokVideo(sub.TargetID)
			}

			if fetchErr != nil || vID == "" {
				continue
			}

			data = struct {
				videoID  string
				videoURL string
				title    string
			}{videoID: vID, videoURL: vURL, title: vTitle}
			cacheLatest[key] = data
		}

		// Nếu vừa mới thêm kênh và chưa có last_video_id, lưu lại ID hiện tại để không bắn video cũ
		if sub.LastVideoID == "" {
			_ = s.store.UpdateLastVideoID(sub.ID, data.videoID)
			continue
		}

		// Nếu phát hiện video mới khác với video đã lưu
		if data.videoID != "" && data.videoID != sub.LastVideoID {
			// Cú pháp thông báo chuẩn: (@ping nếu có) (link mới mới đăng)
			var content string
			if sub.PingRoleID != "" {
				content = fmt.Sprintf("<@&%s> %s", sub.PingRoleID, data.videoURL)
			} else {
				content = data.videoURL
			}

			_, sendErr := sess.ChannelMessageSend(sub.ChannelID, content)
			if sendErr == nil {
				_ = s.store.UpdateLastVideoID(sub.ID, data.videoID)
			} else {
				log.Printf("Lỗi gửi thông báo video mới vào kênh %s: %v", sub.ChannelID, sendErr)
			}
		}
	}
}

// ResolveYouTubeChannel phân giải URL người dùng nhập sang Channel ID, tên kênh và video mới nhất
func ResolveYouTubeChannel(rawURL string) (channelID, channelTitle, latestVideoID, latestVideoURL string, err error) {
	rawURL = strings.TrimSpace(rawURL)

	// Trường hợp người dùng nhập trực tiếp Channel ID hoặc URL chứa channel/UC...
	if strings.HasPrefix(rawURL, "UC") && len(rawURL) >= 20 {
		channelID = rawURL
	} else if match := ytChannelRegex.FindStringSubmatch(rawURL); len(match) > 1 {
		channelID = match[1]
	}

	// Nếu chưa có channelID, kiểm tra nếu là @handle hoặc URL kênh
	if channelID == "" {
		fetchURL := rawURL
		if !strings.HasPrefix(fetchURL, "http") {
			if strings.HasPrefix(fetchURL, "@") {
				fetchURL = "https://www.youtube.com/" + fetchURL
			} else {
				fetchURL = "https://www.youtube.com/@" + fetchURL
			}
		}

		req, reqErr := http.NewRequest("GET", fetchURL, nil)
		if reqErr != nil {
			return "", "", "", "", reqErr
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
		req.Header.Set("Accept-Language", "vi,en;q=0.9")

		resp, respErr := httpClient.Do(req)
		if respErr != nil {
			return "", "", "", "", fmt.Errorf("không thể kết nối tới YouTube: %w", respErr)
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(resp.Body)
		html := string(bodyBytes)

		if match := ytChannelRegex.FindStringSubmatch(html); len(match) > 1 {
			channelID = match[1]
		}
	}

	if channelID == "" {
		return "", "", "", "", fmt.Errorf("không tìm thấy Channel ID của kênh YouTube này")
	}

	// Đọc RSS feed để lấy tên kênh và video mới nhất, đồng thời xác thực kênh tồn tại
	feedURL := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)
	req, err := http.NewRequest("GET", feedURL, nil)
	if err != nil {
		return "", "", "", "", fmt.Errorf("không thể khởi tạo yêu cầu kiểm tra kênh YouTube")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", "", fmt.Errorf("không thể kết nối kiểm tra kênh YouTube")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", "", fmt.Errorf("kênh YouTube này không tồn tại hoặc không hợp lệ")
	}

	var feed XMLFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return "", "", "", "", fmt.Errorf("không thể đọc dữ liệu kênh YouTube")
	}

	channelTitle = feed.Title
	if channelTitle == "" && feed.Author.Name != "" {
		channelTitle = feed.Author.Name
	}
	if channelTitle == "" {
		return "", "", "", "", fmt.Errorf("kênh YouTube này không tồn tại")
	}

	if len(feed.Entries) > 0 {
		latestEntry := feed.Entries[0]
		latestVideoID = latestEntry.VideoID
		latestVideoURL = latestEntry.Link.Href
		if latestVideoURL == "" && latestVideoID != "" {
			latestVideoURL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", latestVideoID)
		}
	}

	return channelID, channelTitle, latestVideoID, latestVideoURL, nil
}

// FetchLatestYouTubeVideo lấy video mới nhất của kênh YouTube theo channel_id
func FetchLatestYouTubeVideo(channelID string) (videoID, videoURL, title string, err error) {
	feedURL := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)
	req, err := http.NewRequest("GET", feedURL, nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("YouTube RSS status %d", resp.StatusCode)
	}

	var feed XMLFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return "", "", "", err
	}

	if len(feed.Entries) == 0 {
		return "", "", "", nil
	}

	first := feed.Entries[0]
	vID := first.VideoID
	vURL := first.Link.Href
	if vURL == "" && vID != "" {
		vURL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", vID)
	}

	return vID, vURL, first.Title, nil
}

// ResolveTikTokChannel phân giải URL TikTok người dùng nhập sang username và tên kênh
func ResolveTikTokChannel(rawURL string) (username, displayName, latestVideoID, latestVideoURL string, err error) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimPrefix(rawURL, "@")

	if match := ttUserRegex.FindStringSubmatch(rawURL); len(match) > 1 {
		username = match[1]
	} else {
		username = rawURL
	}
	username = strings.Trim(username, "/@ ")

	if username == "" {
		return "", "", "", "", fmt.Errorf("đường dẫn kênh TikTok không hợp lệ")
	}

	// 1. Kiểm tra sự tồn tại của kênh TikTok qua oEmbed chính thức
	oembedURL := fmt.Sprintf("https://www.tiktok.com/oembed?url=https://www.tiktok.com/@%s", username)
	req, reqErr := http.NewRequest("GET", oembedURL, nil)
	if reqErr != nil {
		return "", "", "", "", fmt.Errorf("không thể khởi tạo yêu cầu kiểm tra kênh TikTok")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, respErr := httpClient.Do(req)
	if respErr != nil {
		return "", "", "", "", fmt.Errorf("không thể kết nối kiểm tra kênh TikTok")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", "", fmt.Errorf("kênh TikTok @%s không tồn tại", username)
	}

	var oembedData struct {
		AuthorName string `json:"author_name"`
		Title      string `json:"title"`
	}
	if jsonErr := json.NewDecoder(resp.Body).Decode(&oembedData); jsonErr != nil || (oembedData.AuthorName == "" && oembedData.Title == "") {
		return "", "", "", "", fmt.Errorf("kênh TikTok @%s không tồn tại", username)
	}

	displayName = oembedData.AuthorName
	if displayName == "" {
		displayName = oembedData.Title
	}
	if displayName == "" {
		displayName = "@" + username
	}

	// 2. Kiểm tra video mới nhất
	vID, vURL, _, _ := FetchLatestTikTokVideo(username)
	return username, displayName, vID, vURL, nil
}

// FetchLatestTikTokVideo lấy video mới nhất của TikTok user
func FetchLatestTikTokVideo(username string) (videoID, videoURL, title string, err error) {
	profileURL := fmt.Sprintf("https://www.tiktok.com/@%s", username)
	req, err := http.NewRequest("GET", profileURL, nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "vi,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	html := string(bodyBytes)

	// Tìm link video dạng /video/1234567890
	vidRegex := regexp.MustCompile(`\/video\/(\d{15,25})`)
	if m := vidRegex.FindStringSubmatch(html); len(m) > 1 {
		vID := m[1]
		vURL := fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", username, vID)
		return vID, vURL, "TikTok Video", nil
	}

	return "", "", "", nil
}
