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
	RoleMenus     map[string][]string         `json:"role_menus,omitempty"`
	ASFast        bool                        `json:"as_fast"`
	ASDup         bool                        `json:"as_dup"`
}

type UserLevel struct {
	GuildID string
	UserID  string
	XP      int
	Level   int
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
	}

	for _, q := range queries {
		if _, err := m.db.Exec(q); err != nil {
			return err
		}
	}
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
	_ = m.db.QueryRow("SELECT as_fast, as_dup, auto_join_role_id FROM guild_configs WHERE guild_id = ?", guildID).Scan(&asFast, &asDup, &autoRole)

	return &GuildData{
		ASFast:        asFast,
		ASDup:         asDup,
		AutoJoinRoles: map[string]string{"default": autoRole},
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

func (m *MySQLStore) LogTicketAction(guildID, channelID, action, userID, targetID, reason string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, _ = m.db.Exec(`
		INSERT INTO ticket_logs (guild_id, ticket_channel_id, action, user_id, target_id, reason)
		VALUES (?, ?, ?, ?, ?, ?);
	`, guildID, channelID, action, userID, targetID, reason)
}

func (m *MySQLStore) MigrateLegacyData() {
	log.Println("🔍 Đang kiểm tra và tự động di trú dữ liệu cũ sang MySQL...")

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
				log.Println("✅ Đã hoàn tất di trú dữ liệu từ data.json vào MySQL!")
			}
			break
		}
	}
}

func (m *MySQLStore) Close() error {
	return m.db.Close()
}
