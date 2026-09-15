package sticky

import (
	"log"
	"strings"
	"sync"
	"time"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type channelState struct {
	mu           sync.Mutex
	timer        *time.Timer
	firstMsgTime time.Time
	msgCount     int
	typingUsers  map[string]time.Time
}

type Service struct {
	store    *storage.MySQLStore
	channels sync.Map // map[string]*channelState (key: channelID)
}

func NewService(store *storage.MySQLStore) *Service {
	return &Service{
		store: store,
	}
}

func (s *Service) getChannelState(channelID string) *channelState {
	csVal, _ := s.channels.LoadOrStore(channelID, &channelState{
		typingUsers: make(map[string]time.Time),
	})
	return csVal.(*channelState)
}

// HandleTypingStart theo dõi khi người dùng bắt đầu gõ phím để tối ưu thời gian chờ
func (s *Service) HandleTypingStart(sess *discordgo.Session, t *discordgo.TypingStart) {
	if t.GuildID == "" || (sess.State != nil && sess.State.User != nil && t.UserID == sess.State.User.ID) {
		return
	}

	cs := s.getChannelState(t.ChannelID)
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.typingUsers == nil {
		cs.typingUsers = make(map[string]time.Time)
	}
	cs.typingUsers[t.UserID] = time.Now()
}

// HandleMessage xử lý khi có tin nhắn mới trong kênh
func (s *Service) HandleMessage(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" || m.Author == nil || m.Author.Bot {
		return
	}

	settings := s.store.GetChannelSettings(m.GuildID, m.ChannelID)
	if strings.TrimSpace(settings.StickyContent) == "" {
		return
	}

	// Bỏ qua nếu tin nhắn chính là tin nhắn ghim
	if m.ID == settings.StickyLastID {
		return
	}

	cs := s.getChannelState(m.ChannelID)
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.typingUsers == nil {
		cs.typingUsers = make(map[string]time.Time)
	}

	// Người gửi tin nhắn này đã xong lượt gõ
	delete(cs.typingUsers, m.Author.ID)

	// Dọn dẹp những người gõ đã quá 8 giây
	now := time.Now()
	for uid, tTime := range cs.typingUsers {
		if now.Sub(tTime) > 8*time.Second {
			delete(cs.typingUsers, uid)
		}
	}

	// 1. Xóa ngay lập tức tin nhắn ghim cũ nếu có
	if settings.StickyLastID != "" {
		oldID := settings.StickyLastID
		s.store.UpdateChannelSetting(m.GuildID, m.ChannelID, func(c *storage.ChannelSettings) {
			c.StickyLastID = ""
		})
		go func(chID, msgID string) {
			_ = sess.ChannelMessageDelete(chID, msgID)
		}(m.ChannelID, oldID)
	}

	// 2. Gom nhóm debounce thông minh
	if cs.firstMsgTime.IsZero() {
		cs.firstMsgTime = now
		cs.msgCount = 1
	} else {
		cs.msgCount++
	}

	activeTypers := len(cs.typingUsers)

	// Xác định độ trễ gửi lại:
	// - Nếu chỉ 1 tin và 0 người đang gõ: Gửi nhanh sau 1.2 giây
	// - Nếu nhiều người gửi hoặc đang có người gõ: Đợi 2.5 giây cho đoạn chat ngưng
	var delay time.Duration
	if cs.msgCount == 1 && activeTypers == 0 {
		delay = 1200 * time.Millisecond
	} else {
		delay = 2500 * time.Millisecond
	}

	// Giới hạn trần tối đa 5 giây từ tin nhắn đầu tiên của đợt chat
	elapsed := now.Sub(cs.firstMsgTime)
	remainingMax := 5*time.Second - elapsed
	if remainingMax <= 0 {
		delay = 50 * time.Millisecond
	} else if delay > remainingMax {
		delay = remainingMax
	}

	if cs.timer != nil {
		cs.timer.Stop()
	}

	guildID := m.GuildID
	channelID := m.ChannelID

	cs.timer = time.AfterFunc(delay, func() {
		s.postSticky(sess, guildID, channelID, cs)
	})
}

// postSticky gửi tin nhắn ghim mới xuống cuối kênh
func (s *Service) postSticky(sess *discordgo.Session, guildID, channelID string, cs *channelState) {
	cs.mu.Lock()
	cs.firstMsgTime = time.Time{}
	cs.msgCount = 0
	cs.mu.Unlock()

	current := s.store.GetChannelSettings(guildID, channelID)
	if strings.TrimSpace(current.StickyContent) == "" {
		return
	}

	if current.StickyLastID != "" {
		_ = sess.ChannelMessageDelete(channelID, current.StickyLastID)
	}

	newMsg, err := sess.ChannelMessageSend(channelID, current.StickyContent)
	if err != nil {
		log.Printf("Lỗi gửi tin nhắn ghim tại kênh %s: %v", channelID, err)
		return
	}

	if newMsg != nil {
		s.store.UpdateChannelSetting(guildID, channelID, func(c *storage.ChannelSettings) {
			c.StickyLastID = newMsg.ID
		})
	}
}

// SetSticky thiết lập hoặc xóa tin nhắn ghim từ Admin Modal
func (s *Service) SetSticky(sess *discordgo.Session, guildID, channelID, content string) error {
	cs := s.getChannelState(channelID)

	cs.mu.Lock()
	if cs.timer != nil {
		cs.timer.Stop()
		cs.timer = nil
	}
	cs.firstMsgTime = time.Time{}
	cs.msgCount = 0
	cs.mu.Unlock()

	current := s.store.GetChannelSettings(guildID, channelID)
	if current.StickyLastID != "" {
		_ = sess.ChannelMessageDelete(channelID, current.StickyLastID)
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		s.store.UpdateChannelSetting(guildID, channelID, func(c *storage.ChannelSettings) {
			c.StickyContent = ""
			c.StickyLastID = ""
		})
		return nil
	}

	newMsg, err := sess.ChannelMessageSend(channelID, trimmed)
	if err != nil {
		return err
	}

	s.store.UpdateChannelSetting(guildID, channelID, func(c *storage.ChannelSettings) {
		c.StickyContent = trimmed
		c.StickyLastID = newMsg.ID
	})

	return nil
}
