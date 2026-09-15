package wordchain

import (
	"fmt"
	"strings"
	"unicode"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

const (
	BtnToggleChannel  = "noitu_toggle_channel"
	BtnNewGame        = "noitu_new_game"
	BtnLeaderboard    = "noitu_leaderboard"
	BtnMyStats        = "noitu_my_stats"
	BtnLookupModal    = "noitu_lookup_modal"
	BtnHint           = "noitu_hint"
	BtnToggleSolo     = "noitu_toggle_solo"
	BtnRules          = "noitu_rules"
	ModalLookupSubmit = "noitu_modal_lookup_submit"
	ModalInputWord    = "noitu_input_word"
)

func capitalizeFirst(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func BuildControlPanelEmbed(ch *storage.WordChainChannel, channelID string) *discordgo.MessageEmbed {
	statusText := "Đang tắt"
	color := 0xe74c3c

	if ch != nil && ch.IsActive {
		statusText = "Đang hoạt động"
		color = 0x2ecc71
	}

	currentWord := "Chưa có"
	nextSyllable := "Tự do"
	streak := 0
	bestStreak := 0

	if ch != nil {
		if ch.CurrentWord != "" {
			currentWord = capitalizeFirst(ch.CurrentWord)
			parts := strings.Fields(ch.CurrentWord)
			if len(parts) >= 2 {
				nextSyllable = parts[len(parts)-1]
			}
		}
		streak = ch.CurrentStreak
		bestStreak = ch.HighestStreak
	}

	return &discordgo.MessageEmbed{
		Title: "Bảng điều khiển nối từ",
		Description: fmt.Sprintf("Kênh <#%s> • %s\nTừ hiện tại **%s**\nBắt đầu bằng **%s**\nChuỗi **%d** • Kỷ lục **%d**",
			channelID, statusText, currentWord, nextSyllable, streak, bestStreak),
		Color: color,
	}
}

func BuildControlPanelComponents(ch *storage.WordChainChannel, isAdmin bool) []discordgo.MessageComponent {
	if !isAdmin {
		// Dành cho người không có quyền admin: chỉ hiện Tra từ, Gợi ý, Xếp hạng, Hồ sơ, Luật
		return []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Tra từ",
						CustomID: BtnLookupModal,
						Style:    discordgo.SecondaryButton,
					},
					discordgo.Button{
						Label:    "Gợi ý",
						CustomID: BtnHint,
						Style:    discordgo.SecondaryButton,
					},
					discordgo.Button{
						Label:    "Xếp hạng",
						CustomID: BtnLeaderboard,
						Style:    discordgo.SuccessButton,
					},
					discordgo.Button{
						Label:    "Hồ sơ",
						CustomID: BtnMyStats,
						Style:    discordgo.SecondaryButton,
					},
					discordgo.Button{
						Label:    "Luật",
						CustomID: BtnRules,
						Style:    discordgo.SecondaryButton,
					},
				},
			},
		}
	}

	toggleLabel := "Bật kênh"
	toggleStyle := discordgo.SuccessButton
	if ch != nil && ch.IsActive {
		toggleLabel = "Tắt kênh"
		toggleStyle = discordgo.DangerButton
	}

	soloLabel := "Bật solo"
	if ch != nil && ch.AllowSolo {
		soloLabel = "Tắt solo"
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    toggleLabel,
					CustomID: BtnToggleChannel,
					Style:    toggleStyle,
				},
				discordgo.Button{
					Label:    "Ván mới",
					CustomID: BtnNewGame,
					Style:    discordgo.PrimaryButton,
				},
				discordgo.Button{
					Label:    soloLabel,
					CustomID: BtnToggleSolo,
					Style:    discordgo.SecondaryButton,
				},
				discordgo.Button{
					Label:    "Tra từ",
					CustomID: BtnLookupModal,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Gợi ý",
					CustomID: BtnHint,
					Style:    discordgo.SecondaryButton,
				},
				discordgo.Button{
					Label:    "Xếp hạng",
					CustomID: BtnLeaderboard,
					Style:    discordgo.SuccessButton,
				},
				discordgo.Button{
					Label:    "Hồ sơ",
					CustomID: BtnMyStats,
					Style:    discordgo.SecondaryButton,
				},
				discordgo.Button{
					Label:    "Luật",
					CustomID: BtnRules,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
	}
}

func BuildLeaderboardEmbed(guildName string, list []storage.WordChainUserStat) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("Bảng xếp hạng %s", guildName),
		Color: 0xf1c40f,
	}

	if len(list) == 0 {
		embed.Description = "Chưa có dữ liệu"
		return embed
	}

	var lines []string

	for i, s := range list {
		rankPrefix := fmt.Sprintf("#%d", i+1)
		lines = append(lines, fmt.Sprintf("%s <@%s> **%d** điểm • %d từ chuỗi %d",
			rankPrefix, s.UserID, s.Score, s.WordsCount, s.BestStreak))
	}

	embed.Description = strings.Join(lines, "\n")
	return embed
}

func BuildUserStatEmbed(user *discordgo.User, stat *storage.WordChainUserStat) *discordgo.MessageEmbed {
	total := stat.WordsCount + stat.WrongCount
	acc := 0.0
	if total > 0 {
		acc = (float64(stat.WordsCount) / float64(total)) * 100.0
	}

	return &discordgo.MessageEmbed{
		Title: fmt.Sprintf("Hồ sơ %s", user.Username),
		Color: 0x3498db,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: user.AvatarURL("256"),
		},
		Description: fmt.Sprintf("Điểm **%d**\nĐúng **%d** từ\nChuỗi kỷ lục **%d**\nSai **%d** lần\nChính xác **%.1f%%**",
			stat.Score, stat.WordsCount, stat.BestStreak, stat.WrongCount, acc),
	}
}

func BuildWordDefinitionEmbed(word string, meanings []WordMeaning) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("Tra từ %s", capitalizeFirst(word)),
		Color: 0x9b59b6,
	}

	if len(meanings) == 0 {
		embed.Description = fmt.Sprintf("Từ **%s** chưa có định nghĩa chi tiết", word)
		return embed
	}

	var lines []string
	for i, m := range meanings {
		posText := ""
		if m.POS != "" {
			posText = fmt.Sprintf(" *(%s)*", m.POS)
		}
		lines = append(lines, fmt.Sprintf("**%d**%s %s", i+1, posText, m.Definition))
	}

	embed.Description = strings.Join(lines, "\n\n")
	return embed
}

func BuildRulesEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Luật chơi",
		Color: 0x1abc9c,
		Description: "• Từ nối gồm đúng 2 tiếng\n" +
			"• Âm tiết đầu từ sau trùng âm tiết cuối từ trước\n" +
			"• Không lặp lại từ đã dùng\n" +
			"• Không tự nối 2 từ liên tiếp khi tắt solo\n" +
			"• Chat trực tiếp vào kênh để chơi\n" +
			"• Từ cụt nhận thưởng lớn và mở ván mới",
	}
}

func BuildLookupModal() *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalLookupSubmit,
			Title:    "Tra từ điển",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    ModalInputWord,
							Label:       "Từ cần tra",
							Style:       discordgo.TextInputShort,
							Placeholder: "Ví dụ bác sĩ, học tập",
							Required:    true,
							MinLength:   2,
							MaxLength:   50,
						},
					},
				},
			},
		},
	}
}
