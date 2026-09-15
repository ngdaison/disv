package wordchain

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Dictionary struct {
	db   *sql.DB
	lock sync.RWMutex
	rnd  *rand.Rand
}

func NewDictionary(dbPath string) (*Dictionary, error) {
	// Tìm đường dẫn thực tế của dictionary.db bằng cách kiểm tra đường dẫn truyền vào hoặc duyệt ngược thư mục cha
	actualPath := dbPath
	found := false

	if dbPath != "" {
		if _, err := os.Stat(dbPath); err == nil {
			actualPath = dbPath
			found = true
		}
	}

	if !found {
		currDir, _ := os.Getwd()
		for i := 0; i < 6; i++ {
			candidate := filepath.Join(currDir, "dictionary.db")
			if _, err := os.Stat(candidate); err == nil {
				actualPath = candidate
				found = true
				break
			}
			parent := filepath.Dir(currDir)
			if parent == currDir {
				break
			}
			currDir = parent
		}
	}

	absPath, err := filepath.Abs(actualPath)
	if err == nil {
		actualPath = absPath
	}

	db, err := sql.Open("sqlite", actualPath)
	if err != nil {
		return nil, fmt.Errorf("không thể mở database từ điển (%s): %w", actualPath, err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("không thể ping database từ điển (%s): %w", actualPath, err)
	}

	return &Dictionary{
		db:  db,
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (d *Dictionary) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// IsValidWord kiểm tra xem từ có tồn tại trong từ điển tiếng Việt không
func (d *Dictionary) IsValidWord(word string) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()

	cleaned := strings.ToLower(strings.TrimSpace(word))
	var dummy int
	err := d.db.QueryRow("SELECT 1 FROM words WHERE word = ? AND lang_code = 'vi' LIMIT 1", cleaned).Scan(&dummy)
	return err == nil
}

// WordMeaning chứa nghĩa và loại từ
type WordMeaning struct {
	Definition string
	POS        string
}

// GetDefinitions lấy danh sách định nghĩa và từ loại của từ
func (d *Dictionary) GetDefinitions(word string) ([]WordMeaning, error) {
	d.lock.RLock()
	defer d.lock.RUnlock()

	cleaned := strings.ToLower(strings.TrimSpace(word))
	query := `
		SELECT COALESCE(d.definition, ''), COALESCE(d.pos, '')
		FROM words w
		JOIN word_definitions wd ON w.id = wd.word_id
		JOIN definitions d ON wd.definition_id = d.id
		WHERE w.word = ? AND w.lang_code = 'vi'
		LIMIT 5;
	`
	rows, err := d.db.Query(query, cleaned)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []WordMeaning
	for rows.Next() {
		var m WordMeaning
		if err := rows.Scan(&m.Definition, &m.POS); err == nil {
			results = append(results, m)
		}
	}
	return results, nil
}

// HasNextWords kiểm tra xem có từ nào đúng 2 tiếng bắt đầu bằng âm tiết chỉ định không
func (d *Dictionary) HasNextWords(lastSyllable string) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()

	syl := strings.ToLower(strings.TrimSpace(lastSyllable))
	pattern1 := syl + " %"
	pattern2 := syl + " % %"
	var dummy int
	err := d.db.QueryRow("SELECT 1 FROM words WHERE word LIKE ? AND word NOT LIKE ? AND lang_code = 'vi' LIMIT 1", pattern1, pattern2).Scan(&dummy)
	return err == nil
}

// FindNextWords gợi ý các từ đúng 2 tiếng bắt đầu bằng âm tiết chỉ định
func (d *Dictionary) FindNextWords(lastSyllable string, limit int) ([]string, error) {
	d.lock.RLock()
	defer d.lock.RUnlock()

	if limit <= 0 {
		limit = 5
	}

	syl := strings.ToLower(strings.TrimSpace(lastSyllable))
	pattern1 := syl + " %"
	pattern2 := syl + " % %"
	query := `SELECT word FROM words WHERE word LIKE ? AND word NOT LIKE ? AND lang_code = 'vi' LIMIT ?`
	rows, err := d.db.Query(query, pattern1, pattern2, limit*3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err == nil {
			if len(strings.Fields(w)) == 2 {
				words = append(words, w)
				if len(words) >= limit {
					break
				}
			}
		}
	}
	return words, nil
}

// GetRandomStartWord lấy ngẫu nhiên 1 từ 2 tiếng có từ nối tiếp để mở đầu ván
func (d *Dictionary) GetRandomStartWord() (string, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	// Danh sách các từ khởi đầu phổ biến, thú vị
	starterList := []string{
		"bắt đầu", "học tập", "thành công", "phát triển", "mặt trời",
		"tương lai", "hòa bình", "tự do", "hạnh phúc", "bác sĩ",
		"kinh tế", "khoa học", "công nghệ", "du lịch", "yêu thương",
		"sáng tạo", "thế giới", "nghệ thuật", "âm nhạc", "văn hóa",
		"gia đình", "bạn bè", "cuộc sống", "nỗ lực", "tri thức",
	}

	idx := d.rnd.Intn(len(starterList))
	return starterList[idx], nil
}
