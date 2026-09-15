package tiktok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

var (
	tiktokRegex   = regexp.MustCompile(`https:\/\/(?:m|www|vt)?\.tiktok\.com\/\S+`)
	httpClient    = &http.Client{Timeout: 30 * time.Second}
	processedMsgs sync.Map // messageID -> time.Time
	videoLocks    sync.Map // vid -> *sync.Mutex
)

func getVideoMutex(vid string) *sync.Mutex {
	val, _ := videoLocks.LoadOrStore(vid, &sync.Mutex{})
	return val.(*sync.Mutex)
}

func getVideoFolder() string {
	for _, p := range []string{"videotiktok", "../videotiktok", filepath.Join(".", "videotiktok")} {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	_ = os.MkdirAll("videotiktok", 0755)
	return "videotiktok"
}

type TikWMResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data *struct {
		ID     string   `json:"id"`
		Play   string   `json:"play"`
		HDPlay string   `json:"hdplay"`
		WMPlay string   `json:"wmplay"`
		Images []string `json:"images"`
		Music  string   `json:"music"`
	} `json:"data"`
}

type Service struct {
	store  *storage.MySQLStore
	folder string
}

func NewService(store *storage.MySQLStore) *Service {
	f := getVideoFolder()
	return &Service{store: store, folder: f}
}

func (s *Service) HandleMessage(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}

	// Bỏ qua các tin nhắn cũ hơn 2 phút khi mới mở bot
	if !m.Timestamp.IsZero() && time.Since(m.Timestamp) > 2*time.Minute {
		return
	}

	match := tiktokRegex.FindString(m.Content)
	if match == "" {
		return
	}

	// Chống xử lý trùng lặp tin nhắn
	if _, loaded := processedMsgs.LoadOrStore(m.ID, time.Now()); loaded {
		return
	}

	settings := s.store.GetChannelSettings(m.GuildID, m.ChannelID)
	if !settings.TikTokEnabled {
		return
	}

	go s.processTikTokURL(sess, m, match)
}

func makeHTTPRequest(targetURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.tiktok.com/")
	req.Header.Set("Accept", "*/*")
	return httpClient.Do(req)
}

func (s *Service) processTikTokURL(sess *discordgo.Session, m *discordgo.MessageCreate, rawURL string) {
	// Bật hiệu ứng typing tức thì để phản hồi ngay lập tức (delay ~ 0)
	_ = sess.ChannelTyping(m.ChannelID)

	apiURL := fmt.Sprintf("https://www.tikwm.com/api/?url=%s&hd=1", url.QueryEscape(rawURL))
	resp, err := makeHTTPRequest(apiURL)
	if err != nil {
		log.Printf("Lỗi gọi TikWM API: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("TikWM API trả về status: %d", resp.StatusCode)
		return
	}

	var apiData TikWMResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiData); err != nil || apiData.Data == nil {
		log.Printf("Lỗi giải mã TikWM JSON hoặc data rỗng: %v", err)
		return
	}

	vData := apiData.Data
	vid := vData.ID
	if vid == "" {
		vid = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	// Khóa thao tác theo video ID
	mu := getVideoMutex(vid)
	mu.Lock()
	defer mu.Unlock()

	// Xử lý bài đăng dạng ảnh (Photo / Slideshow)
	if len(vData.Images) > 0 {
		s.handleSlides(sess, m, vid, vData.Images, vData.Music)
		return
	}

	// Luôn luôn ưu tiên video có chất lượng cao nhất (HDPlay), sau đó tới Play và WMPlay
	isHD := false
	videoURL := vData.HDPlay
	if videoURL != "" {
		isHD = true
	} else if vData.Play != "" {
		videoURL = vData.Play
	} else if vData.WMPlay != "" {
		videoURL = vData.WMPlay
	}
	if videoURL == "" {
		return
	}
	if !strings.HasPrefix(videoURL, "http") {
		videoURL = "https://www.tikwm.com" + videoURL
	}

	fileName := fmt.Sprintf("%s.mp4", vid)
	if isHD {
		fileName = fmt.Sprintf("%s_hd.mp4", vid)
	}
	rawFilePath := filepath.Join(s.folder, fileName)

	if rfi, err := os.Stat(rawFilePath); err == nil && rfi.Size() == 0 {
		_ = os.Remove(rawFilePath)
	}

	if _, err := os.Stat(rawFilePath); os.IsNotExist(err) {
		if err := downloadFile(videoURL, rawFilePath); err != nil {
			log.Printf("Lỗi tải video TikTok: %v", err)
			return
		}
	}

	var maxSizeBytes uint64 = 10 * 1024 * 1024
	if m.GuildID != "" {
		if guild, err := sess.Guild(m.GuildID); err == nil && guild != nil {
			maxSizeBytes = getGuildMaxUploadLimit(guild)
		}
	}

	fi, err := os.Stat(rawFilePath)
	if err != nil || fi.Size() == 0 {
		return
	}

	sendFilePath := rawFilePath

	// Nếu video dưới giới hạn của server: GỬI NGAY LẬP TỨC (0s delay, không qua FFmpeg)
	if uint64(fi.Size()) > maxSizeBytes {
		// Chỉ khi vượt dung lượng server mới nén với preset veryfast chất lượng cao
		maxSizeMB := int(maxSizeBytes / (1024 * 1024))
		compressedName := fmt.Sprintf("%s_fast_%dmb.mp4", vid, maxSizeMB)
		if isHD {
			compressedName = fmt.Sprintf("%s_hd_fast_%dmb.mp4", vid, maxSizeMB)
		}
		compressedPath := filepath.Join(s.folder, compressedName)

		pfi, perr := os.Stat(compressedPath)
		if perr != nil || uint64(pfi.Size()) > maxSizeBytes || pfi.Size() == 0 {
			compressVideoUltraFast(rawFilePath, compressedPath, maxSizeBytes)
		}

		if pfi2, err2 := os.Stat(compressedPath); err2 == nil && pfi2.Size() > 0 && uint64(pfi2.Size()) <= maxSizeBytes {
			sendFilePath = compressedPath
		} else {
			// Nếu sau khi nén vẫn quá dung lượng server, gửi direct link
			_, _ = sess.ChannelMessageSendReply(m.ChannelID, videoURL, m.Reference())
			_, _ = sess.ChannelMessageEditComplex(&discordgo.MessageEdit{
				Channel: m.ChannelID,
				ID:      m.ID,
				Flags:   discordgo.MessageFlagsSuppressEmbeds,
			})
			return
		}
	}

	f, err := os.Open(sendFilePath)
	if err != nil {
		return
	}
	defer f.Close()

	msgSend := &discordgo.MessageSend{
		Reference: m.Reference(),
		Files: []*discordgo.File{
			{
				Name:   fmt.Sprintf("%s.mp4", vid),
				Reader: f,
			},
		},
	}
	_, _ = sess.ChannelMessageSendComplex(m.ChannelID, msgSend)

	_, _ = sess.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: m.ChannelID,
		ID:      m.ID,
		Flags:   discordgo.MessageFlagsSuppressEmbeds,
	})
}

func (s *Service) handleSlides(sess *discordgo.Session, m *discordgo.MessageCreate, vid string, images []string, audioURL string) {
	slideFolder := filepath.Join(s.folder, vid)
	_ = os.MkdirAll(slideFolder, 0755)

	var audioData []byte
	var audioName string
	var wg sync.WaitGroup

	// 1. Tải file âm thanh nền nếu có song song
	if audioURL != "" {
		if !strings.HasPrefix(audioURL, "http") {
			audioURL = "https://www.tikwm.com" + audioURL
		}
		audioPath := filepath.Join(slideFolder, "audio.mp3")
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := os.Stat(audioPath); os.IsNotExist(err) {
				_ = downloadFile(audioURL, audioPath)
			}
			if data, err := os.ReadFile(audioPath); err == nil && len(data) > 0 {
				audioData = data
				audioName = "audio.mp3"
			}
		}()
	}

	// 2. Tải toàn bộ danh sách hình ảnh song song (Parallel) để đạt tốc độ tức thì
	type slideFile struct {
		index int
		name  string
		data  []byte
	}
	results := make([]slideFile, len(images))

	for i, imgURL := range images {
		idx := i
		url := imgURL
		if !strings.HasPrefix(url, "http") {
			url = "https://www.tikwm.com" + url
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			imgPath := filepath.Join(slideFolder, fmt.Sprintf("%d.jpg", idx))
			if _, err := os.Stat(imgPath); os.IsNotExist(err) {
				_ = downloadFile(url, imgPath)
			}
			if data, err := os.ReadFile(imgPath); err == nil && len(data) > 0 {
				results[idx] = slideFile{
					index: idx,
					name:  fmt.Sprintf("image_%d.jpg", idx+1),
					data:  data,
				}
			}
		}()
	}

	wg.Wait()

	var downloadedImages []slideFile
	for _, item := range results {
		if len(item.data) > 0 {
			downloadedImages = append(downloadedImages, item)
		}
	}

	if len(downloadedImages) == 0 && len(audioData) == 0 {
		return
	}

	// 3. Phân cụm gửi an toàn (tối đa 10 ảnh hoặc < 7.5 MB mỗi cụm)
	const maxBatchBytes = 7500000
	const maxFilesPerBatch = 10

	var batches [][]*discordgo.File
	var currentBatch []*discordgo.File
	var currentBatchSize int

	if len(audioData) > 0 {
		currentBatch = append(currentBatch, &discordgo.File{
			Name:   audioName,
			Reader: bytes.NewReader(audioData),
		})
		currentBatchSize += len(audioData)
	}

	for _, img := range downloadedImages {
		imgSize := len(img.data)
		if len(currentBatch) > 0 && (len(currentBatch) >= maxFilesPerBatch || currentBatchSize+imgSize > maxBatchBytes) {
			batches = append(batches, currentBatch)
			currentBatch = nil
			currentBatchSize = 0
		}

		currentBatch = append(currentBatch, &discordgo.File{
			Name:   img.name,
			Reader: bytes.NewReader(img.data),
		})
		currentBatchSize += imgSize
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	for _, batch := range batches {
		_, _ = sess.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Reference: m.Reference(),
			Files:     batch,
		})
		time.Sleep(200 * time.Millisecond)
	}

	_, _ = sess.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: m.ChannelID,
		ID:      m.ID,
		Flags:   discordgo.MessageFlagsSuppressEmbeds,
	})
}

func getGuildMaxUploadLimit(guild *discordgo.Guild) uint64 {
	if guild == nil {
		return 10 * 1024 * 1024
	}
	switch guild.PremiumTier {
	case discordgo.PremiumTier3:
		return 100 * 1024 * 1024
	case discordgo.PremiumTier2:
		return 50 * 1024 * 1024
	default:
		return 10 * 1024 * 1024
	}
}

func downloadFile(urlStr, destPath string) error {
	resp, err := makeHTTPRequest(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	tempPath := fmt.Sprintf("%s.tmp.%d", destPath, time.Now().UnixNano())
	out, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, resp.Body)
	_ = out.Close()

	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}

	fi, err := os.Stat(tempPath)
	if err != nil || fi.Size() == 0 {
		_ = os.Remove(tempPath)
		return fmt.Errorf("file tải về rỗng")
	}

	_ = os.Remove(destPath)
	if err := os.Rename(tempPath, destPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func getVideoDuration(path string) float64 {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 30.0
	}
	dur, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || dur <= 0 {
		return 30.0
	}
	return dur
}

func compressVideoUltraFast(inputPath, outputPath string, maxSizeBytes uint64) bool {
	duration := getVideoDuration(inputPath)
	if duration <= 0 {
		duration = 30.0
	}

	targetSizeBytes := float64(maxSizeBytes) * 0.92
	audioBits := 128.0 * 1000 * duration
	remainingBits := targetSizeBytes*8 - audioBits
	videoBitrateKbps := int(remainingBits / duration / 1000)

	if videoBitrateKbps < 200 {
		videoBitrateKbps = 200
	}
	if videoBitrateKbps > 6000 {
		videoBitrateKbps = 6000
	}

	tempOutput := fmt.Sprintf("%s.tmp.%d.mp4", outputPath, time.Now().UnixNano())

	// Sử dụng preset veryfast và sao chép trực tiếp luồng audio gốc (-c:a copy) để giữ chất lượng cao nhất
	cmd := exec.Command("ffmpeg", "-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-b:v", fmt.Sprintf("%dk", videoBitrateKbps),
		"-c:a", "copy",
		"-movflags", "+faststart",
		tempOutput,
	)

	err := cmd.Run()
	if err != nil {
		log.Printf("FFmpeg ultrafast lỗi: %v", err)
		_ = os.Remove(tempOutput)
		return false
	}

	tfi, terr := os.Stat(tempOutput)
	if terr != nil || tfi.Size() == 0 {
		_ = os.Remove(tempOutput)
		return false
	}

	_ = os.Remove(outputPath)
	_ = os.Rename(tempOutput, outputPath)
	return true
}
