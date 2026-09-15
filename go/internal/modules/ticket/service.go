package ticket

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"botdis/internal/discord"
	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

const (
	BtnCreate     = "ticket_create"
	BtnClose      = "ticket_close"
	BtnClaim      = "ticket_claim"
	BtnTranscript = "ticket_transcript"
	BtnAddUser    = "ticket_add_user"
	BtnRemoveUser = "ticket_remove_user"
	BtnRename     = "ticket_rename"
	BtnDelete     = "ticket_delete"

	ModalRename     = "modal_ticket_rename"
	ModalAddUser    = "modal_ticket_add_user"
	ModalRemoveUser = "modal_ticket_remove_user"
)

type Service struct {
	store         *storage.MySQLStore
	lock          sync.RWMutex
	userCooldowns map[string]time.Time
	activeTickets map[string]string
}

func NewService(store *storage.MySQLStore) *Service {
	return &Service{
		store:         store,
		userCooldowns: make(map[string]time.Time),
		activeTickets: make(map[string]string),
	}
}

func (s *Service) RegisterRoutes(r *discord.Router) {
	r.RegisterCommand("ticket", s.handleTicketCommand)

	r.RegisterComponent(BtnCreate, s.handleCreateTicket)
	r.RegisterComponent(BtnClose, s.handleCloseTicket)
	r.RegisterComponent(BtnClaim, s.handleClaimTicket)
	r.RegisterComponent(BtnDelete, s.handleDeleteTicket)
	r.RegisterComponent(BtnRename, s.handlePromptRename)
	r.RegisterComponent(BtnAddUser, s.handlePromptAddUser)
	r.RegisterComponent(BtnRemoveUser, s.handlePromptRemoveUser)
	r.RegisterComponent(BtnTranscript, s.handleTranscript)

	r.RegisterModal(ModalRename, s.handleSubmitRename)
	r.RegisterModal(ModalAddUser, s.handleSubmitAddUser)
	r.RegisterModal(ModalRemoveUser, s.handleSubmitRemoveUser)
}

func BuildSupportPanelEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Support",
		Description: "• Chỉ tạo Hỗ Trợ nếu bạn có vấn đề về nạp tiền, bảo hành và lỗi ở trên website.\n" +
			"• Vui lòng không tạo Phiếu Hỗ Trợ cho vui.\n" +
			"• Tạo Phiếu Hỗ Trợ không nhắn gì sẽ Mute 24h.\n" +
			"• Xin cảm ơn.",
		Color: 0x2ecc71,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "TicketTool.xyz - Ticketing without clutter",
		},
	}
}

func BuildSupportPanelActionRow() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Hỗ Trợ",
					Style:    discordgo.DangerButton,
					CustomID: BtnCreate,
					Emoji: &discordgo.ComponentEmoji{
						Name: "📩",
					},
				},
			},
		},
	}
}

func BuildTicketActionRows() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Đóng ticket", Style: discordgo.DangerButton, CustomID: BtnClose, Emoji: &discordgo.ComponentEmoji{Name: "🔒"}},
				discordgo.Button{Label: "Claim ticket", Style: discordgo.SecondaryButton, CustomID: BtnClaim, Emoji: &discordgo.ComponentEmoji{Name: "🟡"}},
				discordgo.Button{Label: "Transcript", Style: discordgo.PrimaryButton, CustomID: BtnTranscript, Emoji: &discordgo.ComponentEmoji{Name: "📄"}},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Thêm người", Style: discordgo.SuccessButton, CustomID: BtnAddUser, Emoji: &discordgo.ComponentEmoji{Name: "➕"}},
				discordgo.Button{Label: "Xóa người", Style: discordgo.SecondaryButton, CustomID: BtnRemoveUser, Emoji: &discordgo.ComponentEmoji{Name: "➖"}},
				discordgo.Button{Label: "Đổi tên", Style: discordgo.SecondaryButton, CustomID: BtnRename, Emoji: &discordgo.ComponentEmoji{Name: "✏️"}},
				discordgo.Button{Label: "Xóa ticket", Style: discordgo.DangerButton, CustomID: BtnDelete, Emoji: &discordgo.ComponentEmoji{Name: "🗑️"}},
			},
		},
	}
}

func (s *Service) handleTicketCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}

	subCmd := data.Options[0]
	switch subCmd.Name {
	case "send":
		channelID := i.ChannelID
		if len(subCmd.Options) > 0 {
			channelID = subCmd.Options[0].ChannelValue(sess).ID
		}

		embed := BuildSupportPanelEmbed()
		components := BuildSupportPanelActionRow()

		_, err := sess.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		})

		msg := fmt.Sprintf("✅ Đã gửi bảng Hỗ Trợ vào kênh <#%s>", channelID)
		if err != nil {
			msg = fmt.Sprintf("❌ Không thể gửi bảng Hỗ Trợ: %v", err)
		}

		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: msg,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}

func (s *Service) handleCreateTicket(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	user := i.Member.User
	guildID := i.GuildID

	s.lock.Lock()
	if lastTime, exists := s.userCooldowns[user.ID]; exists && time.Since(lastTime) < 5*time.Second {
		s.lock.Unlock()
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Vui lòng đợi vài giây trước khi tạo ticket tiếp theo.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	s.userCooldowns[user.ID] = time.Now()

	if chID, found := s.store.GetActiveTicket(guildID, user.ID); found {
		s.lock.Unlock()
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Bạn đã có ticket đang mở: <#%s>", chID),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	s.lock.Unlock()

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	channels, _ := sess.GuildChannels(guildID)
	var categoryID string
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildCategory && (ch.Name == "🎟️ HỖ TRỢ" || ch.Name == "HỖ TRỢ" || ch.Name == "Tickets") {
			categoryID = ch.ID
			break
		}
	}

	if categoryID == "" {
		cat, err := sess.GuildChannelCreate(guildID, "🎟️ HỖ TRỢ", discordgo.ChannelTypeGuildCategory)
		if err == nil && cat != nil {
			categoryID = cat.ID
		}
	}

	ticketName := fmt.Sprintf("ho-tro-%04d", time.Now().Nanosecond()%10000)
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

	ch, err := sess.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:                 ticketName,
		Type:                 discordgo.ChannelTypeGuildText,
		ParentID:             categoryID,
		PermissionOverwrites: overwrites,
	})

	if err != nil {
		_, _ = sess.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("Không thể tạo kênh ticket: %v", err),
		})
		return
	}

	s.lock.Lock()
	s.activeTickets[user.ID] = ch.ID
	s.lock.Unlock()

	_ = s.store.CreateTicket(ch.ID, guildID, user.ID, user.Username, ticketName, time.Now().Nanosecond()%10000)
	s.store.LogTicketAction(guildID, ch.ID, "Tạo ticket mới", user.ID, "", "")

	welcomeEmbed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("📩 Phiếu Hỗ Trợ #%s", ch.Name),
		Description: fmt.Sprintf("Xin chào <@%s>!\nVui lòng trình bày chi tiết vấn đề của bạn kèm hình ảnh hoặc mã giao dịch.\nĐội ngũ hỗ trợ sẽ phản hồi trong giây lát.", user.ID),
		Color:       0x2ecc71,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "👤 Người tạo", Value: fmt.Sprintf("<@%s>", user.ID), Inline: true},
			{Name: "📌 Trạng thái", Value: "Đang chờ hỗ trợ", Inline: true},
		},
	}

	_, _ = sess.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
		Content:    fmt.Sprintf("<@%s>", user.ID),
		Embeds:     []*discordgo.MessageEmbed{welcomeEmbed},
		Components: BuildTicketActionRows(),
	})

	_, _ = sess.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("✅ Ticket của bạn đã được tạo: <#%s>", ch.ID),
	})
}

func (s *Service) handleCloseTicket(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	ch, err := sess.Channel(i.ChannelID)
	if err != nil {
		return
	}

	_, _ = sess.ChannelEdit(ch.ID, &discordgo.ChannelEdit{
		Name: fmt.Sprintf("closed-%s", ch.Name),
	})

	_ = s.store.CloseTicket(ch.ID, "Đóng bởi "+i.Member.User.Username)
	s.store.LogTicketAction(i.GuildID, ch.ID, "Đóng ticket", i.Member.User.ID, "", "")

	closeEmbed := &discordgo.MessageEmbed{
		Title:       "🔒 Ticket đã đóng",
		Description: fmt.Sprintf("Ticket đã được đóng bởi <@%s>.", i.Member.User.ID),
		Color:       0xe74c3c,
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{closeEmbed},
		},
	})
}

func (s *Service) handleClaimTicket(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🟡 Ticket này đã được nhận xử lý bởi <@%s>.", i.Member.User.ID),
		},
	})
}

func (s *Service) handleDeleteTicket(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🗑️ Kênh ticket sẽ được xóa sau 3 giây...",
		},
	})

	go func() {
		time.Sleep(3 * time.Second)
		_, _ = sess.ChannelDelete(i.ChannelID)
	}()
}

func (s *Service) handlePromptRename(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalRename,
			Title:    "Đổi tên ticket",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "new_name",
							Label:     "Tên mới",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 80,
						},
					},
				},
			},
		},
	})
}

func (s *Service) handleSubmitRename(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	newName := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	_, _ = sess.ChannelEdit(i.ChannelID, &discordgo.ChannelEdit{Name: newName})

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Đã đổi tên kênh thành: `%s`", newName),
		},
	})
}

func (s *Service) handlePromptAddUser(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalAddUser,
			Title:    "Thêm người vào ticket",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "user_id",
							Label:       "ID người dùng",
							Placeholder: "Ví dụ: 123456789012345678",
							Style:       discordgo.TextInputShort,
							Required:    true,
						},
					},
				},
			},
		},
	})
}

func (s *Service) handleSubmitAddUser(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	targetID := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	err := sess.ChannelPermissionSet(i.ChannelID, targetID, discordgo.PermissionOverwriteTypeMember, discordgo.PermissionViewChannel|discordgo.PermissionSendMessages|discordgo.PermissionReadMessageHistory, 0)
	msg := fmt.Sprintf("✅ Đã thêm <@%s> vào ticket.", targetID)
	if err != nil {
		msg = fmt.Sprintf("❌ Không thể thêm người dùng: %v", err)
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	})
}

func (s *Service) handlePromptRemoveUser(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalRemoveUser,
			Title:    "Xóa người khỏi ticket",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "user_id",
							Label:       "ID người dùng",
							Placeholder: "Ví dụ: 123456789012345678",
							Style:       discordgo.TextInputShort,
							Required:    true,
						},
					},
				},
			},
		},
	})
}

func (s *Service) handleSubmitRemoveUser(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	targetID := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	err := sess.ChannelPermissionDelete(i.ChannelID, targetID)
	msg := fmt.Sprintf("✅ Đã xóa <@%s> khỏi ticket.", targetID)
	if err != nil {
		msg = fmt.Sprintf("❌ Không thể xóa người dùng: %v", err)
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	})
}

func (s *Service) handleTranscript(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	messages, err := sess.ChannelMessages(i.ChannelID, 100, "", "", "")
	if err != nil {
		_, _ = sess.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: "Lỗi tải tin nhắn."})
		return
	}

	transcriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("transcript_%s.html", i.ChannelID))
	f, err := os.Create(transcriptPath)
	if err != nil {
		return
	}
	defer os.Remove(transcriptPath)

	fmt.Fprintf(f, "<!DOCTYPE html><html><head><meta charset='utf-8'><title>Transcript Ticket</title></head><body style='background:#2f3136;color:#fff;font-family:sans-serif;'><h2>Transcript Ticket</h2><hr>")
	for idx := len(messages) - 1; idx >= 0; idx-- {
		m := messages[idx]
		author := "Unknown"
		if m.Author != nil {
			author = m.Author.Username
		}
		fmt.Fprintf(f, "<div style='margin-bottom:10px;'><b>%s</b> <small style='color:#bbb;'>%s</small><br>%s</div>", author, m.Timestamp.Format("2006-01-02 15:04:05"), m.Content)
	}
	fmt.Fprintf(f, "</body></html>")
	f.Close()

	rf, err := os.Open(transcriptPath)
	if err != nil {
		return
	}
	defer rf.Close()

	_, _ = sess.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: "📄 File Transcript của ticket:",
		Files: []*discordgo.File{
			{
				Name:   fmt.Sprintf("transcript_%s.html", i.ChannelID),
				Reader: rf,
			},
		},
	})
}
