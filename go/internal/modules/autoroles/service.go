package autoroles

import (
	"fmt"

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
	r.RegisterCommand("autorole", s.handleAutoRoleCommand)
}

func (s *Service) HandleGuildMemberAdd(sess *discordgo.Session, m *discordgo.GuildMemberAdd) {
	gData := s.store.GetGuildData(m.GuildID)
	roleID, exists := gData.AutoJoinRoles["default"]
	if !exists || roleID == "" {
		return
	}

	_ = sess.GuildMemberRoleAdd(m.GuildID, m.User.ID, roleID)
}

func (s *Service) handleAutoRoleCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}

	subCmd := data.Options[0]
	switch subCmd.Name {
	case "set":
		role := subCmd.Options[0].RoleValue(sess, i.GuildID)
		s.store.SetAutoRole(i.GuildID, role.ID)

		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("✅ Đã thiết lập AutoRole: <@&%s>", role.ID),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

	case "show":
		gData := s.store.GetGuildData(i.GuildID)
		roleID := gData.AutoJoinRoles["default"]
		msg := "Chưa thiết lập AutoRole."
		if roleID != "" {
			msg = fmt.Sprintf("AutoRole hiện tại: <@&%s>", roleID)
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
