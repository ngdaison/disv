package moderation

import (
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type TrackedMessage struct {
	ChannelID string
	MessageID string
	Content   string
	SentAt    time.Time
}

type AntiSpam struct {
	lock        sync.Mutex
	userHistory map[string][]TrackedMessage
	store       *storage.MySQLStore
}

func NewAntiSpam(store *storage.MySQLStore) *AntiSpam {
	as := &AntiSpam{
		userHistory: make(map[string][]TrackedMessage),
		store:       store,
	}

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			as.cleanOldMessages()
		}
	}()

	return as
}

func (as *AntiSpam) cleanOldMessages() {
	as.lock.Lock()
	defer as.lock.Unlock()

	cutoff := time.Now().Add(-2 * time.Minute)
	for key, msgs := range as.userHistory {
		var valid []TrackedMessage
		for _, m := range msgs {
			if m.SentAt.After(cutoff) {
				valid = append(valid, m)
			}
		}
		if len(valid) == 0 {
			delete(as.userHistory, key)
		} else {
			as.userHistory[key] = valid
		}
	}
}

func (as *AntiSpam) HandleMessage(sess *discordgo.Session, m *discordgo.MessageCreate) bool {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return false
	}

	if as.isUserAdmin(sess, m.GuildID, m.Author.ID) {
		return false
	}

	settings := as.store.GetChannelSettings(m.GuildID, m.ChannelID)

	if settings.ACLink && hasLink(m.Content) {
		_ = sess.ChannelMessageDelete(m.ChannelID, m.ID)
		return true
	}

	if settings.ACMedia && len(m.Attachments) > 0 {
		for _, att := range m.Attachments {
			if strings.HasPrefix(att.ContentType, "image/") || strings.HasPrefix(att.ContentType, "video/") {
				_ = sess.ChannelMessageDelete(m.ChannelID, m.ID)
				return true
			}
		}
	}

	if settings.ACFile && len(m.Attachments) > 0 {
		_ = sess.ChannelMessageDelete(m.ChannelID, m.ID)
		return true
	}

	if settings.ACText && !hasLink(m.Content) && len(m.Attachments) == 0 && len(m.StickerItems) == 0 {
		_ = sess.ChannelMessageDelete(m.ChannelID, m.ID)
		return true
	}

	userKey := m.GuildID + "_" + m.Author.ID
	now := time.Now()

	as.lock.Lock()
	history := as.userHistory[userKey]
	currentTracked := TrackedMessage{
		ChannelID: m.ChannelID,
		MessageID: m.ID,
		Content:   strings.TrimSpace(m.Content),
		SentAt:    now,
	}
	history = append(history, currentTracked)
	as.userHistory[userKey] = history
	as.lock.Unlock()

	if settings.ASFast && len(history) >= 2 {
		recentCount := 0
		var toDeleteFast []TrackedMessage
		for i := len(history) - 1; i >= 0; i-- {
			if now.Sub(history[i].SentAt) <= 1*time.Second {
				recentCount++
				toDeleteFast = append(toDeleteFast, history[i])
			} else {
				break
			}
		}

		if recentCount >= 2 {
			log.Printf("Phát hiện Anti-Fast từ user %s! Xóa tất cả tin nhắn vi phạm.", m.Author.Username)
			as.deleteMessages(sess, toDeleteFast)
			return true
		}
	}

	if settings.ASDup && currentTracked.Content != "" {
		var toDeleteDup []TrackedMessage
		for _, msg := range history {
			if now.Sub(msg.SentAt) <= 60*time.Second && msg.Content == currentTracked.Content {
				toDeleteDup = append(toDeleteDup, msg)
			}
		}

		if len(toDeleteDup) >= 2 {
			log.Printf("Phát hiện Anti-Dup từ user %s! Xóa tất cả tin nhắn trùng lặp.", m.Author.Username)
			as.deleteMessages(sess, toDeleteDup)
			return true
		}
	}

	return false
}

func (as *AntiSpam) deleteMessages(sess *discordgo.Session, msgs []TrackedMessage) {
	for _, msg := range msgs {
		go func(chID, msgID string) {
			_ = sess.ChannelMessageDelete(chID, msgID)
		}(msg.ChannelID, msg.MessageID)
	}
}

func (as *AntiSpam) isUserAdmin(sess *discordgo.Session, guildID, userID string) bool {
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

func hasLink(content string) bool {
	words := strings.Fields(content)
	for _, w := range words {
		if u, err := url.Parse(w); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			return true
		}
	}
	return false
}
