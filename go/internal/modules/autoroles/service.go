package autoroles

import (
	"fmt"
	"time"

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
}

func (s *Service) HandleGuildMemberAdd(sess *discordgo.Session, m *discordgo.GuildMemberAdd) {
	gData := s.store.GetGuildData(m.GuildID)
	roleID, exists := gData.AutoJoinRoles["default"]
	if !exists || roleID == "" {
		return
	}

	if gData.AutoJoinDelay > 0 {
		go func(gid, uid, rid string, delaySec int) {
			time.Sleep(time.Duration(delaySec) * time.Second)
			_ = sess.GuildMemberRoleAdd(gid, uid, rid)
		}(m.GuildID, m.User.ID, roleID, gData.AutoJoinDelay)
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
				Content: fmt.Sprintf("Đã thiết lập autorole <@&%s>", role.ID),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

	case "show":
		gData := s.store.GetGuildData(i.GuildID)
		roleID := gData.AutoJoinRoles["default"]
		msg := "Chưa thiết lập autorole"
		if roleID != "" {
			msg = fmt.Sprintf("Autorole hiện tại <@&%s>", roleID)
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
