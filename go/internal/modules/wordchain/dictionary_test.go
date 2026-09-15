package wordchain

import (
	"strings"
	"testing"
)

func TestDictionary(t *testing.T) {
	dict, err := NewDictionary("../../../dictionary.db")
	if err != nil {
		t.Fatalf("Không thể mở dictionary.db: %v", err)
	}
	defer dict.Close()

	// 1. Kiểm tra từ hợp lệ
	if !dict.IsValidWord("học tập") {
		t.Errorf("Kỳ vọng 'học tập' là từ hợp lệ")
	}
	if !dict.IsValidWord("bác sĩ") {
		t.Errorf("Kỳ vọng 'bác sĩ' là từ hợp lệ")
	}

	// 2. Kiểm tra từ không tồn tại
	if dict.IsValidWord("tuxamlonxyz123") {
		t.Errorf("Kỳ vọng từ rác không tồn tại")
	}

	// 3. Kiểm tra tra cứu định nghĩa
	meanings, err := dict.GetDefinitions("bác sĩ")
	if err != nil {
		t.Fatalf("Lỗi lấy định nghĩa: %v", err)
	}
	if len(meanings) == 0 {
		t.Errorf("Kỳ vọng từ 'bác sĩ' có ít nhất 1 định nghĩa")
	}

	// 4. Kiểm tra tìm từ nối tiếp
	if !dict.HasNextWords("sĩ") {
		t.Errorf("Kỳ vọng chữ 'sĩ' có từ nối tiếp")
	}

	nextWords, err := dict.FindNextWords("sĩ", 3)
	if err != nil {
		t.Fatalf("Lỗi tìm từ nối tiếp: %v", err)
	}
	if len(nextWords) == 0 {
		t.Errorf("Kỳ vọng tìm được các từ bắt đầu bằng 'sĩ'")
	}

	// Kiểm tra độ dài các từ gợi ý cho 'giả' phải đúng 2 tiếng (không được có 'giả câm giả điếc' hay 'giả ba ba')
	giaWords, err := dict.FindNextWords("giả", 10)
	if err != nil {
		t.Fatalf("Lỗi tìm từ gợi ý cho 'giả': %v", err)
	}
	for _, gw := range giaWords {
		parts := strings.Fields(gw)
		if len(parts) != 2 {
			t.Errorf("Từ '%s' không đúng 2 tiếng (có %d tiếng)!", gw, len(parts))
		}
	}

	// 5. Kiểm tra bốc từ mở màn
	startWord, err := dict.GetRandomStartWord()
	if err != nil {
		t.Fatalf("Lỗi bốc từ mở màn: %v", err)
	}
	if startWord == "" {
		t.Errorf("Kỳ vọng từ mở màn không rỗng")
	}
}
