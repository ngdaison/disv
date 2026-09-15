package dashboard

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"botdis/internal/discord"
	"botdis/internal/modules/feed"
	"botdis/internal/modules/ticket"
	"botdis/internal/modules/utility"
	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

const (
	BtnViewTicket               = "setting_view_ticket"
	SelectTicketRole            = "setting_select_ticket_role"
	SelectTicketCategory        = "setting_select_ticket_category"
	SelectTicketArchiveCategory = "setting_select_ticket_archive_category"
	BtnTicketSendPanel          = "setting_ticket_send_panel"

	BtnViewAutoRole    = "setting_view_autorole"
	SelectAutoRole     = "setting_select_autorole"
	BtnAutoRoleDelete  = "setting_autorole_delete"
	BtnAutoRoleDelay   = "setting_autorole_delay"
	ModalAutoRoleDelay = "modal_autorole_delay"

	BtnViewFeed         = "setting_view_feed"
	BtnAddFeedYoutube   = "setting_add_feed_youtube"
	BtnAddFeedTiktok    = "setting_add_feed_tiktok"
	SelectFeedRole      = "setting_select_feed_role"
	BtnFeedNoPing       = "setting_feed_no_ping"
	SelectFeedChannel   = "setting_select_feed_channel"
	SelectDeleteFeed    = "setting_select_delete_feed"
	ModalAddFeedYoutube = "modal_add_feed_youtube"
	ModalAddFeedTiktok  = "modal_add_feed_tiktok"

	BtnViewBotInfo = "setting_view_botinfo"
	BtnViewPing    = "setting_view_ping"
	BtnBackToMain  = "setting_back"
)

type feedSessionState struct {
	targetChannelID string
	pingRoleID      string
}

var userFeedState sync.Map

type Service struct {
	store *storage.MySQLStore
}

func NewService(store *storage.MySQLStore) *Service {
	return &Service{store: store}
}

func (s *Service) RegisterRoutes(r *discord.Router) {
	r.RegisterCommand("setting", s.handleSettingCommand)

	toggleKeys := []string{
		"ac_link", "ac_media", "ac_file", "ac_text",
		"as_fast", "as_dup",
		"level_enabled", "tiktok_enabled", "ai_enabled",
	}

	for _, key := range toggleKeys {
		customID := "toggle_" + key
		k := key
		r.RegisterComponent(customID, func(sess *discordgo.Session, i *discordgo.InteractionCreate) {
			s.handleToggle(sess, i, k)
		})
	}

	r.RegisterComponent(BtnViewTicket, s.handleViewTicket)
	r.RegisterComponent(SelectTicketRole, s.handleSelectTicketRole)
	r.RegisterComponent(SelectTicketCategory, s.handleSelectTicketCategory)
	r.RegisterComponent(SelectTicketArchiveCategory, s.handleSelectTicketArchiveCategory)
	r.RegisterComponent(BtnTicketSendPanel, s.handleTicketSendPanel)

	r.RegisterComponent(BtnViewAutoRole, s.handleViewAutoRole)
	r.RegisterComponent(SelectAutoRole, s.handleSelectAutoRole)
	r.RegisterComponent(BtnAutoRoleDelete, s.handleAutoRoleDelete)
	r.RegisterComponent(BtnAutoRoleDelay, s.handleAutoRoleDelayPrompt)
	r.RegisterModal(ModalAutoRoleDelay, s.handleAutoRoleDelaySubmit)

	r.RegisterComponent(BtnViewFeed, s.handleViewFeed)
	r.RegisterComponent(BtnAddFeedYoutube, s.handleAddFeedYoutubePrompt)
	r.RegisterComponent(BtnAddFeedTiktok, s.handleAddFeedTiktokPrompt)
	r.RegisterComponent(SelectFeedRole, s.handleSelectFeedRole)
	r.RegisterComponent(BtnFeedNoPing, s.handleFeedNoPing)
	r.RegisterComponent(SelectFeedChannel, s.handleSelectFeedChannel)
	r.RegisterComponent(SelectDeleteFeed, s.handleSelectDeleteFeed)
	r.RegisterModal(ModalAddFeedYoutube, s.handleAddFeedYoutubeSubmit)
	r.RegisterModal(ModalAddFeedTiktok, s.handleAddFeedTiktokSubmit)

	r.RegisterComponent(BtnViewBotInfo, s.handleViewBotInfo)
	r.RegisterComponent(BtnViewPing, s.handleViewPing)
	r.RegisterComponent(BtnBackToMain, s.handleBackToMain)
}

func (s *Service) handleSettingCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !s.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Bạn cần quyền Administrator để mở bảng cài đặt",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := s.buildDashboardEmbed(i.GuildID, i.ChannelID)
	components := s.buildDashboardComponents(i.GuildID, i.ChannelID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

func (s *Service) handleToggle(sess *discordgo.Session, i *discordgo.InteractionCreate, key string) {
	guildID := i.GuildID
	channelID := i.ChannelID

	if key == "as_fast" || key == "as_dup" {
		gData := s.store.GetGuildData(guildID)
		newFast := gData.ASFast
		newDup := gData.ASDup
		if key == "as_fast" {
			newFast = !newFast
		} else {
			newDup = !newDup
		}
		s.store.SetGuildAntiSpam(guildID, newFast, newDup)
	} else {
		s.store.UpdateChannelSetting(guildID, channelID, func(cs *storage.ChannelSettings) {
			switch key {
			case "ac_link":
				cs.ACLink = !cs.ACLink
			case "ac_media":
				cs.ACMedia = !cs.ACMedia
			case "ac_file":
				cs.ACFile = !cs.ACFile
			case "ac_text":
				cs.ACText = !cs.ACText
			case "level_enabled":
				cs.LevelEnabled = !cs.LevelEnabled
			case "tiktok_enabled":
				cs.TikTokEnabled = !cs.TikTokEnabled
			case "ai_enabled":
				cs.AIEnabled = !cs.AIEnabled
			}
		})
	}

	embed := s.buildDashboardEmbed(guildID, channelID)
	components := s.buildDashboardComponents(guildID, channelID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleViewTicket(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	embed, components := s.buildTicketConfigView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleSelectTicketRole(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) > 0 {
		roleID := data.Values[0]
		_ = s.store.SetTicketStaffRole(i.GuildID, roleID)
	}

	embed, components := s.buildTicketConfigView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleSelectTicketCategory(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) > 0 {
		catID := data.Values[0]
		_ = s.store.SetTicketCategory(i.GuildID, catID)
	}

	embed, components := s.buildTicketConfigView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleSelectTicketArchiveCategory(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) > 0 {
		catID := data.Values[0]
		_ = s.store.SetTicketArchiveCategory(i.GuildID, catID)
	}

	embed, components := s.buildTicketConfigView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleTicketSendPanel(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := ticket.BuildSupportPanelEmbed()
	components := ticket.BuildSupportPanelActionRow()

	_, err := sess.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})

	msg := "Đã gửi bảng hỗ trợ vào kênh này"
	if err != nil {
		msg = "Không thể gửi bảng hỗ trợ"
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func (s *Service) buildTicketConfigView(guildID string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	tCfg := s.store.GetTicketConfig(guildID)

	staffText := "Chưa thiết lập"
	if tCfg.StaffRoleID != "" {
		staffText = fmt.Sprintf("<@&%s>", tCfg.StaffRoleID)
	}

	catText := "Mặc định"
	if tCfg.TicketCategoryID != "" {
		catText = fmt.Sprintf("<#%s>", tCfg.TicketCategoryID)
	}

	archiveCatText := "Chưa thiết lập"
	if tCfg.ArchiveCategoryID != "" {
		archiveCatText = fmt.Sprintf("<#%s>", tCfg.ArchiveCategoryID)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Cấu hình ticket",
		Description: fmt.Sprintf("Role hỗ trợ hiện tại **%s**\nDanh mục hoạt động **%s**\nDanh mục lưu trữ **%s**\nChọn role hỗ trợ và danh mục bên dưới", staffText, catText, archiveCatText),
		Color:       0x3498db,
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:    discordgo.RoleSelectMenu,
					CustomID:    SelectTicketRole,
					Placeholder: "Chọn role hỗ trợ...",
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:     discordgo.ChannelSelectMenu,
					CustomID:     SelectTicketCategory,
					Placeholder:  "Chọn danh mục ticket...",
					ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildCategory},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:     discordgo.ChannelSelectMenu,
					CustomID:     SelectTicketArchiveCategory,
					Placeholder:  "Chọn danh mục lưu trữ...",
					ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildCategory},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Gửi bảng",
					CustomID: BtnTicketSendPanel,
					Style:    discordgo.SuccessButton,
				},
				discordgo.Button{
					Label:    "Quay lại",
					CustomID: BtnBackToMain,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
	}

	return embed, components
}

func (s *Service) handleViewAutoRole(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	embed, components := s.buildAutoRoleView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleSelectAutoRole(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) > 0 {
		roleID := data.Values[0]
		s.store.SetAutoRole(i.GuildID, roleID)
	}

	embed, components := s.buildAutoRoleView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleAutoRoleDelete(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = s.store.ClearAutoRole(i.GuildID)
	embed, components := s.buildAutoRoleView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleAutoRoleDelayPrompt(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	gData := s.store.GetGuildData(i.GuildID)
	val := fmt.Sprintf("%d", gData.AutoJoinDelay)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalAutoRoleDelay,
			Title:    "Thời gian cấp role",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "delay_seconds",
							Label:       "Thời gian chờ cấp role (giây)",
							Placeholder: "Ví dụ 0, 10, 30, 60... (0 là ngay lập tức)",
							Value:       val,
							Style:       discordgo.TextInputShort,
							Required:    true,
							MaxLength:   6,
						},
					},
				},
			},
		},
	})
}

func (s *Service) handleAutoRoleDelaySubmit(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	secVal := 0
	if len(data.Components) > 0 {
		if row, ok := data.Components[0].(*discordgo.ActionsRow); ok && len(row.Components) > 0 {
			if input, ok := row.Components[0].(*discordgo.TextInput); ok {
				var parsed int
				if _, err := fmt.Sscanf(input.Value, "%d", &parsed); err == nil && parsed >= 0 {
					secVal = parsed
				}
			}
		}
	}

	_ = s.store.SetAutoRoleDelay(i.GuildID, secVal)
	embed, components := s.buildAutoRoleView(i.GuildID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) buildAutoRoleView(guildID string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	gData := s.store.GetGuildData(guildID)
	roleID := gData.AutoJoinRoles["default"]
	currentText := "Chưa thiết lập"
	if roleID != "" {
		currentText = fmt.Sprintf("<@&%s>", roleID)
	}

	delayText := "Ngay lập tức"
	if gData.AutoJoinDelay > 0 {
		delayText = fmt.Sprintf("%d giây", gData.AutoJoinDelay)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Cấu hình autorole",
		Description: fmt.Sprintf("Role tự động hiện tại **%s**\nThời gian cấp **%s**\nChọn role bên dưới để tự động gán cho thành viên mới", currentText, delayText),
		Color:       0x3498db,
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:    discordgo.RoleSelectMenu,
					CustomID:    SelectAutoRole,
					Placeholder: "Chọn role tự động...",
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Xóa",
					CustomID: BtnAutoRoleDelete,
					Style:    discordgo.DangerButton,
				},
				discordgo.Button{
					Label:    "Cài đặt",
					CustomID: BtnAutoRoleDelay,
					Style:    discordgo.SecondaryButton,
				},
				discordgo.Button{
					Label:    "Quay lại",
					CustomID: BtnBackToMain,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
	}
	return embed, components
}

func (s *Service) handleViewBotInfo(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := utility.BuildBotInfoEmbed()
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Quay lại",
					CustomID: BtnBackToMain,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleViewPing(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	latency := sess.HeartbeatLatency().Milliseconds()
	embed := &discordgo.MessageEmbed{
		Title:       "Độ trễ gateway",
		Description: fmt.Sprintf("Độ trễ gateway hiện tại **%dms**", latency),
		Color:       0x2ecc71,
	}
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Kiểm tra lại",
					CustomID: BtnViewPing,
					Style:    discordgo.PrimaryButton,
				},
				discordgo.Button{
					Label:    "Quay lại",
					CustomID: BtnBackToMain,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleBackToMain(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := s.buildDashboardEmbed(i.GuildID, i.ChannelID)
	components := s.buildDashboardComponents(i.GuildID, i.ChannelID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) buildDashboardEmbed(guildID, channelID string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Bảng điều khiển bot",
		Description: "Bấm vào các nút bên dưới để bật tắt hoặc quản lý tính năng",
		Color:       0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "Chống spam",
				Value: "**Chặn link** Xóa tin nhắn chứa link\n" +
					"**Chặn media** Xóa ảnh và video\n" +
					"**Chặn file** Xóa các file đính kèm\n" +
					"**Chặn chat** Chỉ xóa tin nhắn text thuần\n" +
					"**Anti fast** Xóa toàn bộ tin nhắn khi gửi quá nhanh qua các kênh\n" +
					"**Anti dup** Xóa toàn bộ tin nhắn khi gửi trùng lặp qua các kênh",
			},
			{
				Name: "Tiện ích",
				Value: "**TikTok** Tự động tải video không logo và nén theo server\n" +
					"**Leveling** Hệ thống cộng điểm kinh nghiệm và cấp độ\n" +
					"**AI chat** Trò chuyện tự động với AI\n" +
					"**AutoRole** Gán role tự động cho thành viên mới\n" +
					"**Ticket** Hệ thống hỗ trợ thành viên\n" +
					"**Video mới** Tự động thông báo khi có video mới từ YouTube và TikTok",
			},
		},
	}
}

func (s *Service) buildDashboardComponents(guildID, channelID string) []discordgo.MessageComponent {
	settings := s.store.GetChannelSettings(guildID, channelID)

	btnStyle := func(enabled bool) discordgo.ButtonStyle {
		if enabled {
			return discordgo.SuccessButton
		}
		return discordgo.DangerButton
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Cấm link", CustomID: "toggle_ac_link", Style: btnStyle(settings.ACLink)},
				discordgo.Button{Label: "Cấm media", CustomID: "toggle_ac_media", Style: btnStyle(settings.ACMedia)},
				discordgo.Button{Label: "Cấm file", CustomID: "toggle_ac_file", Style: btnStyle(settings.ACFile)},
				discordgo.Button{Label: "Cấm chat", CustomID: "toggle_ac_text", Style: btnStyle(settings.ACText)},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Anti fast", CustomID: "toggle_as_fast", Style: btnStyle(settings.ASFast)},
				discordgo.Button{Label: "Anti dup", CustomID: "toggle_as_dup", Style: btnStyle(settings.ASDup)},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Leveling", CustomID: "toggle_level_enabled", Style: btnStyle(settings.LevelEnabled)},
				discordgo.Button{Label: "TikTok", CustomID: "toggle_tiktok_enabled", Style: btnStyle(settings.TikTokEnabled)},
				discordgo.Button{Label: "AI chat", CustomID: "toggle_ai_enabled", Style: btnStyle(settings.AIEnabled)},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Ticket",
					CustomID: BtnViewTicket,
					Style:    discordgo.SecondaryButton,
				},
				discordgo.Button{
					Label:    "AutoRole",
					CustomID: BtnViewAutoRole,
					Style:    discordgo.SecondaryButton,
				},
				discordgo.Button{
					Label:    "Video mới",
					CustomID: BtnViewFeed,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Thông tin bot",
					CustomID: BtnViewBotInfo,
					Style:    discordgo.SecondaryButton,
				},
				discordgo.Button{
					Label:    "Độ trễ",
					CustomID: BtnViewPing,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
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

func getFeedSessionState(guildID, userID, defaultChannelID string) *feedSessionState {
	key := guildID + "_" + userID
	val, ok := userFeedState.Load(key)
	if !ok {
		state := &feedSessionState{
			targetChannelID: defaultChannelID,
			pingRoleID:      "",
		}
		userFeedState.Store(key, state)
		return state
	}
	return val.(*feedSessionState)
}

func (s *Service) handleViewFeed(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	embed, components := s.buildFeedConfigView(i.GuildID, i.ChannelID, i.Member.User.ID)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleSelectFeedRole(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) > 0 {
		state := getFeedSessionState(i.GuildID, i.Member.User.ID, i.ChannelID)
		state.pingRoleID = data.Values[0]
	}

	embed, components := s.buildFeedConfigView(i.GuildID, i.ChannelID, i.Member.User.ID)
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleFeedNoPing(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	state := getFeedSessionState(i.GuildID, i.Member.User.ID, i.ChannelID)
	state.pingRoleID = ""

	embed, components := s.buildFeedConfigView(i.GuildID, i.ChannelID, i.Member.User.ID)
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleSelectFeedChannel(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) > 0 {
		state := getFeedSessionState(i.GuildID, i.Member.User.ID, i.ChannelID)
		state.targetChannelID = data.Values[0]
	}

	embed, components := s.buildFeedConfigView(i.GuildID, i.ChannelID, i.Member.User.ID)
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleAddFeedYoutubePrompt(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalAddFeedYoutube,
			Title:    "Thêm kênh YouTube",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "channel_url",
							Label:       "Đường dẫn kênh YouTube",
							Placeholder: "https://www.youtube.com/@tên_kênh hoặc ID kênh",
							Style:       discordgo.TextInputShort,
							Required:    true,
							MaxLength:   255,
						},
					},
				},
			},
		},
	})
}

func (s *Service) handleAddFeedTiktokPrompt(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalAddFeedTiktok,
			Title:    "Thêm kênh TikTok",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "channel_url",
							Label:       "Đường dẫn kênh TikTok",
							Placeholder: "https://www.tiktok.com/@username hoặc @username",
							Style:       discordgo.TextInputShort,
							Required:    true,
							MaxLength:   255,
						},
					},
				},
			},
		},
	})
}

func (s *Service) handleAddFeedYoutubeSubmit(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	var urlVal string
	if len(data.Components) > 0 {
		if row, ok := data.Components[0].(*discordgo.ActionsRow); ok && len(row.Components) > 0 {
			if input, ok := row.Components[0].(*discordgo.TextInput); ok {
				urlVal = strings.TrimSpace(input.Value)
			}
		}
	}

	state := getFeedSessionState(i.GuildID, i.Member.User.ID, i.ChannelID)
	targetCh := state.targetChannelID
	if targetCh == "" {
		targetCh = i.ChannelID
	}

	chID, chTitle, latestVID, _, err := feed.ResolveYouTubeChannel(urlVal)
	if err != nil {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Không thể thêm kênh: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	sub := &storage.VideoSubscription{
		GuildID:     i.GuildID,
		ChannelID:   targetCh,
		Platform:    "youtube",
		TargetID:    chID,
		TargetURL:   urlVal,
		TargetName:  chTitle,
		PingRoleID:  state.pingRoleID,
		LastVideoID: latestVID,
	}

	_ = s.store.AddVideoSubscription(sub)

	embed, components := s.buildFeedConfigView(i.GuildID, i.ChannelID, i.Member.User.ID)
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleAddFeedTiktokSubmit(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	var urlVal string
	if len(data.Components) > 0 {
		if row, ok := data.Components[0].(*discordgo.ActionsRow); ok && len(row.Components) > 0 {
			if input, ok := row.Components[0].(*discordgo.TextInput); ok {
				urlVal = strings.TrimSpace(input.Value)
			}
		}
	}

	state := getFeedSessionState(i.GuildID, i.Member.User.ID, i.ChannelID)
	targetCh := state.targetChannelID
	if targetCh == "" {
		targetCh = i.ChannelID
	}

	username, displayName, latestVID, _, err := feed.ResolveTikTokChannel(urlVal)
	if err != nil {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Không thể thêm kênh: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	sub := &storage.VideoSubscription{
		GuildID:     i.GuildID,
		ChannelID:   targetCh,
		Platform:    "tiktok",
		TargetID:    username,
		TargetURL:   urlVal,
		TargetName:  displayName,
		PingRoleID:  state.pingRoleID,
		LastVideoID: latestVID,
	}

	_ = s.store.AddVideoSubscription(sub)

	embed, components := s.buildFeedConfigView(i.GuildID, i.ChannelID, i.Member.User.ID)
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) handleSelectDeleteFeed(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) > 0 {
		id, _ := strconv.Atoi(data.Values[0])
		_ = s.store.DeleteVideoSubscription(id)
	}

	embed, components := s.buildFeedConfigView(i.GuildID, i.ChannelID, i.Member.User.ID)
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (s *Service) buildFeedConfigView(guildID, currentChannelID, userID string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	state := getFeedSessionState(guildID, userID, currentChannelID)
	subs, _ := s.store.GetVideoSubscriptionsByGuild(guildID)

	pingText := "Không ping"
	if state.pingRoleID != "" {
		pingText = fmt.Sprintf("<@&%s>", state.pingRoleID)
	}

	targetCh := state.targetChannelID
	if targetCh == "" {
		targetCh = currentChannelID
	}
	channelText := fmt.Sprintf("<#%s>", targetCh)

	var descLines []string
	descLines = append(descLines, fmt.Sprintf("Role ping chuẩn bị thêm **%s**", pingText))
	descLines = append(descLines, fmt.Sprintf("Kênh nhận thông báo **%s**", channelText))
	descLines = append(descLines, "")
	descLines = append(descLines, "**Danh sách kênh đang theo dõi**")

	if len(subs) == 0 {
		descLines = append(descLines, "Chưa có kênh nào được đăng ký theo dõi trong server")
	} else {
		for i, sub := range subs {
			pText := "Không ping"
			if sub.PingRoleID != "" {
				pText = fmt.Sprintf("<@&%s>", sub.PingRoleID)
			}
			plat := "YouTube"
			if sub.Platform == "tiktok" {
				plat = "TikTok"
			}
			descLines = append(descLines, fmt.Sprintf("%d. **[%s] %s**\nKênh nhận <#%s> | Ping %s", i+1, plat, sub.TargetName, sub.ChannelID, pText))
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Cấu hình thông báo video mới",
		Description: strings.Join(descLines, "\n"),
		Color:       0x3498db,
	}

	var components []discordgo.MessageComponent

	// Row 1: Thêm YouTube, Thêm TikTok, Không ping
	components = append(components, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Thêm YouTube",
				CustomID: BtnAddFeedYoutube,
				Style:    discordgo.SuccessButton,
			},
			discordgo.Button{
				Label:    "Thêm TikTok",
				CustomID: BtnAddFeedTiktok,
				Style:    discordgo.PrimaryButton,
			},
			discordgo.Button{
				Label:    "Không ping",
				CustomID: BtnFeedNoPing,
				Style:    discordgo.SecondaryButton,
			},
		},
	})

	// Row 2: Chọn role ping
	components = append(components, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:    discordgo.RoleSelectMenu,
				CustomID:    SelectFeedRole,
				Placeholder: "Chọn role ping (hoặc bấm Không ping)...",
			},
		},
	})

	// Row 3: Chọn kênh Discord nhận thông báo
	components = append(components, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:     discordgo.ChannelSelectMenu,
				CustomID:     SelectFeedChannel,
				Placeholder:  "Chọn kênh nhận thông báo video mới...",
				ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText},
			},
		},
	})

	// Row 4: Xóa kênh đang theo dõi (nếu có)
	if len(subs) > 0 {
		var delOptions []discordgo.SelectMenuOption
		for i, sub := range subs {
			if i >= 25 {
				break
			}
			plat := "YouTube"
			if sub.Platform == "tiktok" {
				plat = "TikTok"
			}
			label := fmt.Sprintf("[%s] %s", plat, sub.TargetName)
			if len(label) > 100 {
				label = label[:97] + "..."
			}
			delOptions = append(delOptions, discordgo.SelectMenuOption{
				Label:       label,
				Value:       fmt.Sprintf("%d", sub.ID),
				Description: fmt.Sprintf("Kênh nhận: #%s", sub.ChannelID),
			})
		}

		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    SelectDeleteFeed,
					Placeholder: "Chọn kênh video muốn xóa khỏi theo dõi...",
					Options:     delOptions,
				},
			},
		})
	}

	// Row cuối: Quay lại
	components = append(components, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Quay lại",
				CustomID: BtnBackToMain,
				Style:    discordgo.SecondaryButton,
			},
		},
	})

	return embed, components
}
