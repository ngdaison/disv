package chatai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"botdis/internal/storage"

	"github.com/bwmarrin/discordgo"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type Service struct {
	apiKey       string
	systemPrompt string
	store        *storage.MySQLStore
}

func NewService(apiKey string, store *storage.MySQLStore) *Service {
	possiblePaths := []string{"train.txt", "../train.txt", filepath.Join(".", "train.txt")}
	var prompt string
	for _, p := range possiblePaths {
		if data, err := os.ReadFile(p); err == nil {
			prompt = string(data)
			break
		}
	}

	return &Service{
		apiKey:       apiKey,
		systemPrompt: prompt,
		store:        store,
	}
}

func (s *Service) HandleMessage(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return
	}

	settings := s.store.GetChannelSettings(m.GuildID, m.ChannelID)
	isMentioned := false
	if sess.State != nil && sess.State.User != nil {
		for _, user := range m.Mentions {
			if user.ID == sess.State.User.ID {
				isMentioned = true
				break
			}
		}
	}

	if !settings.AIEnabled && !isMentioned {
		return
	}

	if s.apiKey == "" {
		return
	}

	go s.generateAndReply(sess, m)
}

type geminiRequest struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (s *Service) generateAndReply(sess *discordgo.Session, m *discordgo.MessageCreate) {
	_ = sess.ChannelTyping(m.ChannelID)

	cleanContent := m.Content
	if sess.State != nil && sess.State.User != nil {
		cleanContent = strings.ReplaceAll(cleanContent, fmt.Sprintf("<@%s>", sess.State.User.ID), "")
		cleanContent = strings.ReplaceAll(cleanContent, fmt.Sprintf("<@!%s>", sess.State.User.ID), "")
	}
	cleanContent = strings.TrimSpace(cleanContent)

	if cleanContent == "" {
		cleanContent = "Xin chào!"
	}

	userPrompt := cleanContent
	if s.systemPrompt != "" {
		userPrompt = s.systemPrompt + "\n\nNgười dùng hỏi:\n" + cleanContent
	}

	reqBody := geminiRequest{
		Contents: []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{
			{
				Role: "user",
				Parts: []struct {
					Text string `json:"text"`
				}{
					{Text: userPrompt},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=%s", s.apiKey)
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("Lỗi gọi Gemini API: %v", err)
		return
	}
	defer resp.Body.Close()

	var gResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return
	}

	replyText := gResp.Candidates[0].Content.Parts[0].Text

	for len(replyText) > 0 {
		chunkSize := 1950
		if len(replyText) < chunkSize {
			chunkSize = len(replyText)
		}
		chunk := replyText[:chunkSize]
		replyText = replyText[chunkSize:]

		_, _ = sess.ChannelMessageSendReply(m.ChannelID, chunk, m.Reference())
	}
}
