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
	"sync"
	"time"
	"unicode"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type Service struct {
	apiKey        string
	localAIURL    string
	systemPrompt  string
	store         *storage.MySQLStore
	processedMsgs sync.Map
}

func NewService(apiKey, localAIURL string, store *storage.MySQLStore) *Service {
	possiblePaths := []string{"train.txt", "../train.txt", filepath.Join(".", "train.txt")}
	var prompt string
	for _, p := range possiblePaths {
		if data, err := os.ReadFile(p); err == nil {
			prompt = string(data)
			break
		}
	}

	if localAIURL == "" {
		localAIURL = "http://localhost:6660/api"
	}

	svc := &Service{
		apiKey:       apiKey,
		localAIURL:   localAIURL,
		systemPrompt: prompt,
		store:        store,
	}

	// Tự động dọn dẹp cache ID tin nhắn mỗi 5 phút để giải phóng bộ nhớ
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			svc.processedMsgs.Range(func(key, value any) bool {
				if t, ok := value.(time.Time); ok && now.Sub(t) > 10*time.Minute {
					svc.processedMsgs.Delete(key)
				}
				return true
			})
		}
	}()

	return svc
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

	if s.localAIURL == "" && s.apiKey == "" {
		return
	}

	// Chống trùng lặp tin nhắn: nếu tin nhắn này đang hoặc đã được xử lý thì bỏ qua
	if _, loaded := s.processedMsgs.LoadOrStore(m.ID, time.Now()); loaded {
		return
	}

	go s.generateAndReply(sess, m)
}

type localAIRequest struct {
	Prompt string `json:"prompt"`
}

type localAIResponse struct {
	Text           string      `json:"text"`
	Thoughts       interface{} `json:"thoughts"`
	ConversationID string      `json:"conversation_id"`
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

func (s *Service) callLocalAI(prompt string, timeout time.Duration) (string, error) {
	if s.localAIURL == "" {
		return "", fmt.Errorf("local AI URL chưa cấu hình")
	}

	endpoint := strings.TrimRight(s.localAIURL, "/") + "/generate"
	reqBody := localAIRequest{Prompt: prompt}
	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("local AI trả về mã %d", resp.StatusCode)
	}

	var lResp localAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&lResp); err != nil {
		return "", err
	}

	reply := strings.TrimSpace(lResp.Text)
	if reply == "" {
		return "", fmt.Errorf("kết quả từ local AI rỗng")
	}
	return reply, nil
}

func (s *Service) callGemini(prompt string, timeout time.Duration) (string, error) {
	if s.apiKey == "" {
		return "", fmt.Errorf("gemini api key trống")
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
					{Text: prompt},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=%s", s.apiKey)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini trả về mã %d", resp.StatusCode)
	}

	var gResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return "", err
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini không có phản hồi")
	}

	return strings.TrimSpace(gResp.Candidates[0].Content.Parts[0].Text), nil
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

	// Lấy tối đa 10 tin nhắn gần nhất trước tin nhắn này trong kênh để làm ngữ cảnh nhớ lại
	var historyLines []string
	if sess != nil {
		msgs, err := sess.ChannelMessages(m.ChannelID, 10, m.ID, "", "")
		if err == nil && len(msgs) > 0 {
			for i := len(msgs) - 1; i >= 0; i-- {
				oldMsg := msgs[i]
				if oldMsg == nil {
					continue
				}
				c := strings.TrimSpace(oldMsg.Content)
				if c == "" {
					continue
				}
				// Bỏ qua các tin nhắn lệnh bot
				if strings.HasPrefix(c, "/") || strings.HasPrefix(c, "!") || strings.HasPrefix(c, ".") {
					continue
				}
				if sess.State != nil && sess.State.User != nil {
					c = strings.ReplaceAll(c, fmt.Sprintf("<@%s>", sess.State.User.ID), "")
					c = strings.ReplaceAll(c, fmt.Sprintf("<@!%s>", sess.State.User.ID), "")
					c = strings.TrimSpace(c)
				}
				if c == "" {
					continue
				}

				senderName := "Người dùng"
				if oldMsg.Author != nil {
					if sess.State != nil && sess.State.User != nil && oldMsg.Author.ID == sess.State.User.ID {
						senderName = "AI KiyoVN"
					} else if oldMsg.Author.Username != "" {
						senderName = oldMsg.Author.Username
					}
				}
				historyLines = append(historyLines, fmt.Sprintf("- %s: %s", senderName, c))
			}
		}
	}

	var sb strings.Builder
	if s.systemPrompt != "" {
		sb.WriteString(s.systemPrompt)
		sb.WriteString("\n\n")
	}

	if len(historyLines) > 0 {
		sb.WriteString("Ngữ cảnh lịch sử trò chuyện gần đây trong kênh (tối đa 10 tin nhắn trước):\n")
		for _, h := range historyLines {
			sb.WriteString(h)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	userName := "Người dùng"
	if m.Author != nil && m.Author.Username != "" {
		userName = m.Author.Username
	}

	sb.WriteString(fmt.Sprintf("Tin nhắn mới nhất từ %s:\n%s\n\n", userName, cleanContent))
	sb.WriteString("Dựa trên toàn bộ thông tin về KiyoVN và ngữ cảnh lịch sử trò chuyện ở trên, hãy trả lời tin nhắn mới nhất thật ngắn gọn, chính xác, tự nhiên và đúng trọng tâm:")
	userPrompt := sb.String()

	var replyText string
	var err error

	// 1. Ưu tiên Local AI API
	if s.localAIURL != "" {
		replyText, err = s.callLocalAI(userPrompt, 45*time.Second)
		if err != nil {
			log.Printf("Lỗi gọi Local AI: %v", err)
		}
	}

	// 2. Fallback qua Gemini nếu Local AI không trả về
	if replyText == "" && s.apiKey != "" {
		replyText, err = s.callGemini(userPrompt, 30*time.Second)
		if err != nil {
			log.Printf("Lỗi gọi Gemini fallback: %v", err)
		}
	}

	if replyText == "" {
		return
	}

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

	prompt := fmt.Sprintf(`Người dùng Discord cần hỗ trợ nội dung: "%s"
Nhiệm vụ:
1. Đặt 1 tiêu đề tóm tắt vấn đề thật thông minh, ngắn gọn, dưới 35 ký tự, viết hoa chữ đầu câu (Ví dụ: "Tạo tài khoản bachoammo", "Lỗi nạp thẻ", "Quên mật khẩu nick kiyovn"). Tuyệt đối không trích lại cả câu người dùng, không có icon, không có dấu hai chấm.
2. Đặt 1 tên slug kênh Discord bằng chữ thường không dấu, phân tách bằng dấu gạch ngang, bắt đầu bằng ho-tro-, tối đa 22 ký tự (Ví dụ: ho-tro-bachoammo, ho-tro-nap-the, ho-tro-mat-khau).

Trả về đúng 2 dòng:
Dòng 1: Tiêu đề
Dòng 2: Slug`, cleanProblem)

	// 1. Thử gọi Local AI API trước (http://localhost:6660/api)
	if s != nil && s.localAIURL != "" {
		if rawText, err := s.callLocalAI(prompt, 15*time.Second); err == nil {
			if title, slug, ok := parseTicketMeta(rawText); ok {
				return title, slug
			}
		} else {
			log.Printf("Lỗi gọi Local AI cho Ticket Meta: %v", err)
		}
	}

	// 2. Thử fallback qua Gemini nếu có apiKey
	if s != nil && s.apiKey != "" {
		if rawText, err := s.callGemini(prompt, 6*time.Second); err == nil {
			if title, slug, ok := parseTicketMeta(rawText); ok {
				return title, slug
			}
		}
	}

	// 3. Fallback dự phòng thông minh không cần mạng
	return smartTitle, smartSlug
}

func parseTicketMeta(reply string) (string, string, bool) {
	lines := strings.Split(reply, "\n")
	var validLines []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			validLines = append(validLines, l)
		}
	}

	if len(validLines) < 2 {
		return "", "", false
	}

	aiTitle := cleanTicketMetaLine(validLines[0])
	aiSlug := cleanTicketMetaLine(validLines[1])

	aiTitle = strings.ReplaceAll(aiTitle, ":", "")
	aiTitle = strings.TrimSpace(aiTitle)

	aiSlug = strings.ToLower(aiSlug)
	aiSlug = ToSlug(aiSlug)
	if !strings.HasPrefix(aiSlug, "ho-tro-") {
		aiSlug = "ho-tro-" + aiSlug
	}
	if len(aiSlug) > 22 {
		aiSlug = aiSlug[:22]
	}
	if len(aiTitle) > 35 {
		aiTitle = aiTitle[:35]
	}

	if len(aiTitle) > 0 {
		r := []rune(aiTitle)
		r[0] = unicode.ToUpper(r[0])
		aiTitle = string(r)
		return aiTitle, aiSlug, true
	}

	return "", "", false
}

func cleanTicketMetaLine(line string) string {
	line = strings.TrimSpace(line)
	prefixes := []string{"dòng 1:", "dòng 2:", "tiêu đề:", "slug:", "1.", "2.", "title:"}
	lower := strings.ToLower(line)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			line = strings.TrimSpace(line[len(p):])
			lower = strings.ToLower(line)
		}
	}
	line = strings.ReplaceAll(line, "\"", "")
	line = strings.ReplaceAll(line, "`", "")
	line = strings.ReplaceAll(line, "*", "")
	return strings.TrimSpace(line)
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
