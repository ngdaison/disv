package utility

import (
	"fmt"
	"runtime"
	"time"

	"botdis/internal/discord"

	"github.com/bwmarrin/discordgo"
)

var startTime = time.Now()

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) RegisterRoutes(r *discord.Router) {
	r.RegisterCommand("ping", s.handlePing)
	r.RegisterCommand("botinfo", s.handleBotInfo)
}

func (s *Service) handlePing(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	latency := sess.HeartbeatLatency().Milliseconds()
	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🏓 **Pong!** Độ trễ Gateway: `%dms`", latency),
		},
	})
}

func (s *Service) handleBotInfo(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	allocMB := float64(m.Alloc) / 1024 / 1024
	sysMB := float64(m.Sys) / 1024 / 1024
	uptime := time.Since(startTime).Round(time.Second)

	embed := &discordgo.MessageEmbed{
		Title: "🤖 Thông Tin Bot (Golang Native Edition)",
		Color: 0x00add8,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "⚡ Ngôn ngữ", Value: fmt.Sprintf("Go %s", runtime.Version()), Inline: true},
			{Name: "⏳ Thời gian chạy", Value: uptime.String(), Inline: true},
			{Name: "💾 RAM đang dùng", Value: fmt.Sprintf("**%.2f MB** (Sys: %.2f MB)", allocMB, sysMB), Inline: true},
			{Name: "🧵 Goroutines", Value: fmt.Sprintf("%d", runtime.NumGoroutine()), Inline: true},
			{Name: "🖥️ Hệ điều hành", Value: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Hiệu năng cao • Tiết kiệm 85% RAM so với Python",
		},
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
