package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"botdis/config"
	"botdis/internal/discord"
	"botdis/internal/modules/autoroles"
	"botdis/internal/modules/chatai"
	"botdis/internal/modules/dashboard"
	"botdis/internal/modules/leveling"
	"botdis/internal/modules/moderation"
	"botdis/internal/modules/ticket"
	"botdis/internal/modules/tiktok"
	"botdis/internal/modules/utility"
	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

func main() {
	log.Println("🚀 Đang khởi động Bot Discord bằng Golang...")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Lỗi nạp cấu hình: %v", err)
	}

	store, err := storage.OpenMySQL(cfg.MySQL)
	if err != nil {
		log.Fatalf("Lỗi kết nối MySQL: %v", err)
	}
	defer store.Close()

	router := discord.NewRouter()
	tiktokService := tiktok.NewService(store)
	ticketService := ticket.NewService(store)
	antiSpam := moderation.NewAntiSpam(store)
	levelingService := leveling.NewService(store)
	utilityService := utility.NewService()
	autorolesService := autoroles.NewService(store)
	dashboardService := dashboard.NewService(store)
	chataiService := chatai.NewService(cfg.AIAPIKey, store)

	ticketService.RegisterRoutes(router)
	levelingService.RegisterRoutes(router)
	utilityService.RegisterRoutes(router)
	autorolesService.RegisterRoutes(router)
	dashboardService.RegisterRoutes(router)
	antiSpam.RegisterCommands(router)

	sess, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("Lỗi tạo Discord session: %v", err)
	}

	sess.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsMessageContent

	sess.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("✅ Đã đăng nhập với tư cách: %s#%s (ID: %s)", r.User.Username, r.User.Discriminator, r.User.ID)
		if cfg.StatusMessage != "" {
			_ = s.UpdateGameStatus(0, cfg.StatusMessage)
		}

		commands := []*discordgo.ApplicationCommand{
			{
				Name:        "ticket",
				Description: "Hệ thống Ticket Hỗ Trợ",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "send",
						Description: "Gửi bảng Hỗ Trợ mẫu vào kênh chỉ định",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionChannel,
								Name:        "channel",
								Description: "Kênh muốn gửi bảng Hỗ Trợ (mặc định là kênh hiện tại)",
								Required:    false,
							},
						},
					},
				},
			},
			{
				Name:        "setting",
				Description: "Bảng điều khiển cài đặt Bot cho kênh hiện tại",
			},
			{
				Name:        "ping",
				Description: "Kiểm tra độ trễ của bot",
			},
			{
				Name:        "botinfo",
				Description: "Xem thông tin hệ thống, RAM và phiên bản Go",
			},
			{
				Name:        "rank",
				Description: "Xem cấp độ và điểm kinh nghiệm XP",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "Người dùng cần xem (mặc định là bản thân)",
						Required:    false,
					},
				},
			},
			{
				Name:        "autorole",
				Description: "Cấu hình role tự động gán cho thành viên mới",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "set",
						Description: "Chọn role tự động gán",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionRole,
								Name:        "role",
								Description: "Role cần gán",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "show",
						Description: "Xem role tự động hiện tại",
					},
				},
			},
			{
				Name:        "clear",
				Description: "Xóa tin nhắn hàng loạt trong kênh",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionInteger,
						Name:        "amount",
						Description: "Số lượng tin nhắn cần xóa (1 - 100)",
						Required:    false,
					},
				},
			},
			{
				Name:        "mute",
				Description: "Mute (Timeout) thành viên",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "Thành viên cần Mute",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionInteger,
						Name:        "minutes",
						Description: "Thời gian Mute (phút)",
						Required:    false,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "reason",
						Description: "Lý do Mute",
						Required:    false,
					},
				},
			},
			{
				Name:        "unmute",
				Description: "Bỏ Mute (Timeout) cho thành viên",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "Thành viên cần bỏ Mute",
						Required:    true,
					},
				},
			},
			{
				Name:        "kick",
				Description: "Kick thành viên khỏi server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "Thành viên cần Kick",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "reason",
						Description: "Lý do Kick",
						Required:    false,
					},
				},
			},
			{
				Name:        "ban",
				Description: "Ban thành viên khỏi server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "Thành viên cần Ban",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "reason",
						Description: "Lý do Ban",
						Required:    false,
					},
				},
			},
		}

		for _, cmd := range commands {
			_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
			if err != nil {
				log.Printf("Lỗi đăng ký command /%s: %v", cmd.Name, err)
			} else {
				log.Printf("Đã đồng bộ command: /%s", cmd.Name)
			}
		}
	})

	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		router.HandleInteraction(s, i)
	})

	sess.AddHandler(func(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
		autorolesService.HandleGuildMemberAdd(s, m)
	})

	sess.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.Bot {
			return
		}

		violated := antiSpam.HandleMessage(s, m)
		if violated {
			return
		}

		levelingService.HandleMessage(s, m)

		tiktokService.HandleMessage(s, m)

		chataiService.HandleMessage(s, m)
	})

	err = sess.Open()
	if err != nil {
		log.Fatalf("Lỗi mở kết nối Discord: %v", err)
	}
	defer sess.Close()

	log.Println("⚡ Bot Go đang hoạt động! Nhấn Ctrl+C để dừng.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	fmt.Println("\nĐang tắt Bot Go...")
}
