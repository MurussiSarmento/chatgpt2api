package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/basketikun/chatgpt2api/internal/repository"
	"github.com/google/uuid"
)

var ErrUnauthorized = errors.New("the key is invalid or has expired; please log in again")

type Identity struct {
	ID    string
	Name  string
	Role  string
	Plans  []string
	Routes []string
}

type AuthService struct {
	repo     *repository.Repository
	adminKey string
	mu       sync.RWMutex
	keys     map[string]cachedKey
}

type cachedKey struct {
	item      repository.APIKey
	expiresAt time.Time
}

func NewAuthService(repo *repository.Repository, adminKey string) *AuthService {
	return &AuthService{repo: repo, adminKey: adminKey, keys: map[string]cachedKey{}}
}

func (s *AuthService) apiKey(ctx context.Context, key string) (repository.APIKey, error) {
	s.mu.RLock()
	cached, ok := s.keys[key]
	s.mu.RUnlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.item, nil
	}
	item, err := s.repo.APIKeyByValue(ctx, key)
	if err != nil {
		return item, err
	}
	s.mu.Lock()
	s.keys[key] = cachedKey{item: item, expiresAt: time.Now().Add(30 * time.Second)}
	s.mu.Unlock()
	return item, nil
}

func (s *AuthService) clearKeys() {
	s.mu.Lock()
	clear(s.keys)
	s.mu.Unlock()
}

func (s *AuthService) Authenticate(ctx context.Context, authorization string, touch bool) (Identity, error) {
	scheme, key, ok := strings.Cut(authorization, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") || key == "" {
		return Identity{}, ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(key), []byte(s.adminKey)) == 1 {
		return Identity{ID: "admin", Name: "Administrator", Role: "admin", Routes: []string{"web"}}, nil
	}
	item, err := s.apiKey(ctx, key)
	if errors.Is(err, repository.ErrNotFound) {
		return Identity{}, ErrUnauthorized
	}
	if err != nil {
		return Identity{}, err
	}
	if touch {
		if err := s.repo.TouchAPIKey(ctx, item.ID, time.Now().UTC()); err != nil {
			return Identity{}, err
		}
	}
	return Identity{ID: item.ID, Name: item.Name, Role: item.Role, Plans: strings.Split(item.Plan, ","), Routes: strings.Split(item.Route, ",")}, nil
}

func (s *AuthService) List(ctx context.Context) ([]repository.APIKey, error) {
	return s.repo.APIKeys(ctx, "user")
}

func (s *AuthService) Create(ctx context.Context, name string, key string, plan []string, route []string) (repository.APIKey, error) {
	if key == "" {
		return repository.APIKey{}, errors.New("please enter a new dedicated key")
	}
	if key == s.adminKey {
		return repository.APIKey{}, errors.New("this key conflicts with the administrator key; please choose a different key")
	}
	exists, err := s.repo.APIKeyExists(ctx, key, "")
	if err != nil {
		return repository.APIKey{}, err
	}
	if exists {
		return repository.APIKey{}, errors.New("this dedicated key already exists; please choose a different key")
	}
	name, err = s.availableName(ctx, name, "")
	if err != nil {
		return repository.APIKey{}, err
	}
	item := repository.APIKey{ID: strings.ReplaceAll(uuid.NewString(), "-", ""), Name: name, Role: "user", Key: key, Plan: strings.Join(plan, ","), Route: strings.Join(route, ","), Enabled: true, CreatedAt: time.Now().UTC()}
	return item, s.repo.SaveAPIKey(ctx, item)
}

func (s *AuthService) Update(ctx context.Context, id string, name *string, enabled *bool, key *string, plan []string, route []string) (repository.APIKey, error) {
	item, err := s.repo.APIKeyByID(ctx, id)
	if err != nil {
		return repository.APIKey{}, err
	}
	if item.Role != "user" {
		return repository.APIKey{}, repository.ErrNotFound
	}
	if name != nil {
		item.Name, err = s.availableName(ctx, *name, id)
		if err != nil {
			return repository.APIKey{}, err
		}
	}
	if key != nil {
		if *key == "" || *key == s.adminKey {
			return repository.APIKey{}, errors.New("please enter a new key different from the administrator key")
		}
		exists, err := s.repo.APIKeyExists(ctx, *key, id)
		if err != nil {
			return repository.APIKey{}, err
		}
		if exists {
			return repository.APIKey{}, errors.New("this dedicated key already exists; please choose a different key")
		}
		item.Key = *key
	}
	if enabled != nil {
		item.Enabled = *enabled
	}
	if len(plan) > 0 {
		item.Plan = strings.Join(plan, ",")
	}
	if len(route) > 0 {
		item.Route = strings.Join(route, ",")
	}
	err = s.repo.SaveAPIKey(ctx, item)
	s.clearKeys()
	return item, err
}

func (s *AuthService) Delete(ctx context.Context, id string) error {
	item, err := s.repo.APIKeyByID(ctx, id)
	if err != nil {
		return err
	}
	if item.Role != "user" {
		return repository.ErrNotFound
	}
	err = s.repo.DeleteAPIKey(ctx, id)
	s.clearKeys()
	return err
}

func (s *AuthService) availableName(ctx context.Context, name string, excludeID string) (string, error) {
	if name != "" {
		exists, err := s.repo.APIKeyNameExists(ctx, name, "user", excludeID)
		if err != nil {
			return "", err
		}
		if exists {
			return "", errors.New("this name is already in use; please choose a more distinct name")
		}
		return name, nil
	}
	name = "User"
	for suffix := 2; ; suffix++ {
		exists, err := s.repo.APIKeyNameExists(ctx, name, "user", excludeID)
		if err != nil || !exists {
			return name, err
		}
		name = "User " + strconv.Itoa(suffix)
	}
}
