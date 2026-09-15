package tiktok

import (
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
	"time"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

var (
	tiktokRegex = regexp.MustCompile(`https:\/\/(?:m|www|vt)?\.tiktok\.com\/\S+`)
	client      = &http.Client{Timeout: 30 * time.Second}
)

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

	match := tiktokRegex.FindString(m.Content)
	if match == "" {
		return
	}

	settings := s.store.GetChannelSettings(m.GuildID, m.ChannelID)
	if !settings.TikTokEnabled {
		return
	}

	go s.processTikTokURL(sess, m, match)
}

func (s *Service) processTikTokURL(sess *discordgo.Session, m *discordgo.MessageCreate, rawURL string) {
	apiURL := fmt.Sprintf("https://www.tikwm.com/api/?url=%s&hd=1", url.QueryEscape(rawURL))
	resp, err := client.Get(apiURL)
	if err != nil {
		log.Printf("Lỗi gọi TikWM API: %v", err)
		return
	}
	defer resp.Body.Close()

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

	if len(vData.Images) > 0 {
		s.handleSlides(sess, m, vid, vData.Images, vData.Music)
		return
	}

	videoURL := vData.HDPlay
	if videoURL == "" {
		videoURL = vData.Play
	}
	if videoURL == "" {
		videoURL = vData.WMPlay
	}
	if videoURL == "" {
		return
	}
	if !strings.HasPrefix(videoURL, "http") {
		videoURL = "https://www.tikwm.com" + videoURL
	}

	rawFilePath := filepath.Join(s.folder, fmt.Sprintf("%s.mp4", vid))
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
	if err != nil {
		return
	}

	sendFilePath := rawFilePath

	if uint64(fi.Size()) > maxSizeBytes {
		maxSizeMB := int(maxSizeBytes / (1024 * 1024))
		compressedPath := filepath.Join(s.folder, fmt.Sprintf("%s_compressed_%dmb.mp4", vid, maxSizeMB))

		cfi, cerr := os.Stat(compressedPath)
		if cerr != nil || uint64(cfi.Size()) > maxSizeBytes {
			compressVideo(rawFilePath, compressedPath, maxSizeBytes)
		}

		if cfi2, err2 := os.Stat(compressedPath); err2 == nil && cfi2.Size() > 0 {
			sendFilePath = compressedPath
		}
	}

	finalFi, err := os.Stat(sendFilePath)
	if err == nil && uint64(finalFi.Size()) > maxSizeBytes {
		_, _ = sess.ChannelMessageSendReply(m.ChannelID, videoURL, m.Reference())
		_, _ = sess.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: m.ChannelID,
			ID:      m.ID,
			Flags:   discordgo.MessageFlagsSuppressEmbeds,
		})
		return
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

	var files []*discordgo.File

	if audioURL != "" {
		if !strings.HasPrefix(audioURL, "http") {
			audioURL = "https://www.tikwm.com" + audioURL
		}
		audioPath := filepath.Join(slideFolder, "audio.mp3")
		if _, err := os.Stat(audioPath); os.IsNotExist(err) {
			_ = downloadFile(audioURL, audioPath)
		}
		if af, err := os.Open(audioPath); err == nil {
			defer af.Close()
			files = append(files, &discordgo.File{Name: "audio.mp3", Reader: af})
		}
	}

	for i, imgURL := range images {
		imgPath := filepath.Join(slideFolder, fmt.Sprintf("%d.jpg", i))
		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			_ = downloadFile(imgURL, imgPath)
		}
		if imgFile, err := os.Open(imgPath); err == nil {
			defer imgFile.Close()
			files = append(files, &discordgo.File{Name: fmt.Sprintf("image_%d.jpg", i), Reader: imgFile})
		}
	}

	for i := 0; i < len(files); i += 10 {
		end := i + 10
		if end > len(files) {
			end = len(files)
		}
		chunk := files[i:end]
		_, _ = sess.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Reference: m.Reference(),
			Files:     chunk,
		})
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
	resp, err := client.Get(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
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

func compressVideo(inputPath, outputPath string, maxSizeBytes uint64) bool {
	duration := getVideoDuration(inputPath)
	if duration <= 0 {
		duration = 30.0
	}

	maxSizeMB := float64(maxSizeBytes) / (1024 * 1024)
	targetSizeBytes := float64(maxSizeBytes) * 0.90
	audioBitrateKbps := 96.0
	audioBits := audioBitrateKbps * 1000 * duration

	remainingBits := targetSizeBytes*8 - audioBits
	videoBitrateKbps := int(remainingBits / duration / 1000)

	var maxCapBitrate int
	var scaleFilter string
	if maxSizeMB >= 80 {
		maxCapBitrate = 18000
		scaleFilter = "scale=-2:'min(1080,ih)'"
	} else if maxSizeMB >= 40 {
		maxCapBitrate = 12000
		scaleFilter = "scale=-2:'min(1080,ih)'"
	} else {
		maxCapBitrate = 3000
		scaleFilter = "scale=-2:'min(720,ih)'"
	}

	if videoBitrateKbps < 150 {
		videoBitrateKbps = 150
	}
	if videoBitrateKbps > maxCapBitrate {
		videoBitrateKbps = maxCapBitrate
	}

	maxrate := int(float64(videoBitrateKbps) * 1.15)
	bufsize := videoBitrateKbps * 2

	cmd := exec.Command("ffmpeg", "-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-b:v", fmt.Sprintf("%dk", videoBitrateKbps),
		"-maxrate", fmt.Sprintf("%dk", maxrate),
		"-bufsize", fmt.Sprintf("%dk", bufsize),
		"-vf", scaleFilter,
		"-preset", "faster",
		"-c:a", "aac",
		"-b:a", fmt.Sprintf("%dk", int(audioBitrateKbps)),
		outputPath,
	)

	err := cmd.Run()
	if err != nil {
		log.Printf("FFmpeg lỗi: %v", err)
		return false
	}

	fi, err := os.Stat(outputPath)
	return err == nil && fi.Size() > 0
}
