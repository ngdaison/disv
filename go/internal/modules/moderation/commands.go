package moderation

import (
	"fmt"
	"time"

	"botdis/internal/discord"

	"github.com/bwmarrin/discordgo"
)

func (as *AntiSpam) RegisterCommands(r *discord.Router) {
	r.RegisterCommand("clear", as.handleClear)
	r.RegisterCommand("kick", as.handleKick)
	r.RegisterCommand("ban", as.handleBan)
	r.RegisterCommand("mute", as.handleMute)
	r.RegisterCommand("unmute", as.handleUnmute)
}

func (as *AntiSpam) handleClear(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !as.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Bạn không có quyền dùng lệnh này.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	amount := int64(10)
	options := i.ApplicationCommandData().Options
	if len(options) > 0 {
		amount = options[0].IntValue()
	}
	if amount <= 0 {
		amount = 1
	}
	if amount > 100 {
		amount = 100
	}

	msgs, err := sess.ChannelMessages(i.ChannelID, int(amount), "", "", "")
	if err != nil {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Lỗi khi lấy tin nhắn.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	var msgIDs []string
	for _, m := range msgs {
		msgIDs = append(msgIDs, m.ID)
	}

	_ = sess.ChannelMessagesBulkDelete(i.ChannelID, msgIDs)

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🧹 Đã xóa `%d` tin nhắn!", len(msgIDs)),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func (as *AntiSpam) handleKick(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !as.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Bạn không có quyền Kick.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	options := i.ApplicationCommandData().Options
	targetUser := options[0].UserValue(sess)
	reason := "Không có lý do"
	if len(options) > 1 {
		reason = options[1].StringValue()
	}

	err := sess.GuildMemberDeleteWithReason(i.GuildID, targetUser.ID, reason)
	msg := fmt.Sprintf("👢 Đã Kick <@%s>. Lý do: %s", targetUser.ID, reason)
	if err != nil {
		msg = fmt.Sprintf("❌ Không thể Kick thành viên: %v", err)
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	})
}

func (as *AntiSpam) handleBan(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !as.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Bạn không có quyền Ban.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	options := i.ApplicationCommandData().Options
	targetUser := options[0].UserValue(sess)
	reason := "Không có lý do"
	if len(options) > 1 {
		reason = options[1].StringValue()
	}

	err := sess.GuildBanCreateWithReason(i.GuildID, targetUser.ID, reason, 0)
	msg := fmt.Sprintf("🔨 Đã Ban <@%s>. Lý do: %s", targetUser.ID, reason)
	if err != nil {
		msg = fmt.Sprintf("❌ Không thể Ban thành viên: %v", err)
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	})
}

func (as *AntiSpam) handleMute(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !as.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Bạn không có quyền Mute.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	options := i.ApplicationCommandData().Options
	targetUser := options[0].UserValue(sess)
	minutes := int64(10)
	if len(options) > 1 {
		minutes = options[1].IntValue()
	}
	reason := "Không có lý do"
	if len(options) > 2 {
		reason = options[2].StringValue()
	}

	until := time.Now().Add(time.Duration(minutes) * time.Minute)
	err := sess.GuildMemberTimeout(i.GuildID, targetUser.ID, &until)
	msg := fmt.Sprintf("🔇 Đã Mute (Timeout) <@%s> trong `%d phút`. Lý do: %s", targetUser.ID, minutes, reason)
	if err != nil {
		msg = fmt.Sprintf("❌ Không thể Mute thành viên: %v", err)
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	})
}

func (as *AntiSpam) handleUnmute(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if !as.isUserAdmin(sess, i.GuildID, i.Member.User.ID) {
		_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Bạn không có quyền Unmute.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	options := i.ApplicationCommandData().Options
	targetUser := options[0].UserValue(sess)

	err := sess.GuildMemberTimeout(i.GuildID, targetUser.ID, nil)
	msg := fmt.Sprintf("🔊 Đã bỏ Mute (Timeout) cho <@%s>.", targetUser.ID)
	if err != nil {
		msg = fmt.Sprintf("❌ Không thể bỏ Mute: %v", err)
	}

	_ = sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	})
}
