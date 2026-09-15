package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"botdis/config"

	_ "github.com/go-sql-driver/mysql"
)

type ChannelSettings struct {
	ACLink        bool   `json:"ac_link"`
	ACMedia       bool   `json:"ac_media"`
	ACImage       bool   `json:"ac_image"`
	ACFile        bool   `json:"ac_file"`
	ACText        bool   `json:"ac_text"`
	ASFast        bool   `json:"as_fast"`
	ASDup         bool   `json:"as_dup"`
	LevelEnabled  bool   `json:"level_enabled"`
	TikTokEnabled bool   `json:"tiktok_enabled"`
	StickyContent string `json:"sticky_content,omitempty"`
	StickyLastID  string `json:"sticky_last_id,omitempty"`
	AIEnabled     bool   `json:"ai_enabled"`
}

type GuildData struct {
	Settings      map[string]*ChannelSettings `json:"settings"`
	Users         map[string]interface{}      `json:"users,omitempty"`
	BadVideos     []string                    `json:"bad_videos,omitempty"`
	LevelRoles    map[string]string           `json:"level_roles,omitempty"`
	AutoJoinRoles map[string]string           `json:"auto_join_roles,omitempty"`
	AutoJoinDelay int                         `json:"auto_join_delay"`
	RoleMenus     map[string][]string         `json:"role_menus,omitempty"`
	ASFast        bool                        `json:"as_fast"`
	ASDup         bool                        `json:"as_dup"`
}

type TicketConfig struct {
	GuildID           string `json:"guild_id"`
	PanelChannelID    string `json:"panel_channel_id"`
	PanelMessageID    string `json:"panel_message_id"`
	TicketCategoryID  string `json:"ticket_category_id"`
	ArchiveCategoryID string `json:"archive_category_id"`
	StaffRoleID       string `json:"staff_role_id"`
	LogChannelID      string `json:"log_channel_id"`
	TicketCount       int    `json:"ticket_count"`
}

type TicketMessageAttachment struct {
	URL         string `json:"url"`
	ProxyURL    string `json:"proxy_url,omitempty"`
	Filename    string `json:"filename"`
	Size        int    `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type TicketMessageRecord struct {
	TicketChannelID string                    `json:"ticket_channel_id"`
	MessageID       string                    `json:"message_id"`
	AuthorID        string                    `json:"author_id"`
	AuthorName      string                    `json:"author_name"`
	Content         string                    `json:"content"`
	Attachments     []TicketMessageAttachment `json:"attachments"`
	EmbedsJSON      string                    `json:"embeds_json,omitempty"`
	SentAt          time.Time                 `json:"sent_at"`
}

type ExpiredTicketRecord struct {
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
}

type UserLevel struct {
	GuildID string
	UserID  string
	XP      int
	Level   int
}

type WordChainChannel struct {
	ChannelID     string   `json:"channel_id"`
	GuildID       string   `json:"guild_id"`
	IsActive      bool     `json:"is_active"`
	AllowSolo     bool     `json:"allow_solo"`
	CurrentWord   string   `json:"current_word"`
	LastUserID    string   `json:"last_user_id"`
	CurrentStreak int      `json:"current_streak"`
	HighestStreak int      `json:"highest_streak"`
	TotalWords    int      `json:"total_words"`
	UsedWords     []string `json:"used_words"`
}

type WordChainUserStat struct {
	GuildID    string `json:"guild_id"`
	UserID     string `json:"user_id"`
	Score      int    `json:"score"`
	WordsCount int    `json:"words_count"`
	BestStreak int    `json:"best_streak"`
	WrongCount int    `json:"wrong_count"`
}

type VideoSubscription struct {
	ID          int       `json:"id"`
	GuildID     string    `json:"guild_id"`
	ChannelID   string    `json:"channel_id"`
	Platform    string    `json:"platform"`
	TargetID    string    `json:"target_id"`
	TargetURL   string    `json:"target_url"`
	TargetName  string    `json:"target_name"`
	PingRoleID  string    `json:"ping_role_id"`
	LastVideoID string    `json:"last_video_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type MySQLStore struct {
	db   *sql.DB
	lock sync.RWMutex
}

func OpenMySQL(cfg config.MySQLConfig) (*MySQLStore, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 3306
	}
	if cfg.User == "" {
		cfg.User = "root"
	}
	if cfg.Database == "" {
		cfg.Database = "botdis"
	}

	rootDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port)

	initDB, err := sql.Open("mysql", rootDSN)
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối tới MySQL server: %w", err)
	}

	createDBQuery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", cfg.Database)
	if _, err := initDB.Exec(createDBQuery); err != nil {
		initDB.Close()
		return nil, fmt.Errorf("không thể tạo database `%s`: %w", cfg.Database, err)
	}
	initDB.Close()

	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dbDSN)
	if err != nil {
		return nil, fmt.Errorf("lỗi mở kết nối database: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("không thể ping MySQL: %w", err)
	}

	store := &MySQLStore{db: db}

	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("lỗi khởi tạo bảng MySQL: %w", err)
	}

	go store.MigrateLegacyData()

	return store, nil
}

func (m *MySQLStore) createTables() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS guild_configs (
			guild_id VARCHAR(32) PRIMARY KEY,
			as_fast BOOLEAN DEFAULT 0,
			as_dup BOOLEAN DEFAULT 0,
			auto_join_role_id VARCHAR(32) DEFAULT '',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS channel_settings (
			guild_id VARCHAR(32) NOT NULL,
			channel_id VARCHAR(32) NOT NULL,
			ac_link BOOLEAN DEFAULT 0,
			ac_media BOOLEAN DEFAULT 0,
			ac_file BOOLEAN DEFAULT 0,
			ac_text BOOLEAN DEFAULT 0,
			level_enabled BOOLEAN DEFAULT 0,
			tiktok_enabled BOOLEAN DEFAULT 0,
			ai_enabled BOOLEAN DEFAULT 0,
			sticky_content TEXT,
			sticky_last_id VARCHAR(32),
			PRIMARY KEY (guild_id, channel_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS users_level (
			guild_id VARCHAR(32) NOT NULL,
			user_id VARCHAR(32) NOT NULL,
			xp INT DEFAULT 0,
			level INT DEFAULT 1,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (guild_id, user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS user_warnings (
			id INT AUTO_INCREMENT PRIMARY KEY,
			guild_id VARCHAR(32) NOT NULL,
			user_id VARCHAR(32) NOT NULL,
			reason VARCHAR(255) NOT NULL,
			step INT DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_user (guild_id, user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS ticket_configs (
			guild_id VARCHAR(32) PRIMARY KEY,
			panel_channel_id VARCHAR(32) DEFAULT '',
			panel_message_id VARCHAR(32) DEFAULT '',
			ticket_category_id VARCHAR(32) DEFAULT '',
			archive_category_id VARCHAR(32) DEFAULT '',
			staff_role_id VARCHAR(32) DEFAULT '',
			log_channel_id VARCHAR(32) DEFAULT '',
			ticket_count INT DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS tickets (
			channel_id VARCHAR(32) PRIMARY KEY,
			guild_id VARCHAR(32) NOT NULL,
			owner_id VARCHAR(32) NOT NULL,
			owner_name VARCHAR(100) NOT NULL,
			status VARCHAR(20) DEFAULT 'open',
			claimed_by VARCHAR(32) DEFAULT '',
			ticket_number INT DEFAULT 0,
			channel_name VARCHAR(100) NOT NULL,
			close_reason VARCHAR(255) DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			closed_at TIMESTAMP NULL,
			INDEX idx_guild_owner (guild_id, owner_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS ticket_logs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			guild_id VARCHAR(32) NOT NULL,
			ticket_channel_id VARCHAR(32) NOT NULL,
			action VARCHAR(100) NOT NULL,
			user_id VARCHAR(32) NOT NULL,
			target_id VARCHAR(32) DEFAULT '',
			reason VARCHAR(255) DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS wordchain_channels (
			channel_id VARCHAR(32) PRIMARY KEY,
			guild_id VARCHAR(32) NOT NULL,
			is_active BOOLEAN DEFAULT 1,
			allow_solo BOOLEAN DEFAULT 0,
			current_word VARCHAR(100) DEFAULT '',
			last_user_id VARCHAR(32) DEFAULT '',
			current_streak INT DEFAULT 0,
			highest_streak INT DEFAULT 0,
			total_words INT DEFAULT 0,
			used_words LONGTEXT,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_guild (guild_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS wordchain_stats (
			guild_id VARCHAR(32) NOT NULL,
			user_id VARCHAR(32) NOT NULL,
			score INT DEFAULT 0,
			words_count INT DEFAULT 0,
			best_streak INT DEFAULT 0,
			wrong_count INT DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (guild_id, user_id),
			INDEX idx_score (guild_id, score DESC)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS video_subscriptions (
			id INT AUTO_INCREMENT PRIMARY KEY,
			guild_id VARCHAR(32) NOT NULL,
			channel_id VARCHAR(32) NOT NULL,
			platform VARCHAR(16) NOT NULL,
			target_id VARCHAR(128) NOT NULL,
			target_url VARCHAR(255) NOT NULL,
			target_name VARCHAR(128) DEFAULT '',
			ping_role_id VARCHAR(32) DEFAULT '',
			last_video_id VARCHAR(128) DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_platform (platform),
			INDEX idx_guild_channel (guild_id, channel_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		`CREATE TABLE IF NOT EXISTS ticket_messages (
			id INT AUTO_INCREMENT PRIMARY KEY,
			ticket_channel_id VARCHAR(32) NOT NULL,
			message_id VARCHAR(32) NOT NULL,
			author_id VARCHAR(32) NOT NULL,
			author_name VARCHAR(100) NOT NULL,
			content TEXT,
			attachments JSON,
			embeds JSON,
			sent_at DATETIME,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_ticket_channel (ticket_channel_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
	}

	for _, q := range queries {
		if _, err := m.db.Exec(q); err != nil {
			return err
		}
	}

	_, _ = m.db.Exec("ALTER TABLE guild_configs ADD COLUMN auto_join_delay_seconds INT DEFAULT 0;")
	_, _ = m.db.Exec("ALTER TABLE tickets ADD COLUMN scheduled_delete_at TIMESTAMP NULL;")
	_, _ = m.db.Exec("ALTER TABLE ticket_configs ADD COLUMN archive_category_id VARCHAR(32) DEFAULT '';")

	return nil
}

func (m *MySQLStore) GetChannelSettings(guildID, channelID string) ChannelSettings {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var s ChannelSettings
	var stickyContent, stickyLastID sql.NullString

	var asFast, asDup bool
	_ = m.db.QueryRow("SELECT as_fast, as_dup FROM guild_configs WHERE guild_id = ?", guildID).Scan(&asFast, &asDup)

	err := m.db.QueryRow(`
		SELECT ac_link, ac_media, ac_file, ac_text, level_enabled, tiktok_enabled, ai_enabled, sticky_content, sticky_last_id
		FROM channel_settings WHERE guild_id = ? AND channel_id = ?
	`, guildID, channelID).Scan(
		&s.ACLink, &s.ACMedia, &s.ACFile, &s.ACText,
		&s.LevelEnabled, &s.TikTokEnabled, &s.AIEnabled,
		&stickyContent, &stickyLastID,
	)

	if err != nil {
		return ChannelSettings{ASFast: asFast, ASDup: asDup}
	}

	s.ASFast = asFast
	s.ASDup = asDup
	if stickyContent.Valid {
		s.StickyContent = stickyContent.String
	}
	if stickyLastID.Valid {
		s.StickyLastID = stickyLastID.String
	}
	return s
}

func (m *MySQLStore) UpdateChannelSetting(guildID, channelID string, updateFn func(*ChannelSettings)) {
	current := m.GetChannelSettings(guildID, channelID)
	updateFn(&current)

	m.lock.Lock()
	defer m.lock.Unlock()

	_, _ = m.db.Exec(`
		INSERT INTO channel_settings (guild_id, channel_id, ac_link, ac_media, ac_file, ac_text, level_enabled, tiktok_enabled, ai_enabled, sticky_content, sticky_last_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			ac_link = VALUES(ac_link),
			ac_media = VALUES(ac_media),
			ac_file = VALUES(ac_file),
			ac_text = VALUES(ac_text),
			level_enabled = VALUES(level_enabled),
			tiktok_enabled = VALUES(tiktok_enabled),
			ai_enabled = VALUES(ai_enabled),
			sticky_content = VALUES(sticky_content),
			sticky_last_id = VALUES(sticky_last_id);
	`, guildID, channelID, current.ACLink, current.ACMedia, current.ACFile, current.ACText,
		current.LevelEnabled, current.TikTokEnabled, current.AIEnabled, current.StickyContent, current.StickyLastID)
}

func (m *MySQLStore) SetGuildAntiSpam(guildID string, asFast, asDup bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, _ = m.db.Exec(`
		INSERT INTO guild_configs (guild_id, as_fast, as_dup)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE as_fast = VALUES(as_fast), as_dup = VALUES(as_dup);
	`, guildID, asFast, asDup)
}

func (m *MySQLStore) GetGuildData(guildID string) *GuildData {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var asFast, asDup bool
	var autoRole string
	var autoDelay int
	_ = m.db.QueryRow("SELECT as_fast, as_dup, auto_join_role_id, COALESCE(auto_join_delay_seconds, 0) FROM guild_configs WHERE guild_id = ?", guildID).Scan(&asFast, &asDup, &autoRole, &autoDelay)

	return &GuildData{
		ASFast:        asFast,
		ASDup:         asDup,
		AutoJoinRoles: map[string]string{"default": autoRole},
		AutoJoinDelay: autoDelay,
		Settings:      make(map[string]*ChannelSettings),
	}
}

func (m *MySQLStore) SetAutoRole(guildID, roleID string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, _ = m.db.Exec(`
		INSERT INTO guild_configs (guild_id, auto_join_role_id)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE auto_join_role_id = VALUES(auto_join_role_id);
	`, guildID, roleID)
}

func (m *MySQLStore) ClearAutoRole(guildID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec("UPDATE guild_configs SET auto_join_role_id = '' WHERE guild_id = ?", guildID)
	return err
}

func (m *MySQLStore) SetAutoRoleDelay(guildID string, delaySec int) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO guild_configs (guild_id, auto_join_delay_seconds)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE auto_join_delay_seconds = VALUES(auto_join_delay_seconds);
	`, guildID, delaySec)
	return err
}

func (m *MySQLStore) GetUserLevel(guildID, userID string) (*UserLevel, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var xp, level int
	err := m.db.QueryRow("SELECT xp, level FROM users_level WHERE guild_id = ? AND user_id = ?", guildID, userID).Scan(&xp, &level)
	if err != nil {
		if err == sql.ErrNoRows {
			return &UserLevel{GuildID: guildID, UserID: userID, XP: 0, Level: 1}, nil
		}
		return nil, err
	}
	return &UserLevel{GuildID: guildID, UserID: userID, XP: xp, Level: level}, nil
}

func (m *MySQLStore) AddUserXP(guildID, userID string, amount int) (newXP, newLevel int, leveledUp bool, err error) {
	current, err := m.GetUserLevel(guildID, userID)
	if err != nil {
		return 0, 0, false, err
	}

	newXP = current.XP + amount
	newLevel = current.Level
	leveledUp = false

	xpNeeded := newLevel * 100
	for newXP >= xpNeeded {
		newXP -= xpNeeded
		newLevel++
		leveledUp = true
		xpNeeded = newLevel * 100
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	_, err = m.db.Exec(`
		INSERT INTO users_level (guild_id, user_id, xp, level)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE xp = VALUES(xp), level = VALUES(level);
	`, guildID, userID, newXP, newLevel)

	return newXP, newLevel, leveledUp, err
}

func (m *MySQLStore) CreateTicket(channelID, guildID, ownerID, ownerName, channelName string, ticketNumber int) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO tickets (channel_id, guild_id, owner_id, owner_name, channel_name, ticket_number, status)
		VALUES (?, ?, ?, ?, ?, ?, 'open');
	`, channelID, guildID, ownerID, ownerName, channelName, ticketNumber)
	return err
}

func (m *MySQLStore) CloseTicket(channelID, reason string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec(`
		UPDATE tickets SET status = 'closed', close_reason = ?, closed_at = NOW()
		WHERE channel_id = ?;
	`, reason, channelID)
	return err
}

func (m *MySQLStore) GetActiveTicket(guildID, ownerID string) (channelID string, found bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	err := m.db.QueryRow(`
		SELECT channel_id FROM tickets
		WHERE guild_id = ? AND owner_id = ? AND status = 'open'
		LIMIT 1;
	`, guildID, ownerID).Scan(&channelID)

	return channelID, err == nil
}

func (m *MySQLStore) GetTicketConfig(guildID string) *TicketConfig {
	m.lock.RLock()
	defer m.lock.RUnlock()

	cfg := &TicketConfig{GuildID: guildID}
	_ = m.db.QueryRow(`
		SELECT panel_channel_id, panel_message_id, ticket_category_id, COALESCE(archive_category_id, ''), staff_role_id, log_channel_id, ticket_count
		FROM ticket_configs WHERE guild_id = ?
	`, guildID).Scan(&cfg.PanelChannelID, &cfg.PanelMessageID, &cfg.TicketCategoryID, &cfg.ArchiveCategoryID, &cfg.StaffRoleID, &cfg.LogChannelID, &cfg.TicketCount)
	return cfg
}

func (m *MySQLStore) SetTicketStaffRole(guildID, roleID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO ticket_configs (guild_id, staff_role_id)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE staff_role_id = VALUES(staff_role_id);
	`, guildID, roleID)
	return err
}

func (m *MySQLStore) SetTicketCategory(guildID, categoryID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO ticket_configs (guild_id, ticket_category_id)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE ticket_category_id = VALUES(ticket_category_id);
	`, guildID, categoryID)
	return err
}

func (m *MySQLStore) SetTicketArchiveCategory(guildID, categoryID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO ticket_configs (guild_id, archive_category_id)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE archive_category_id = VALUES(archive_category_id);
	`, guildID, categoryID)
	return err
}

func (m *MySQLStore) IncrementTicketCount(guildID string) int {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, _ = m.db.Exec(`
		INSERT INTO ticket_configs (guild_id, ticket_count)
		VALUES (?, 1)
		ON DUPLICATE KEY UPDATE ticket_count = ticket_count + 1;
	`, guildID)

	var count int
	_ = m.db.QueryRow("SELECT ticket_count FROM ticket_configs WHERE guild_id = ?", guildID).Scan(&count)
	if count <= 0 {
		count = 1
	}
	return count
}

func (m *MySQLStore) FindTicketByChannel(channelID string) (ownerID string, found bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	err := m.db.QueryRow("SELECT owner_id FROM tickets WHERE channel_id = ? LIMIT 1", channelID).Scan(&ownerID)
	return ownerID, err == nil
}

func (m *MySQLStore) LogTicketAction(guildID, channelID, action, userID, targetID, reason string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, _ = m.db.Exec(`
		INSERT INTO ticket_logs (guild_id, ticket_channel_id, action, user_id, target_id, reason)
		VALUES (?, ?, ?, ?, ?, ?);
	`, guildID, channelID, action, userID, targetID, reason)
}

func (m *MySQLStore) SaveTicketMessages(channelID string, msgs []TicketMessageRecord) error {
	if len(msgs) == 0 {
		return nil
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO ticket_messages (ticket_channel_id, message_id, author_id, author_name, content, attachments, embeds, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, msg := range msgs {
		attJSON, _ := json.Marshal(msg.Attachments)
		if len(msg.Attachments) == 0 {
			attJSON = []byte("[]")
		}
		embedsVal := msg.EmbedsJSON
		if embedsVal == "" {
			embedsVal = "[]"
		}

		_, err = stmt.Exec(
			channelID,
			msg.MessageID,
			msg.AuthorID,
			msg.AuthorName,
			msg.Content,
			string(attJSON),
			embedsVal,
			msg.SentAt,
		)
		if err != nil {
			log.Printf("Lỗi lưu message ticket %s: %v", msg.MessageID, err)
		}
	}

	return tx.Commit()
}

func (m *MySQLStore) CloseTicketWithSchedule(channelID, reason string, days int) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec(`
		UPDATE tickets 
		SET status = 'closed', close_reason = ?, closed_at = NOW(), scheduled_delete_at = DATE_ADD(NOW(), INTERVAL ? DAY)
		WHERE channel_id = ?;
	`, reason, days, channelID)
	return err
}

func (m *MySQLStore) GetExpiredTickets() ([]ExpiredTicketRecord, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	rows, err := m.db.Query(`
		SELECT channel_id, guild_id 
		FROM tickets 
		WHERE status = 'closed' AND scheduled_delete_at IS NOT NULL AND scheduled_delete_at <= NOW()
		LIMIT 50;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ExpiredTicketRecord
	for rows.Next() {
		var rec ExpiredTicketRecord
		if err := rows.Scan(&rec.ChannelID, &rec.GuildID); err == nil {
			result = append(result, rec)
		}
	}
	return result, nil
}

func (m *MySQLStore) MarkTicketDeleted(channelID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec("UPDATE tickets SET status = 'deleted' WHERE channel_id = ?", channelID)
	return err
}

func (m *MySQLStore) MigrateLegacyData() {
	log.Println("Đang kiểm tra và tự động di trú dữ liệu cũ sang MySQL...")

	possiblePaths := []string{"data.json", "../data.json", filepath.Join(".", "data.json")}
	for _, p := range possiblePaths {
		if data, err := os.ReadFile(p); err == nil {
			var raw map[string]*GuildData
			if err := json.Unmarshal(data, &raw); err == nil {
				for gid, gd := range raw {
					m.SetGuildAntiSpam(gid, gd.ASFast, gd.ASDup)
					if role, ok := gd.AutoJoinRoles["default"]; ok && role != "" {
						m.SetAutoRole(gid, role)
					}
					for cid, cs := range gd.Settings {
						if cs != nil {
							m.UpdateChannelSetting(gid, cid, func(target *ChannelSettings) {
								*target = *cs
							})
						}
					}
				}
				log.Println("Đã hoàn tất di trú dữ liệu từ data.json vào MySQL!")
			}
			break
		}
	}
}

func (m *MySQLStore) Close() error {
	return m.db.Close()
}

func (m *MySQLStore) GetWordChainChannel(channelID string) (*WordChainChannel, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var ch WordChainChannel
	var usedWordsJSON sql.NullString
	query := `SELECT channel_id, guild_id, is_active, allow_solo, current_word, last_user_id, current_streak, highest_streak, total_words, used_words FROM wordchain_channels WHERE channel_id = ?`
	row := m.db.QueryRow(query, channelID)
	err := row.Scan(&ch.ChannelID, &ch.GuildID, &ch.IsActive, &ch.AllowSolo, &ch.CurrentWord, &ch.LastUserID, &ch.CurrentStreak, &ch.HighestStreak, &ch.TotalWords, &usedWordsJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if usedWordsJSON.Valid && usedWordsJSON.String != "" {
		_ = json.Unmarshal([]byte(usedWordsJSON.String), &ch.UsedWords)
	}
	if ch.UsedWords == nil {
		ch.UsedWords = []string{}
	}

	return &ch, nil
}

func (m *MySQLStore) SaveWordChainChannel(ch *WordChainChannel) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	usedJSON, err := json.Marshal(ch.UsedWords)
	if err != nil {
		usedJSON = []byte("[]")
	}

	query := `
		INSERT INTO wordchain_channels (channel_id, guild_id, is_active, allow_solo, current_word, last_user_id, current_streak, highest_streak, total_words, used_words)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			is_active = VALUES(is_active),
			allow_solo = VALUES(allow_solo),
			current_word = VALUES(current_word),
			last_user_id = VALUES(last_user_id),
			current_streak = VALUES(current_streak),
			highest_streak = VALUES(highest_streak),
			total_words = VALUES(total_words),
			used_words = VALUES(used_words);
	`
	_, err = m.db.Exec(query, ch.ChannelID, ch.GuildID, ch.IsActive, ch.AllowSolo, ch.CurrentWord, ch.LastUserID, ch.CurrentStreak, ch.HighestStreak, ch.TotalWords, string(usedJSON))
	return err
}

func (m *MySQLStore) GetActiveWordChainChannelIDs(guildID string) ([]string, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	query := `SELECT channel_id FROM wordchain_channels WHERE guild_id = ? AND is_active = 1`
	rows, err := m.db.Query(query, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channelIDs []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err == nil {
			channelIDs = append(channelIDs, cid)
		}
	}
	return channelIDs, nil
}

func (m *MySQLStore) AddWordChainScore(guildID, userID string, scoreGain int, isCorrect bool, currentStreak int) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	var wordsInc, wrongInc int
	if isCorrect {
		wordsInc = 1
	} else {
		wrongInc = 1
	}

	query := `
		INSERT INTO wordchain_stats (guild_id, user_id, score, words_count, best_streak, wrong_count)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			score = score + VALUES(score),
			words_count = words_count + VALUES(words_count),
			wrong_count = wrong_count + VALUES(wrong_count),
			best_streak = GREATEST(best_streak, VALUES(best_streak));
	`
	_, err := m.db.Exec(query, guildID, userID, scoreGain, wordsInc, currentStreak, wrongInc)
	return err
}

func (m *MySQLStore) GetWordChainLeaderboard(guildID string, limit int) ([]WordChainUserStat, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	query := `SELECT guild_id, user_id, score, words_count, best_streak, wrong_count FROM wordchain_stats WHERE guild_id = ? ORDER BY score DESC, words_count DESC LIMIT ?`
	rows, err := m.db.Query(query, guildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WordChainUserStat
	for rows.Next() {
		var stat WordChainUserStat
		if err := rows.Scan(&stat.GuildID, &stat.UserID, &stat.Score, &stat.WordsCount, &stat.BestStreak, &stat.WrongCount); err == nil {
			list = append(list, stat)
		}
	}
	return list, nil
}

func (m *MySQLStore) GetWordChainUserStat(guildID, userID string) (*WordChainUserStat, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var stat WordChainUserStat
	query := `SELECT guild_id, user_id, score, words_count, best_streak, wrong_count FROM wordchain_stats WHERE guild_id = ? AND user_id = ?`
	row := m.db.QueryRow(query, guildID, userID)
	err := row.Scan(&stat.GuildID, &stat.UserID, &stat.Score, &stat.WordsCount, &stat.BestStreak, &stat.WrongCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return &WordChainUserStat{
				GuildID: guildID,
				UserID:  userID,
			}, nil
		}
		return nil, err
	}
	return &stat, nil
}

func (m *MySQLStore) AddVideoSubscription(sub *VideoSubscription) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	query := `INSERT INTO video_subscriptions 
		(guild_id, channel_id, platform, target_id, target_url, target_name, ping_role_id, last_video_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := m.db.Exec(query, sub.GuildID, sub.ChannelID, sub.Platform, sub.TargetID, sub.TargetURL, sub.TargetName, sub.PingRoleID, sub.LastVideoID)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		sub.ID = int(id)
	}
	return nil
}

func (m *MySQLStore) GetVideoSubscriptionsByGuild(guildID string) ([]*VideoSubscription, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	query := `SELECT id, guild_id, channel_id, platform, target_id, target_url, target_name, ping_role_id, last_video_id, created_at
		FROM video_subscriptions WHERE guild_id = ? ORDER BY id DESC`
	rows, err := m.db.Query(query, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*VideoSubscription
	for rows.Next() {
		var s VideoSubscription
		if err := rows.Scan(&s.ID, &s.GuildID, &s.ChannelID, &s.Platform, &s.TargetID, &s.TargetURL, &s.TargetName, &s.PingRoleID, &s.LastVideoID, &s.CreatedAt); err != nil {
			continue
		}
		list = append(list, &s)
	}
	return list, nil
}

func (m *MySQLStore) GetVideoSubscriptionsByChannel(guildID, channelID string) ([]*VideoSubscription, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	query := `SELECT id, guild_id, channel_id, platform, target_id, target_url, target_name, ping_role_id, last_video_id, created_at
		FROM video_subscriptions WHERE guild_id = ? AND channel_id = ? ORDER BY id DESC`
	rows, err := m.db.Query(query, guildID, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*VideoSubscription
	for rows.Next() {
		var s VideoSubscription
		if err := rows.Scan(&s.ID, &s.GuildID, &s.ChannelID, &s.Platform, &s.TargetID, &s.TargetURL, &s.TargetName, &s.PingRoleID, &s.LastVideoID, &s.CreatedAt); err != nil {
			continue
		}
		list = append(list, &s)
	}
	return list, nil
}

func (m *MySQLStore) DeleteVideoSubscription(id int) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec("DELETE FROM video_subscriptions WHERE id = ?", id)
	return err
}

func (m *MySQLStore) GetAllVideoSubscriptions() ([]*VideoSubscription, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	query := `SELECT id, guild_id, channel_id, platform, target_id, target_url, target_name, ping_role_id, last_video_id, created_at
		FROM video_subscriptions ORDER BY id ASC`
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*VideoSubscription
	for rows.Next() {
		var s VideoSubscription
		if err := rows.Scan(&s.ID, &s.GuildID, &s.ChannelID, &s.Platform, &s.TargetID, &s.TargetURL, &s.TargetName, &s.PingRoleID, &s.LastVideoID, &s.CreatedAt); err != nil {
			continue
		}
		list = append(list, &s)
	}
	return list, nil
}

func (m *MySQLStore) UpdateLastVideoID(id int, lastVideoID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, err := m.db.Exec("UPDATE video_subscriptions SET last_video_id = ? WHERE id = ?", lastVideoID, id)
	return err
}

