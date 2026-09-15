package wordchain

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"botdis/internal/discord"
	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type userMsgRecord struct {
	msgID     string
	content   string
	timestamp time.Time
}

type Service struct {
	store          *storage.MySQLStore
	dict           *Dictionary
	lock           sync.RWMutex
	channels       map[string]*storage.WordChainChannel
	userViolations map[string]int
	userHistory    map[string][]userMsgRecord
}

func NewService(store *storage.MySQLStore, dict *Dictionary) *Service {
	s := &Service{
		store:          store,
		dict:           dict,
		channels:       make(map[string]*storage.WordChainChannel),
		userViolations: make(map[string]int),
		userHistory:    make(map[string][]userMsgRecord),
	}

	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		for range ticker.C {
			s.cleanOldHistory()
		}
	}()

	return s
}

func (s *Service) cleanOldHistory() {
	s.lock.Lock()
	defer s.lock.Unlock()

	cutoff := time.Now().Add(-2 * time.Minute)
	for key, msgs := range s.userHistory {
		var valid []userMsgRecord
		for _, m := range msgs {
			if m.timestamp.After(cutoff) {
				valid = append(valid, m)
			}
		}
		if len(valid) == 0 {
			delete(s.userHistory, key)
		} else {
			s.userHistory[key] = valid
		}
	}
}

func (s *Service) RegisterRoutes(r *discord.Router) {
	r.RegisterCommand("noitu", s.handleNoiTuCommand)
	r.RegisterCommand("tratu", s.handleTraTuCommand)

	r.RegisterComponent(BtnToggleChannel, s.handleToggleChannel)
	r.RegisterComponent(BtnNewGame, s.handleNewGame)
	r.RegisterComponent(BtnLeaderboard, s.handleLeaderboard)
	r.RegisterComponent(BtnMyStats, s.handleMyStats)
	r.RegisterComponent(BtnLookupModal, s.handlePromptLookup)
	r.RegisterComponent(BtnHint, s.handleHint)
	r.RegisterComponent(BtnToggleSolo, s.handleToggleSolo)
	r.RegisterComponent(BtnRules, s.handleRules)

	r.RegisterModal(ModalLookupSubmit, s.handleSubmitLookup)
}

func (s *Service) getOrLoadChannel(channelID, guildID string) *storage.WordChainChannel {
	s.lock.Lock()
	defer s.lock.Unlock()

	if ch, ok := s.channels[channelID]; ok {
		return ch
	}

	ch, err := s.store.GetWordChainChannel(channelID)
	if err != nil {
		log.Printf("Lỗi tải thông tin kênh nối từ %s: %v", channelID, err)
	}
	if ch == nil {
		ch = &storage.WordChainChannel{
			ChannelID: channelID,
			GuildID:   guildID,
			IsActive:  false,
			AllowSolo: false,
			UsedWords: []string{},
		}
	}
	s.channels[channelID] = ch
	return ch
}

func (s *Service) getActiveChannelIDs(sess *discordgo.Session, guildID string) []string {
	dbIDs, err := s.store.GetActiveWordChainChannelIDs(guildID)
	if err != nil {
		log.Printf("Lỗi lấy danh sách kênh nối từ hoạt động: %v", err)
	}

	activeMap := make(map[string]bool)
	for _, id := range dbIDs {
		activeMap[id] = true
	}

	s.lock.RLock()
	for cid, ch := range s.channels {
		if ch.GuildID == guildID {
			if ch.IsActive {
				activeMap[cid] = true
			} else {
				delete(activeMap, cid)
			}
		}
	}
	s.lock.RUnlock()

	var result []string
	for cid := range activeMap {
		result = append(result, cid)
	}
	sort.Strings(result)
	return result
}

// handleNoiTuCommand mở ngay bảng điều khiển tương tác (chỉ người dùng lệnh mới nhìn thấy)
func (s *Service) handleNoiTuCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	ch := s.getOrLoadChannel(i.ChannelID, i.GuildID)
	isAdmin := s.isUserAdmin(sess, i.GuildID, i.Member.User.ID)

	if !ch.IsActive {
		activeIDs := s.getActiveChannelIDs(sess, i.GuildID)
		var content string
		if len(activeIDs) > 0 {
			var mentions []string
			for _, id := range activeIDs {
				mentions = append(mentions, fmt.Sprintf("<#%s>", id))
			}
			content = "Lệnh này chỉ có thể sử dụng trong kênh " + strings.Join(mentions, " ")
		} else {
			content = "Chưa có kênh nào được kích hoạt nối từ"
		}

		var components []discordgo.MessageComponent
		if isAdmin {
			components = []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Kích hoạt nối từ tại kênh này",
							CustomID: BtnToggleChannel,
							Style:    discordgo.SuccessButton,
						},
					},
				},
			}
		}

		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content:    content,
				Components: components,
				Flags:      discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := BuildControlPanelEmbed(ch, i.ChannelID)
	components := BuildControlPanelComponents(ch, isAdmin)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

// handleTraTuCommand tra cứu nhanh từ điển
func (s *Service) handleTraTuCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	wordToLookup := ""
	if len(data.Options) > 0 {
		wordToLookup = data.Options[0].StringValue()
	}

	wordToLookup = strings.ToLower(strings.TrimSpace(wordToLookup))
	if wordToLookup == "" {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Vui lòng nhập từ muốn tra",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	meanings, _ := s.dict.GetDefinitions(wordToLookup)
	embed := BuildWordDefinitionEmbed(wordToLookup, meanings)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (s *Service) handleToggleChannel(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !s.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Bạn cần quyền Administrator",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	ch := s.getOrLoadChannel(i.ChannelID, i.GuildID)
	s.lock.Lock()
	ch.IsActive = !ch.IsActive
	if ch.IsActive && ch.CurrentWord == "" {
		if startWord, err := s.dict.GetRandomStartWord(); err == nil {
			ch.CurrentWord = startWord
			ch.UsedWords = []string{startWord}
		}
	}
	s.lock.Unlock()

	_ = s.store.SaveWordChainChannel(ch)

	isAdmin := s.isUserAdmin(sess, i.GuildID, i.Member.User.ID)
	embed := BuildControlPanelEmbed(ch, i.ChannelID)
	components := BuildControlPanelComponents(ch, isAdmin)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    " ",
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})

	if ch.IsActive {
		parts := strings.Fields(ch.CurrentWord)
		nextSyl := ""
		if len(parts) >= 2 {
			nextSyl = parts[1]
		}
		_, _ = sess.ChannelMessageSend(i.ChannelID, fmt.Sprintf("Kênh nối từ đã được kích hoạt\nTừ mở màn **%s** bắt đầu bằng **%s**",
			capitalizeFirst(ch.CurrentWord), nextSyl))
	} else {
		_, _ = sess.ChannelMessageSend(i.ChannelID, "Kênh nối từ đã tạm dừng")
	}
}

func (s *Service) handleNewGame(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !s.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Bạn cần quyền Administrator",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	ch := s.startNewGame(sess, i.ChannelID, i.GuildID, "")

	isAdmin := s.isUserAdmin(sess, i.GuildID, i.Member.User.ID)
	embed := BuildControlPanelEmbed(ch, i.ChannelID)
	components := BuildControlPanelComponents(ch, isAdmin)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})

	parts := strings.Fields(ch.CurrentWord)
	nextSyl := ""
	if len(parts) >= 2 {
		nextSyl = parts[1]
	}

	_, _ = sess.ChannelMessageSend(i.ChannelID, fmt.Sprintf("Ván mới bắt đầu với từ **%s**\nBắt đầu bằng **%s**",
		capitalizeFirst(ch.CurrentWord), nextSyl))
}

func (s *Service) startNewGame(sess *discordgo.Session, channelID, guildID, customWord string) *storage.WordChainChannel {
	ch := s.getOrLoadChannel(channelID, guildID)
	s.lock.Lock()
	defer s.lock.Unlock()

	word := strings.ToLower(strings.TrimSpace(customWord))
	if word == "" || !s.dict.IsValidWord(word) || len(strings.Fields(word)) != 2 {
		randomWord, err := s.dict.GetRandomStartWord()
		if err == nil {
			word = randomWord
		} else {
			word = "học tập"
		}
	}

	ch.CurrentWord = word
	ch.LastUserID = ""
	ch.CurrentStreak = 0
	ch.UsedWords = []string{word}

	_ = s.store.SaveWordChainChannel(ch)
	return ch
}

func (s *Service) handleLeaderboard(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	leaderboard, err := s.store.GetWordChainLeaderboard(i.GuildID, 10)
	if err != nil {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Không thể tải bảng xếp hạng",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	guildName := "Server"
	if g, err := sess.Guild(i.GuildID); err == nil && g != nil {
		guildName = g.Name
	}
	embed := BuildLeaderboardEmbed(guildName, leaderboard)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func (s *Service) handleMyStats(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	stat, err := s.store.GetWordChainUserStat(i.GuildID, i.Member.User.ID)
	if err != nil {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Không thể tải thông tin cá nhân",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := BuildUserStatEmbed(i.Member.User, stat)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func (s *Service) handlePromptLookup(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	modal := BuildLookupModal()
	_ = sess.InteractionRespond(i.Interaction, modal)
}

func (s *Service) handleSubmitLookup(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	wordToLookup := ""
	for _, row := range data.Components {
		if actionRow, ok := row.(*discordgo.ActionsRow); ok {
			for _, comp := range actionRow.Components {
				if textInput, ok := comp.(*discordgo.TextInput); ok {
					if textInput.CustomID == ModalInputWord {
						wordToLookup = textInput.Value
					}
				}
			}
		}
	}

	wordToLookup = strings.ToLower(strings.TrimSpace(wordToLookup))
	if wordToLookup == "" {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Vui lòng nhập từ muốn tra",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	meanings, _ := s.dict.GetDefinitions(wordToLookup)
	embed := BuildWordDefinitionEmbed(wordToLookup, meanings)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func (s *Service) handleHint(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	ch := s.getOrLoadChannel(i.ChannelID, i.GuildID)
	if ch.CurrentWord == "" {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Ván chơi chưa bắt đầu",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	parts := strings.Fields(ch.CurrentWord)
	if len(parts) < 2 {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Từ hiện tại không hợp lệ",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	lastSyllable := parts[len(parts)-1]
	candidates, _ := s.dict.FindNextWords(lastSyllable, 5)

	var available []string
	for _, cand := range candidates {
		used := false
		for _, u := range ch.UsedWords {
			if u == cand {
				used = true
				break
			}
		}
		if !used {
			available = append(available, fmt.Sprintf("`%s`", cand))
		}
	}

	hintMsg := ""
	if len(available) == 0 {
		hintMsg = fmt.Sprintf("Không còn gợi ý nào cho chữ '%s'", lastSyllable)
	} else {
		hintMsg = fmt.Sprintf("Gợi ý các từ bắt đầu bằng **%s**\n%s", lastSyllable, strings.Join(available, ", "))
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: hintMsg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func (s *Service) handleToggleSolo(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !s.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Bạn cần quyền Administrator",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	ch := s.getOrLoadChannel(i.ChannelID, i.GuildID)
	s.lock.Lock()
	ch.AllowSolo = !ch.AllowSolo
	s.lock.Unlock()

	_ = s.store.SaveWordChainChannel(ch)

	isAdmin := s.isUserAdmin(sess, i.GuildID, i.Member.User.ID)
	embed := BuildControlPanelEmbed(ch, i.ChannelID)
	components := BuildControlPanelComponents(ch, isAdmin)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleRules(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := BuildRulesEmbed()
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

// HandleMessage xử lý tự động khi có tin nhắn chat trong Kênh Nối Từ
// CHỈ thả reaction icon, KHÔNG gửi bất kỳ tin nhắn cảnh báo text nào!
func (s *Service) HandleMessage(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}

	ch := s.getOrLoadChannel(m.ChannelID, m.GuildID)
	if !ch.IsActive {
		return
	}

	// Bỏ qua nếu là lệnh slash hoặc tin nhắn bắt đầu bằng dấu gạch chéo /
	content := strings.TrimSpace(m.Content)
	if strings.HasPrefix(content, "/") {
		return
	}

	// Chống Spam & Nội Dung: Xóa Link, Media (Ảnh/Video), File hoặc Sticker trong Kênh Nối Từ
	if hasLink(m.Content) || len(m.Attachments) > 0 || len(m.StickerItems) > 0 {
		_ = sess.ChannelMessageDelete(m.ChannelID, m.ID)
		return
	}

	// Chuẩn hóa từ
	cleaned := strings.ToLower(content)
	cleaned = strings.Trim(cleaned, ".,!?~`@#$%^&*()_+-=[]{}|;':\"<>/\\")
	parts := strings.Fields(cleaned)

	// Ghi nhận lịch sử tin nhắn của người dùng trong kênh này
	uKey := m.ChannelID + "_" + m.Author.ID
	now := time.Now()

	s.lock.Lock()
	history := s.userHistory[uKey]
	currentRec := userMsgRecord{
		msgID:     m.ID,
		content:   cleaned,
		timestamp: now,
	}
	history = append(history, currentRec)
	s.userHistory[uKey] = history
	s.lock.Unlock()

	// 1. Anti Fast: Xóa toàn bộ tin nhắn khi gửi quá nhanh qua kênh (>= 2 tin trong 1.5s)
	fastCount := 0
	var fastMsgs []string
	for i := len(history) - 1; i >= 0; i-- {
		if now.Sub(history[i].timestamp) <= 1500*time.Millisecond {
			fastCount++
			fastMsgs = append(fastMsgs, history[i].msgID)
		} else {
			break
		}
	}
	if fastCount >= 2 {
		for _, msgID := range fastMsgs {
			_ = sess.ChannelMessageDelete(m.ChannelID, msgID)
		}
		return
	}

	// 2. Anti Dup: Xóa toàn bộ tin nhắn khi gửi trùng lặp qua kênh (trong vòng 30s)
	if cleaned != "" {
		dupCount := 0
		var dupMsgs []string
		for _, rec := range history {
			if now.Sub(rec.timestamp) <= 30*time.Second && rec.content == cleaned {
				dupCount++
				dupMsgs = append(dupMsgs, rec.msgID)
			}
		}
		if dupCount >= 2 {
			for _, msgID := range dupMsgs {
				_ = sess.ChannelMessageDelete(m.ChannelID, msgID)
			}
			return
		}
	}

	// 3. Nếu có 1 người trả lời rồi mà mình còn trả lời tiếp: XÓA LUÔN TIN NHẮN và áp dụng phạt chống spam
	if !ch.AllowSolo && ch.LastUserID == m.Author.ID {
		_ = sess.ChannelMessageDelete(m.ChannelID, m.ID)

		s.lock.Lock()
		vKey := m.ChannelID + "_" + m.Author.ID
		s.userViolations[vKey]++
		vCount := s.userViolations[vKey]
		s.lock.Unlock()

		_ = s.store.AddWordChainScore(m.GuildID, m.Author.ID, 0, false, 0)

		switch vCount {
		case 1, 2:
			// Lần 1 và 2: Xóa luôn tin nhắn, không để lại rác kênh
		case 3:
			s.sendTemporaryMessage(sess, m.ChannelID, fmt.Sprintf("<@%s> Bạn vừa trả lời lượt trước rồi, vui lòng đợi người khác nối tiếp", m.Author.ID), 5*time.Second)
		case 4:
			s.sendTemporaryMessage(sess, m.ChannelID, fmt.Sprintf("<@%s> Cảnh báo lần 2! Tiếp tục gửi khi chưa đến lượt sẽ bị tạm khóa chat", m.Author.ID), 5*time.Second)
		case 5:
			until := time.Now().Add(1 * time.Hour)
			_ = sess.GuildMemberTimeout(m.GuildID, m.Author.ID, &until)
			s.sendTemporaryMessage(sess, m.ChannelID, fmt.Sprintf("Đã tạm khóa chat <@%s> trong 1 giờ do spam gửi liên tiếp khi chưa đến lượt", m.Author.ID), 6*time.Second)
		case 6:
			until := time.Now().Add(24 * time.Hour)
			_ = sess.GuildMemberTimeout(m.GuildID, m.Author.ID, &until)
			s.sendTemporaryMessage(sess, m.ChannelID, fmt.Sprintf("Đã tạm khóa chat <@%s> trong 24 giờ do vi phạm lần 4", m.Author.ID), 6*time.Second)
		default:
			until := time.Now().Add(168 * time.Hour)
			_ = sess.GuildMemberTimeout(m.GuildID, m.Author.ID, &until)
			s.sendTemporaryMessage(sess, m.ChannelID, fmt.Sprintf("Đã tạm khóa chat <@%s> trong 7 ngày do vi phạm nhiều lần", m.Author.ID), 6*time.Second)
		}
		return
	}

	// Kiểm tra độ dài từ: phải đúng 2 tiếng
	if len(parts) != 2 {
		_ = sess.MessageReactionAdd(m.ChannelID, m.ID, "⚠️")
		return
	}

	// Kiểm tra âm tiết đầu tiên có khớp với âm tiết cuối của từ trước không
	if ch.CurrentWord != "" {
		prevParts := strings.Fields(ch.CurrentWord)
		if len(prevParts) >= 2 {
			expectedPrefix := prevParts[len(prevParts)-1]
			if parts[0] != expectedPrefix {
				_ = sess.MessageReactionAdd(m.ChannelID, m.ID, "❌")
				_ = s.store.AddWordChainScore(m.GuildID, m.Author.ID, 0, false, 0)
				return
			}
		}
	}

	// Kiểm tra từ đã dùng trong ván hiện tại chưa
	for _, used := range ch.UsedWords {
		if used == cleaned {
			_ = sess.MessageReactionAdd(m.ChannelID, m.ID, "🔁")
			_ = s.store.AddWordChainScore(m.GuildID, m.Author.ID, 0, false, 0)
			return
		}
	}

	// Kiểm tra từ có tồn tại trong từ điển dictionary.db không
	if !s.dict.IsValidWord(cleaned) {
		_ = sess.MessageReactionAdd(m.ChannelID, m.ID, "❌")
		_ = s.store.AddWordChainScore(m.GuildID, m.Author.ID, 0, false, 0)
		return
	}

	// NỐI TỪ HỢP LỆ!
	_ = sess.MessageReactionAdd(m.ChannelID, m.ID, "✅")

	s.lock.Lock()
	if ch.LastUserID != "" {
		delete(s.userViolations, m.ChannelID+"_"+ch.LastUserID)
	}
	ch.CurrentWord = cleaned
	ch.LastUserID = m.Author.ID
	ch.CurrentStreak++
	if ch.CurrentStreak > ch.HighestStreak {
		ch.HighestStreak = ch.CurrentStreak
	}
	ch.TotalWords++
	ch.UsedWords = append(ch.UsedWords, cleaned)
	s.lock.Unlock()

	// Tính điểm: 10 điểm cơ bản + thưởng streak
	scoreGain := 10 + (ch.CurrentStreak/5)*5
	_ = s.store.AddWordChainScore(m.GuildID, m.Author.ID, scoreGain, true, ch.CurrentStreak)
	_ = s.store.SaveWordChainChannel(ch)

	// Thông báo khi đạt mốc chuỗi đẹp (10, 25, 50, 100...)
	if ch.CurrentStreak == 10 || ch.CurrentStreak == 25 || ch.CurrentStreak == 50 || ch.CurrentStreak == 100 || (ch.CurrentStreak > 100 && ch.CurrentStreak%50 == 0) {
		_, _ = sess.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> đạt chuỗi %d từ (+%d điểm)",
			m.Author.ID, ch.CurrentStreak, scoreGain))
	}

	// Kiểm tra xem từ vừa đưa ra có phải "từ cụt" không (không còn từ nào trong từ điển nối tiếp được)
	nextSyllable := parts[1]
	if !s.dict.HasNextWords(nextSyllable) {
		bonusScore := 50
		_ = s.store.AddWordChainScore(m.GuildID, m.Author.ID, bonusScore, true, ch.CurrentStreak)

		congratsMsg := fmt.Sprintf("<@%s> kết thúc ván bằng từ hiểm hóc **%s** (+%d điểm)\nChuỗi đạt %d từ. Bắt đầu ván mới",
			m.Author.ID, capitalizeFirst(cleaned), bonusScore, ch.CurrentStreak)

		_, _ = sess.ChannelMessageSend(m.ChannelID, congratsMsg)

		time.Sleep(2 * time.Second)
		newCh := s.startNewGame(sess, m.ChannelID, m.GuildID, "")
		newParts := strings.Fields(newCh.CurrentWord)
		_, _ = sess.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Ván mới bắt đầu với từ **%s**\nBắt đầu bằng **%s**",
			capitalizeFirst(newCh.CurrentWord), newParts[1]))
	}
}

func (s *Service) isUserAdmin(sess *discordgo.Session, guildID, userID string) bool {
	member, err := sess.GuildMember(guildID, userID)
	if err != nil || member == nil {
		return false
	}
	guild, err := sess.Guild(guildID)
	if err != nil || guild == nil {
		return false
	}
	if guild.OwnerID == userID {
		return true
	}
	for _, roleID := range member.Roles {
		for _, r := range guild.Roles {
			if r.ID == roleID && (r.Permissions&discordgo.PermissionAdministrator != 0) {
				return true
			}
		}
	}
	return false
}

func (s *Service) sendTemporaryMessage(sess *discordgo.Session, channelID string, content string, duration time.Duration) {
	msg, err := sess.ChannelMessageSend(channelID, content)
	if err == nil && msg != nil {
		go func() {
			time.Sleep(duration)
			_ = sess.ChannelMessageDelete(channelID, msg.ID)
		}()
	}
}

func hasLink(content string) bool {
	words := strings.Fields(content)
	for _, w := range words {
		lower := strings.ToLower(w)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "discord.gg/") {
			return true
		}
	}
	return false
}
