package ticket

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"botdis/internal/discord"
	"botdis/internal/modules/chatai"
	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

const (
	BtnCreate         = "ticket_create"
	BtnClose          = "ticket_close"
	ModalCreateTicket = "modal_ticket_create"
	InputContent      = "ticket_content_input"
)

type Service struct {
	store          *storage.MySQLStore
	aiService      *chatai.Service
	lock           sync.RWMutex
	userCooldowns  map[string]time.Time
	activeTickets  map[string]string // userID -> channelID
	closingTickets map[string]bool   // channelID -> true
}

func NewService(store *storage.MySQLStore, aiService *chatai.Service) *Service {
	return &Service{
		store:          store,
		aiService:      aiService,
		userCooldowns:  make(map[string]time.Time),
		activeTickets:  make(map[string]string),
		closingTickets: make(map[string]bool),
	}
}

func (s *Service) RegisterRoutes(r *discord.Router) {
	r.RegisterComponent(BtnCreate, s.handlePromptCreateTicket)
	r.RegisterComponent(BtnClose, s.handleCloseTicket)
	r.RegisterModal(ModalCreateTicket, s.handleSubmitTicketProblem)
	r.RegisterModal("modal_ticket_create_problem", s.handleSubmitTicketProblem)
}

func (s *Service) HandleChannelDelete(sess *discordgo.Session, ch *discordgo.ChannelDelete) {
	if ch == nil || ch.Channel == nil {
		return
	}
	chID := ch.Channel.ID

	s.lock.Lock()
	for uID, cID := range s.activeTickets {
		if cID == chID {
			delete(s.activeTickets, uID)
		}
	}
	delete(s.closingTickets, chID)
	s.lock.Unlock()

	_ = s.store.CloseTicket(chID, "Kênh bị xóa trên Discord")
}

func BuildSupportPanelEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Hỗ trợ",
		Description: "Bấm nút bên dưới để tạo ticket hỗ trợ",
		Color:       0x2ecc71,
	}
}

func BuildSupportPanelActionRow() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Hỗ trợ",
					Emoji:    &discordgo.ComponentEmoji{Name: "📩"},
					Style:    discordgo.SuccessButton,
					CustomID: BtnCreate,
				},
			},
		},
	}
}

func (s *Service) handlePromptCreateTicket(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	user := i.Member.User
	guildID := i.GuildID

	s.lock.Lock()
	if lastTime, exists := s.userCooldowns[user.ID]; exists && time.Since(lastTime) < 5*time.Second {
		s.lock.Unlock()
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Vui lòng đợi vài giây trước khi tạo ticket tiếp theo",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	s.userCooldowns[user.ID] = time.Now()

	chID, found := s.activeTickets[user.ID]
	if !found {
		chID, found = s.store.GetActiveTicket(guildID, user.ID)
	}
	if found && chID != "" {
		ch, err := sess.Channel(chID)
		if err != nil || ch == nil {
			_ = s.store.CloseTicket(chID, "Kênh cũ đã bị xóa")
			delete(s.activeTickets, user.ID)
		} else {
			s.lock.Unlock()
			_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("Bạn đã có ticket đang mở <#%s>", chID),
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}
	s.lock.Unlock()

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalCreateTicket,
			Title:    "Hỗ trợ",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    InputContent,
							Label:       "Mô tả",
							Placeholder: "Nhập nội dung...",
							Style:       discordgo.TextInputParagraph,
							Required:    false,
							MinLength:   0,
							MaxLength:   2000,
						},
					},
				},
			},
		},
	})
}

func (s *Service) handleSubmitTicketProblem(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	user := i.Member.User
	guildID := i.GuildID

	data := i.ModalSubmitData()
	problemText := ""
	if len(data.Components) > 0 {
		if row, ok := data.Components[0].(*discordgo.ActionsRow); ok && len(row.Components) > 0 {
			if input, ok := row.Components[0].(*discordgo.TextInput); ok {
				problemText = input.Value
			}
		}
	}
	problemText = strings.TrimSpace(problemText)
	if problemText == "" {
		problemText = "Hỗ trợ"
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	aiTitle, channelSlug := s.aiService.GenerateTicketMeta(problemText)

	tCfg := s.store.GetTicketConfig(guildID)

	var categoryID string
	if tCfg.TicketCategoryID != "" {
		cat, err := sess.Channel(tCfg.TicketCategoryID)
		if err == nil && cat != nil && cat.Type == discordgo.ChannelTypeGuildCategory {
			categoryID = cat.ID
		}
	}

	overwrites := []*discordgo.PermissionOverwrite{
		{
			ID:   guildID,
			Type: discordgo.PermissionOverwriteTypeRole,
			Deny: discordgo.PermissionViewChannel,
		},
		{
			ID:    user.ID,
			Type:  discordgo.PermissionOverwriteTypeMember,
			Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionReadMessageHistory | discordgo.PermissionAttachFiles | discordgo.PermissionEmbedLinks,
		},
	}

	if tCfg.StaffRoleID != "" {
		overwrites = append(overwrites, &discordgo.PermissionOverwrite{
			ID:    tCfg.StaffRoleID,
			Type:  discordgo.PermissionOverwriteTypeRole,
			Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionReadMessageHistory | discordgo.PermissionAttachFiles | discordgo.PermissionEmbedLinks,
		})
	}

	randomCode := rand.Intn(9000) + 1000
	if len(channelSlug) > 26 {
		channelSlug = channelSlug[:26]
	}
	channelName := fmt.Sprintf("%s-%04d", channelSlug, randomCode)
	if len(channelName) > 32 {
		channelName = channelName[:32]
	}

	ch, err := sess.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:                 channelName,
		Type:                 discordgo.ChannelTypeGuildText,
		ParentID:             categoryID,
		PermissionOverwrites: overwrites,
	})

	if err != nil {
		_, _ = sess.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("Không thể tạo kênh ticket %v", err),
		})
		return
	}

	s.lock.Lock()
	s.activeTickets[user.ID] = ch.ID
	s.lock.Unlock()

	_ = s.store.CreateTicket(ch.ID, guildID, user.ID, user.Username, channelName, randomCode)
	s.store.LogTicketAction(guildID, ch.ID, "Tạo ticket mới", user.ID, "", problemText)

	mentionText := fmt.Sprintf("<@%s>", user.ID)
	if tCfg.StaffRoleID != "" {
		mentionText += fmt.Sprintf(" <@&%s>", tCfg.StaffRoleID)
	}

	welcomeEmbed := &discordgo.MessageEmbed{
		Title:       aiTitle,
		Description: fmt.Sprintf("Người tạo <@%s>\nMô tả %s", user.ID, problemText),
		Color:       0x2ecc71,
	}

	closeBtnRow := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Đóng",
					Style:    discordgo.DangerButton,
					CustomID: BtnClose,
				},
			},
		},
	}

	_, _ = sess.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
		Content:    mentionText,
		Embeds:     []*discordgo.MessageEmbed{welcomeEmbed},
		Components: closeBtnRow,
	})

	_, _ = sess.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("Ticket đã được tạo tại <#%s>", ch.ID),
	})
}

func (s *Service) fetchAllChannelMessages(sess *discordgo.Session, channelID string) ([]storage.TicketMessageRecord, error) {
	var allRecords []storage.TicketMessageRecord
	lastID := ""

	for {
		msgs, err := sess.ChannelMessages(channelID, 100, lastID, "", "")
		if err != nil {
			return allRecords, err
		}
		if len(msgs) == 0 {
			break
		}

		for _, m := range msgs {
			var atts []storage.TicketMessageAttachment
			for _, a := range m.Attachments {
				atts = append(atts, storage.TicketMessageAttachment{
					URL:         a.URL,
					ProxyURL:    a.ProxyURL,
					Filename:    a.Filename,
					Size:        a.Size,
					ContentType: a.ContentType,
					Width:       a.Width,
					Height:      a.Height,
				})
			}

			embedsJSON := ""
			if len(m.Embeds) > 0 {
				if b, err := json.Marshal(m.Embeds); err == nil {
					embedsJSON = string(b)
				}
			}

			authorName := "Unknown"
			authorID := ""
			if m.Author != nil {
				authorName = m.Author.Username
				authorID = m.Author.ID
			}

			allRecords = append(allRecords, storage.TicketMessageRecord{
				TicketChannelID: channelID,
				MessageID:       m.ID,
				AuthorID:        authorID,
				AuthorName:      authorName,
				Content:         m.Content,
				Attachments:     atts,
				EmbedsJSON:      embedsJSON,
				SentAt:          m.Timestamp,
			})
		}

		lastID = msgs[len(msgs)-1].ID
		if len(msgs) < 100 {
			break
		}
	}

	for i, j := 0, len(allRecords)-1; i < j; i, j = i+1, j-1 {
		allRecords[i], allRecords[j] = allRecords[j], allRecords[i]
	}

	return allRecords, nil
}

func (s *Service) handleCloseTicket(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	chID := i.ChannelID
	user := i.Member.User

	s.lock.Lock()
	if s.closingTickets == nil {
		s.closingTickets = make(map[string]bool)
	}
	if s.closingTickets[chID] || s.store.IsTicketClosed(chID) {
		s.lock.Unlock()
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Ticket này đã được đóng rồi",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	s.closingTickets[chID] = true
	s.lock.Unlock()

	// Vô hiệu hóa nút Đóng trên tin nhắn ngay lập tức (chỉ bấm được 1 lần)
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Đã đóng",
							Style:    discordgo.SecondaryButton,
							CustomID: "ticket_closed_disabled",
							Disabled: true,
						},
					},
				},
			},
		},
	})

	msgs, _ := s.fetchAllChannelMessages(sess, chID)
	_ = s.store.SaveTicketMessages(chID, msgs)

	ownerID, found := s.store.FindTicketByChannel(chID)

	closeReason := "Đóng bởi " + user.Username
	_ = s.store.CloseTicketWithSchedule(chID, closeReason, 3)
	s.store.LogTicketAction(i.GuildID, chID, "Đóng ticket", user.ID, "", fmt.Sprintf("Lưu %d tin nhắn, xóa sau 3 ngày", len(msgs)))

	s.lock.Lock()
	for uID, cID := range s.activeTickets {
		if cID == chID {
			delete(s.activeTickets, uID)
		}
	}
	s.lock.Unlock()

	if found && ownerID != "" {
		_ = sess.ChannelPermissionSet(chID, ownerID, discordgo.PermissionOverwriteTypeMember,
			0,
			discordgo.PermissionViewChannel|discordgo.PermissionSendMessages|discordgo.PermissionReadMessageHistory|discordgo.PermissionAttachFiles|discordgo.PermissionEmbedLinks,
		)
	}

	tCfg := s.store.GetTicketConfig(i.GuildID)

	ch, err := sess.Channel(chID)
	if err == nil && ch != nil {
		for _, overwrite := range ch.PermissionOverwrites {
			if overwrite.Type == discordgo.PermissionOverwriteTypeMember && overwrite.ID != sess.State.User.ID {
				if overwrite.ID == ownerID || (tCfg != nil && overwrite.ID != tCfg.StaffRoleID) {
					_ = sess.ChannelPermissionSet(chID, overwrite.ID, discordgo.PermissionOverwriteTypeMember,
						0,
						discordgo.PermissionViewChannel|discordgo.PermissionSendMessages|discordgo.PermissionReadMessageHistory|discordgo.PermissionAttachFiles|discordgo.PermissionEmbedLinks,
					)
				}
			}
		}

		newName := ch.Name
		if !strings.HasPrefix(newName, "dong-") && !strings.HasPrefix(newName, "closed-") {
			newName = "dong-" + newName
			if len(newName) > 32 {
				newName = newName[:32]
			}
		}

		editData := &discordgo.ChannelEdit{Name: newName}
		if tCfg != nil && tCfg.ArchiveCategoryID != "" {
			editData.ParentID = tCfg.ArchiveCategoryID
		}
		_, _ = sess.ChannelEdit(chID, editData)
	}

	closeEmbed := &discordgo.MessageEmbed{
		Title: "Ticket đã đóng",
		Color: 0xe74c3c,
	}

	_, _ = sess.ChannelMessageSendEmbed(chID, closeEmbed)
}

func (s *Service) StartAutoDeleteWorker(sess *discordgo.Session) {
	go s.cleanupExpiredTickets(sess)

	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		for range ticker.C {
			s.cleanupExpiredTickets(sess)
		}
	}()
}

func (s *Service) cleanupExpiredTickets(sess *discordgo.Session) {
	expired, err := s.store.GetExpiredTickets()
	if err != nil || len(expired) == 0 {
		return
	}

	for _, rec := range expired {
		_, delErr := sess.ChannelDelete(rec.ChannelID)
		if delErr != nil {
			log.Printf("Xóa kênh ticket hết hạn %s: %v", rec.ChannelID, delErr)
		}

		_ = s.store.MarkTicketDeleted(rec.ChannelID)
		s.store.LogTicketAction(rec.GuildID, rec.ChannelID, "Tự động xóa kênh", "SYSTEM", "", "Hết hạn lưu trữ 3 ngày")

		s.lock.Lock()
		for uID, cID := range s.activeTickets {
			if cID == rec.ChannelID {
				delete(s.activeTickets, uID)
			}
		}
		delete(s.closingTickets, rec.ChannelID)
		s.lock.Unlock()
	}
}
