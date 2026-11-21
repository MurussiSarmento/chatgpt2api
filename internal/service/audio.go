package service

import (
	"context"
	"sync"
	"time"

	"github.com/basketikun/chatgpt2api/internal/config"
	chatgpt "github.com/basketikun/chatgpt2api/internal/upstream"
)

const speechCacheTTL = 10 * time.Minute
const speechCacheMaxEntries = 32
const speechCacheMaxBytes = 4 << 20

// AudioService exposes the ChatGPT web voice capabilities (TTS / STT) as standard OpenAI audio endpoints.
type AudioService struct {
	accounts *AccountService
	logs     *LogService
	proxy    *ProxyService
	config   *config.Config
	cacheMu  sync.Mutex
	cache    map[string]cachedSpeech
}

type cachedSpeech struct {
	Data        []byte
	ContentType string
	ExpiresAt   time.Time
}

func NewAudioService(accounts *AccountService, logs *LogService, proxy *ProxyService, config *config.Config) *AudioService {
	return &AudioService{accounts: accounts, logs: logs, proxy: proxy, config: config, cache: map[string]cachedSpeech{}}
}

// Synthesizing once requires a full round of upstream conversation, so there's no need to run it again for repeated synthesis of the same text.
func (s *AudioService) cachedSpeech(key string) ([]byte, string, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	item, ok := s.cache[key]
	if !ok {
		return nil, "", false
	}
	if time.Now().After(item.ExpiresAt) {
		delete(s.cache, key)
		return nil, "", false
	}
	return item.Data, item.ContentType, true
}

func (s *AudioService) storeSpeech(key string, data []byte, contentType string) {
	if len(data) == 0 || len(data) > speechCacheMaxBytes {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if len(s.cache) >= speechCacheMaxEntries {
		for oldest := range s.cache {
			delete(s.cache, oldest)
			break
		}
	}
	s.cache[key] = cachedSpeech{Data: data, ContentType: contentType, ExpiresAt: time.Now().Add(speechCacheTTL)}
}

type SpeechRequest struct {
	Input  string
	Voice  string
	Model  string
	Format string
}

// Speech synthesizes text into speech and returns the audio bytes and Content-Type.
func (s *AudioService) Speech(ctx context.Context, request SpeechRequest, callLog *CallLog) ([]byte, string, error) {
	voice, err := chatgpt.ResolveSpeechVoice(request.Voice)
	if err != nil {
		return nil, "", err
	}
	cacheKey := request.Input + "\x00" + voice + "\x00" + request.Format
	if data, contentType, ok := s.cachedSpeech(cacheKey); ok {
		callLog.Add("Speech cache hit", nil)
		return data, contentType, nil
	}
	token, err := s.accounts.TextTokenForModel(ctx, nil, nil, "auto")
	if err != nil {
		return nil, "", err
	}
	account, err := s.accounts.Account(ctx, token)
	if err != nil {
		return nil, "", err
	}
	if account.Email != nil {
		callLog.Update(map[string]any{"account_email": *account.Email})
	}
	callLog.Add("Upstream account selected", map[string]any{"account_email": account.Email})
	client := chatgpt.NewOpenAIBackendAPI(token, account.Proxy, s.proxy, s.config).WithAccountID(account.AccountID)
	started := time.Now()
	data, contentType, err := client.Speak(ctx, request.Input, voice, request.Format)
	if err != nil {
		return nil, "", err
	}
	callLog.Add("Upstream returned audio", map[string]any{"elapsed_ms": time.Since(started).Milliseconds(), "bytes": len(data)})
	s.storeSpeech(cacheKey, data, contentType)
	return data, contentType, nil
}

// Transcribe converts audio into text.
func (s *AudioService) Transcribe(ctx context.Context, data []byte, filename string, contentType string, callLog *CallLog) (string, error) {
	token, err := s.accounts.TextTokenForModel(ctx, nil, nil, "auto")
	if err != nil {
		return "", err
	}
	account, err := s.accounts.Account(ctx, token)
	if err != nil {
		return "", err
	}
	if account.Email != nil {
		callLog.Update(map[string]any{"account_email": *account.Email})
	}
	callLog.Add("Upstream account selected", map[string]any{"account_email": account.Email})
	client := chatgpt.NewOpenAIBackendAPI(token, account.Proxy, s.proxy, s.config).WithAccountID(account.AccountID)
	return client.Transcribe(ctx, data, filename, contentType)
}
