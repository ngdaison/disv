package chatai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type Service struct {
	apiKey       string
	systemPrompt string
	store        *storage.MySQLStore
}

func NewService(apiKey string, store *storage.MySQLStore) *Service {
	possiblePaths := []string{"train.txt", "../train.txt", filepath.Join(".", "train.txt")}
	var prompt string
	for _, p := range possiblePaths {
		if data, err := os.ReadFile(p); err == nil {
			prompt = string(data)
			break
		}
	}

	return &Service{
		apiKey:       apiKey,
		systemPrompt: prompt,
		store:        store,
	}
}

func (s *Service) HandleMessage(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return
	}

	settings := s.store.GetChannelSettings(m.GuildID, m.ChannelID)
	isMentioned := false
	if sess.State != nil && sess.State.User != nil {
		for _, user := range m.Mentions {
			if user.ID == sess.State.User.ID {
				isMentioned = true
				break
			}
		}
	}

	if !settings.AIEnabled && !isMentioned {
		return
	}

	if s.apiKey == "" {
		return
	}

	go s.generateAndReply(sess, m)
}

type geminiRequest struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (s *Service) generateAndReply(sess *discordgo.Session, m *discordgo.MessageCreate) {
	_ = sess.ChannelTyping(m.ChannelID)

	cleanContent := m.Content
	if sess.State != nil && sess.State.User != nil {
		cleanContent = strings.ReplaceAll(cleanContent, fmt.Sprintf("<@%s>", sess.State.User.ID), "")
		cleanContent = strings.ReplaceAll(cleanContent, fmt.Sprintf("<@!%s>", sess.State.User.ID), "")
	}
	cleanContent = strings.TrimSpace(cleanContent)

	if cleanContent == "" {
		cleanContent = "Xin chào!"
	}

	userPrompt := cleanContent
	if s.systemPrompt != "" {
		userPrompt = s.systemPrompt + "\n\nNgười dùng hỏi:\n" + cleanContent
	}

	reqBody := geminiRequest{
		Contents: []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{
			{
				Role: "user",
				Parts: []struct {
					Text string `json:"text"`
				}{
					{Text: userPrompt},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=%s", s.apiKey)
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("Lỗi gọi Gemini API: %v", err)
		return
	}
	defer resp.Body.Close()

	var gResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return
	}

	replyText := gResp.Candidates[0].Content.Parts[0].Text

	for len(replyText) > 0 {
		chunkSize := 1950
		if len(replyText) < chunkSize {
			chunkSize = len(replyText)
		}
		chunk := replyText[:chunkSize]
		replyText = replyText[chunkSize:]

		_, _ = sess.ChannelMessageSendReply(m.ChannelID, chunk, m.Reference())
	}
}

func (s *Service) GenerateTicketMeta(problem string) (string, string) {
	cleanProblem := strings.TrimSpace(problem)
	if cleanProblem == "" {
		return "Hỗ trợ thành viên", "ho-tro-thanh-vien"
	}

	smartTitle, smartSlug := GenerateSmartTicketMeta(cleanProblem)

	if s == nil || s.apiKey == "" {
		return smartTitle, smartSlug
	}

	prompt := fmt.Sprintf(`Người dùng Discord cần hỗ trợ nội dung: "%s"
Nhiệm vụ:
1. Đặt 1 tiêu đề tóm tắt vấn đề thật thông minh, ngắn gọn, dưới 35 ký tự, viết hoa chữ đầu câu (Ví dụ: "Tạo tài khoản bachoammo", "Lỗi nạp thẻ", "Quên mật khẩu nick kiyovn"). Tuyệt đối không trích lại cả câu người dùng, không có icon, không có dấu hai chấm.
2. Đặt 1 tên slug kênh Discord bằng chữ thường không dấu, phân tách bằng dấu gạch ngang, bắt đầu bằng ho-tro-, tối đa 22 ký tự (Ví dụ: ho-tro-bachoammo, ho-tro-nap-the, ho-tro-mat-khau).

Trả về đúng 2 dòng:
Dòng 1: Tiêu đề
Dòng 2: Slug`, cleanProblem)

	reqBody := geminiRequest{
		Contents: []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{
			{
				Role: "user",
				Parts: []struct {
					Text string `json:"text"`
				}{
					{Text: prompt},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return smartTitle, smartSlug
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=%s", s.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return smartTitle, smartSlug
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return smartTitle, smartSlug
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return smartTitle, smartSlug
	}

	var gResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return smartTitle, smartSlug
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return smartTitle, smartSlug
	}

	reply := strings.TrimSpace(gResp.Candidates[0].Content.Parts[0].Text)
	lines := strings.Split(reply, "\n")
	var validLines []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			validLines = append(validLines, l)
		}
	}

	if len(validLines) >= 2 {
		aiTitle := strings.ReplaceAll(validLines[0], ":", "")
		aiTitle = strings.ReplaceAll(aiTitle, "\"", "")
		aiTitle = strings.TrimSpace(aiTitle)

		aiSlug := strings.ToLower(validLines[1])
		aiSlug = strings.ReplaceAll(aiSlug, "\"", "")
		aiSlug = ToSlug(aiSlug)
		if !strings.HasPrefix(aiSlug, "ho-tro-") {
			aiSlug = "ho-tro-" + aiSlug
		}
		if len(aiSlug) > 26 {
			aiSlug = aiSlug[:26]
		}
		if len(aiTitle) > 0 {
			r := []rune(aiTitle)
			r[0] = unicode.ToUpper(r[0])
			aiTitle = string(r)
			return aiTitle, aiSlug
		}
	}

	return smartTitle, smartSlug
}

func GenerateSmartTicketMeta(problem string) (string, string) {
	cleanProblem := strings.TrimSpace(problem)
	if cleanProblem == "" {
		return "Hỗ trợ thành viên", "ho-tro-thanh-vien"
	}

	lower := strings.ToLower(cleanProblem)
	words := strings.Fields(cleanProblem)

	intentTitle := ""
	intentSlug := ""

	if strings.Contains(lower, "tạo tài khoản") || strings.Contains(lower, "tạo acc") || strings.Contains(lower, "đăng ký") || strings.Contains(lower, "reg acc") || strings.Contains(lower, "lập nick") || strings.Contains(lower, "tạo nick") {
		intentTitle = "Tạo tài khoản"
		intentSlug = "tao-tai-khoan"
	} else if strings.Contains(lower, "không vào được") || strings.Contains(lower, "không vô được") || strings.Contains(lower, "không thể vào") || strings.Contains(lower, "lỗi vào") || strings.Contains(lower, "không kết nối") {
		intentTitle = "Không vào được"
		intentSlug = "khong-vao-duoc"
	} else if strings.Contains(lower, "quên mật khẩu") || strings.Contains(lower, "mất pass") || strings.Contains(lower, "mất mật khẩu") || strings.Contains(lower, "đổi mật khẩu") || strings.Contains(lower, "đổi pass") || strings.Contains(lower, "pass") || strings.Contains(lower, "mật khẩu") {
		intentTitle = "Mật khẩu tài khoản"
		intentSlug = "mat-khau"
	} else if strings.Contains(lower, "bị ban") || strings.Contains(lower, "bị khóa") || strings.Contains(lower, "unban") || strings.Contains(lower, "mở khóa") || strings.Contains(lower, "bị cấm") {
		intentTitle = "Mở khóa tài khoản"
		intentSlug = "mo-khoa"
	} else if strings.Contains(lower, "nạp thẻ") || strings.Contains(lower, "nạp tiền") || strings.Contains(lower, "donate") || strings.Contains(lower, "chuyển khoản") || strings.Contains(lower, "mua hàng") || strings.Contains(lower, "thanh toán") || strings.Contains(lower, "chưa nhận được") || strings.Contains(lower, "xu") {
		intentTitle = "Hỗ trợ nạp tiền"
		intentSlug = "nap-tien"
	} else if strings.Contains(lower, "mất đồ") || strings.Contains(lower, "mất item") || strings.Contains(lower, "rơi đồ") || strings.Contains(lower, "bị hack") {
		intentTitle = "Hỗ trợ mất đồ"
		intentSlug = "mat-do"
	} else if strings.Contains(lower, "lag") || strings.Contains(lower, "crash") || strings.Contains(lower, "văng game") || strings.Contains(lower, "mất kết nối") || strings.Contains(lower, "disconect") || strings.Contains(lower, "ping cao") {
		intentTitle = "Lỗi kết nối"
		intentSlug = "ket-noi"
	} else if strings.Contains(lower, "bug") || strings.Contains(lower, "lỗi game") || strings.Contains(lower, "báo lỗi") || strings.Contains(lower, "lỗi") {
		intentTitle = "Báo cáo lỗi"
		intentSlug = "bao-loi"
	}

	stopWords := map[string]bool{
		"tôi": true, "tao": true, "em": true, "mình": true, "bạn": true, "ad": true, "admin": true, "bot": true,
		"cho": true, "với": true, "ạ": true, "ơi": true, "nha": true, "nhé": true, "và": true, "để": true,
		"là": true, "cái": true, "này": true, "đó": true, "kia": true, "cần": true, "giúp": true, "hỗ": true,
		"trợ": true, "làm": true, "sao": true, "được": true, "không": true, "thể": true, "thì": true, "mà": true,
		"có": true, "ai": true, "ở": true, "trong": true, "ra": true, "vào": true, "đi": true, "lại": true,
		"từ": true, "của": true, "về": true, "vì": true, "do": true, "bị": true, "các": true, "những": true,
		"một": true, "người": true, "xin": true, "vô": true, "hay": true, "hoặc": true, "nhưng": true,
		"đang": true, "sẽ": true, "đã": true, "quá": true, "rất": true, "lắm": true, "quên": true, "mất": true,
		"lỗi": true, "nick": true, "acc": true, "game": true, "gấp": true,
	}

	var keyTokens []string
	for _, w := range words {
		cleanW := strings.ToLower(strings.Trim(w, ",.?!;:'\"()[]{}<>`~*-_/\\"))
		if cleanW == "" {
			continue
		}
		if !stopWords[cleanW] {
			keyTokens = append(keyTokens, w)
		}
	}

	title := ""
	slug := ""

	if len(keyTokens) > 0 {
		var specificEntity string
		for _, kt := range keyTokens {
			lowerKt := strings.ToLower(kt)
			if !strings.Contains("tạo tài khoản mật khẩu nạp thẻ tiền mất đồ rương game kết nối server", lowerKt) {
				specificEntity = kt
				break
			}
		}

		if intentTitle != "" {
			if specificEntity != "" {
				title = fmt.Sprintf("%s %s", intentTitle, specificEntity)
				slug = fmt.Sprintf("ho-tro-%s", ToSlug(specificEntity))
			} else {
				title = intentTitle
				if len(keyTokens) > 0 && len(keyTokens) <= 2 {
					title = fmt.Sprintf("%s %s", intentTitle, strings.Join(keyTokens, " "))
				}
				slug = fmt.Sprintf("ho-tro-%s", intentSlug)
			}
		} else {
			numTokens := 3
			if len(keyTokens) < numTokens {
				numTokens = len(keyTokens)
			}
			picked := keyTokens[:numTokens]
			title = strings.Join(picked, " ")
			slug = "ho-tro-" + ToSlug(strings.Join(picked, "-"))
		}
	} else {
		if intentTitle != "" {
			title = intentTitle
			slug = fmt.Sprintf("ho-tro-%s", intentSlug)
		} else {
			title = "Hỗ trợ thành viên"
			slug = "ho-tro-thanh-vien"
		}
	}

	r := []rune(title)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
		title = string(r)
	}

	title = strings.ReplaceAll(title, ":", "")
	if len(title) > 35 {
		title = title[:35]
	}

	slug = strings.ToLower(slug)
	slug = ToSlug(slug)
	if !strings.HasPrefix(slug, "ho-tro-") {
		slug = "ho-tro-" + slug
	}
	if len(slug) > 22 {
		slug = slug[:22]
	}

	return title, slug
}

func ToSlug(s string) string {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer(
		"à", "a", "á", "a", "ả", "a", "ã", "a", "ạ", "a",
		"ă", "a", "ằ", "a", "ắ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
		"â", "a", "ầ", "a", "ấ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a",
		"è", "e", "é", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e",
		"ê", "e", "ề", "e", "ế", "e", "ể", "e", "ễ", "e", "ệ", "e",
		"ì", "i", "í", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
		"ò", "o", "ó", "o", "ỏ", "o", "õ", "o", "ọ", "o",
		"ô", "o", "ồ", "o", "ố", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
		"ơ", "o", "ờ", "o", "ớ", "o", "ở", "o", "ỡ", "o", "ợ", "o",
		"ù", "u", "ú", "u", "ủ", "u", "ũ", "u", "ụ", "u",
		"ư", "u", "ừ", "u", "ứ", "u", "ử", "u", "ữ", "u", "ự", "u",
		"ỳ", "y", "ý", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y",
		"đ", "d",
	)
	s = replacer.Replace(s)

	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		res = "ticket"
	}
	return res
}
