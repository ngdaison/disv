package leveling

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"botdis/internal/discord"
	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type Service struct {
	store     *storage.MySQLStore
	cooldowns map[string]time.Time
	lock      sync.Mutex
}

func NewService(store *storage.MySQLStore) *Service {
	return &Service{
		store:     store,
		cooldowns: make(map[string]time.Time),
	}
}

func (s *Service) RegisterRoutes(r *discord.Router) {
	r.RegisterCommand("rank", s.handleRankCommand)
}

func (s *Service) HandleMessage(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return
	}

	settings := s.store.GetChannelSettings(m.GuildID, m.ChannelID)
	if !settings.LevelEnabled {
		return
	}

	key := m.GuildID + "_" + m.Author.ID

	s.lock.Lock()
	lastXPTime, exists := s.cooldowns[key]
	if exists && time.Since(lastXPTime) < 60*time.Second {
		s.lock.Unlock()
		return
	}
	s.cooldowns[key] = time.Now()
	s.lock.Unlock()

	xpGained := rand.Intn(11) + 15
	newXP, newLevel, leveledUp, err := s.store.AddUserXP(m.GuildID, m.Author.ID, xpGained)
	if err != nil {
		return
	}

	if leveledUp {
		embed := &discordgo.MessageEmbed{
			Title:       "🎉 Chúc Mừng Thăng Cấp!",
			Description: fmt.Sprintf("Chúc mừng <@%s> đã đạt **Cấp độ %d**! (Hiện tại: %d XP)", m.Author.ID, newLevel, newXP),
			Color:       0xf1c40f,
		}
		_, _ = sess.ChannelMessageSendEmbed(m.ChannelID, embed)
	}
}

func (s *Service) handleRankCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	targetUser := i.Member.User
	data := i.ApplicationCommandData()
	if len(data.Options) > 0 {
		targetUser = data.Options[0].UserValue(sess)
	}

	ul, err := s.store.GetUserLevel(i.GuildID, targetUser.ID)
	if err != nil || ul == nil {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Không thể lấy dữ liệu cấp độ.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	xpNeeded := ul.Level * 100
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("📊 Thẻ Cấp Độ - %s", targetUser.Username),
		Color: 0x3498db,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: targetUser.AvatarURL("256"),
		},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "⭐ Cấp Độ", Value: fmt.Sprintf("**%d**", ul.Level), Inline: true},
			{Name: "✨ Điểm Kinh Nghiệm (XP)", Value: fmt.Sprintf("**%d / %d**", ul.XP, xpNeeded), Inline: true},
		},
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
