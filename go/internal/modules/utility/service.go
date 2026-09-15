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
}

func BuildBotInfoEmbed() *discordgo.MessageEmbed {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	allocMB := float64(m.Alloc) / 1024 / 1024
	uptime := time.Since(startTime).Round(time.Second)

	return &discordgo.MessageEmbed{
		Title: "Thông tin bot",
		Color: 0x00add8,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Ngôn ngữ", Value: fmt.Sprintf("Go %s", runtime.Version()), Inline: true},
			{Name: "Thời gian chạy", Value: uptime.String(), Inline: true},
			{Name: "RAM đang dùng", Value: fmt.Sprintf("**%.2f MB**", allocMB), Inline: true},
			{Name: "Goroutines", Value: fmt.Sprintf("%d", runtime.NumGoroutine()), Inline: true},
			{Name: "Hệ điều hành", Value: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH), Inline: true},
		},
	}
}

func GetPingContent(sess *discordgo.Session) string {
	latency := sess.HeartbeatLatency().Milliseconds()
	return fmt.Sprintf("**Pong!** Độ trễ gateway `%dms`", latency)
}
