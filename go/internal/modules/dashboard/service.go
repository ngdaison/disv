package dashboard

import (
	"botdis/internal/discord"
	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

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
}

func (s *Service) handleSettingCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !s.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Bạn cần quyền Administrator để mở bảng cài đặt.",
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

func (s *Service) buildDashboardEmbed(guildID, channelID string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "⚙️ Bảng Điều Khiển Bot",
		Description: "Bấm vào các nút bên dưới để Bật (Xanh lá) hoặc Tắt (Đỏ).",
		Color:       0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "🚫 Chống Spam & Nội Dung",
				Value: "**Chặn Link**: Xóa tin nhắn chứa link.\n" +
					"**Chặn Media**: Xóa Ảnh và Video.\n" +
					"**Chặn File**: Xóa các file đính kèm.\n" +
					"**Chặn Chat**: Chỉ xóa tin nhắn Text thuần.\n" +
					"**Anti Fast**: Xóa toàn bộ tin nhắn khi gửi quá nhanh qua các kênh.\n" +
					"**Anti Dup**: Xóa toàn bộ tin nhắn khi gửi trùng lặp qua các kênh.",
			},
			{
				Name: "⭐ Tiện Ích & Tính Năng",
				Value: "**TikTok**: Tự động tải video không logo & nén theo server.\n" +
					"**Leveling**: Hệ thống cộng điểm kinh nghiệm XP & Cấp độ.\n" +
					"**AI Chat**: Trò chuyện tự động với Google Gemini AI.",
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
				discordgo.Button{Label: "Cấm Link", CustomID: "toggle_ac_link", Style: btnStyle(settings.ACLink)},
				discordgo.Button{Label: "Cấm Media", CustomID: "toggle_ac_media", Style: btnStyle(settings.ACMedia)},
				discordgo.Button{Label: "Cấm File", CustomID: "toggle_ac_file", Style: btnStyle(settings.ACFile)},
				discordgo.Button{Label: "Cấm Chat", CustomID: "toggle_ac_text", Style: btnStyle(settings.ACText)},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Anti Fast", CustomID: "toggle_as_fast", Style: btnStyle(settings.ASFast)},
				discordgo.Button{Label: "Anti Dup", CustomID: "toggle_as_dup", Style: btnStyle(settings.ASDup)},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Leveling", CustomID: "toggle_level_enabled", Style: btnStyle(settings.LevelEnabled)},
				discordgo.Button{Label: "TikTok", CustomID: "toggle_tiktok_enabled", Style: btnStyle(settings.TikTokEnabled)},
				discordgo.Button{Label: "AI Chat", CustomID: "toggle_ai_enabled", Style: btnStyle(settings.AIEnabled)},
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
