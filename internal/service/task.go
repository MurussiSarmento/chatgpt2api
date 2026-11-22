package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/basketikun/chatgpt2api/internal/config"
	"github.com/basketikun/chatgpt2api/internal/repository"
	upstream "github.com/basketikun/chatgpt2api/internal/upstream"
)

const maxActiveTasks = 10

var ErrTooManyTasks = errors.New("too many tasks in progress; please wait for some to finish before submitting more")

type TaskService struct {
	repo     *repository.Repository
	ai       *AIService
	veta     *VetaService
	artistly *ArtistlyService
	config   *config.Config
}

func NewTaskService(repo *repository.Repository, ai *AIService, veta *VetaService, artistly *ArtistlyService, config *config.Config) *TaskService {
	return &TaskService{repo: repo, ai: ai, veta: veta, artistly: artistly, config: config}
}

func (s *TaskService) List(ctx context.Context, ownerID string, ids []int64, types []string) ([]repository.Task, []int64, error) {
	if err := s.CleanupExpired(ctx); err != nil {
		return nil, nil, err
	}
	items, err := s.repo.Tasks(ctx, ownerID, ids, types)
	if err != nil {
		return nil, nil, err
	}
	found := map[int64]bool{}
	for _, item := range items {
		found[item.ID] = true
	}
	missing := []int64{}
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	return items, missing, nil
}

func (s *TaskService) SubmitImage(ctx context.Context, identity Identity, request ImageRequest, historyID *int64, turnID *int64, kind string) (repository.Task, error) {
	if request.Model == "" || request.Model == "auto" {
		request.Model = s.config.ImageModelName
	}
	active, err := s.repo.CountActiveTasks(ctx, identity.ID)
	if err != nil {
		return repository.Task{}, err
	}
	if active >= maxActiveTasks {
		return repository.Task{}, ErrTooManyTasks
	}
	if s.config.BaseURL != "" {
		request.BaseURL = s.config.BaseURL
	}
	taskTitle := title(request.Prompt)
	if historyID != nil {
		if history, err := s.repo.Task(ctx, identity.ID, *historyID); err == nil {
			taskTitle = history.Title
		}
	}
	now := time.Now().UTC()
	item := repository.Task{OwnerID: identity.ID, HistoryID: historyID, TurnID: turnID, Title: taskTitle, Type: kind, Status: "queued", Prompt: request.Prompt, Model: request.Model, Size: request.Size, Quality: request.Quality, OutputCount: request.Count, BaseURL: request.BaseURL, CreatedAt: now, UpdatedAt: now}
	item, err = s.repo.SaveTask(ctx, item)
	if err != nil {
		return item, err
	}
	if item.HistoryID == nil {
		item.HistoryID = &item.ID
	}
	if item.TurnID == nil {
		item.TurnID = &item.ID
	}
	item, err = s.repo.SaveTask(ctx, item)
	if err == nil {
		go s.runImage(identity, item, request)
	}
	return item, err
}

func (s *TaskService) runImage(identity Identity, task repository.Task, request ImageRequest) {
	ctx := context.Background()
	now := time.Now().UTC()
	task.Status, task.Progress, task.StartedAt, task.UpdatedAt = "running", "", &now, now
	s.repo.SaveTask(ctx, task)
	started := time.Now()
	callLog := s.ai.logs.NewCallLog(identity, map[string]string{"image_generation": "/api/tasks/image-generation", "image_edit": "/api/tasks/image-edit"}[task.Type], task.Model, task.Type+" ", task.Prompt, started)

	// Dispatch Veta and Artistly models before falling through to the standard
	// ChatGPT image service, which would reject them as unsupported.
	var results []ImageResult
	var err error
	switch {
	case s.veta != nil && isVetaImageModel(request.Model):
		result, vetaErr := s.veta.Image(ctx, request.Prompt, vetaImageModel(request.Model), aspectFromSize(request.Size), "", request.BaseURL)
		if vetaErr != nil {
			err = vetaErr
		} else {
			results = []ImageResult{result}
		}
	case s.artistly != nil && IsArtistlyModel(request.Model):
		result, artistlyErr := s.artistly.Image(ctx, request.Prompt, request.Model, request.Quality, artistlyAspectFromSize(request.Size), request.Count, request.BaseURL)
		if artistlyErr != nil {
			err = artistlyErr
		} else {
			results = []ImageResult{result}
		}
	default:
		results, err = s.ai.Images(ctx, request, callLog)
	}
	ended := time.Now().UTC()
	duration := int(time.Since(started).Milliseconds())
	task.EndedAt, task.UpdatedAt, task.DurationMS = &ended, ended, &duration
	if err != nil {
		var imageErr *ImageError
		if errors.As(err, &imageErr) {
			task.ConversationID = imageErr.ConversationID
			task.AccountEmail = imageErr.AccountEmail
		}
		task.Status, task.Error, task.Progress = "error", err.Error(), ""
		s.repo.SaveTask(ctx, task)
		return
	}
	output := make([]repository.TaskResult, 0, len(results))
	for index, result := range results {
		output = append(output, repository.TaskResult{TaskID: task.ID, Position: index, Type: "image", Name: "image-" + strconv.Itoa(index+1) + ".png", URL: result.URL, RevisedPrompt: result.RevisedPrompt, ContentType: "image/png"})
	}
	s.repo.ReplaceTaskResults(ctx, task.ID, output)
	task.InputTokens, task.OutputTokens, task.TotalTokens = imageTaskUsage(request, results)
	task.Status, task.Progress = "success", ""
	s.repo.SaveTask(ctx, task)
}

func (s *TaskService) CleanupExpired(ctx context.Context) error {
	return s.repo.DeleteFinishedTasksBefore(ctx, time.Now().AddDate(0, 0, -s.config.Image.RetentionDays))
}
func title(value string) string {
	runes := []rune(value)
	if len(runes) > 24 {
		return string(runes[:24])
	}
	return value
}

// isVetaImageModel reports whether model should be routed to Veta IA.
// Keep in sync with the map in handler/ai.go and the catalog advertisement.
var vetaImageModels = map[string]bool{
	"nano_banana_pro":  true,
	"nano_banana_fast": true,
}

func isVetaImageModel(model string) bool { return vetaImageModels[model] }

func vetaImageModel(model string) string { return model }

// IsArtistlyModel delegates to the upstream package so the service layer does
// not need to maintain its own list.
func IsArtistlyModel(model string) bool { return upstream.IsArtistlyModel(model) }

// aspectFromSize maps an OpenAI-style WxH size to a Veta aspect ratio.
func aspectFromSize(size string) string {
	switch size {
	case "1024x1792", "1080x1920", "720x1280":
		return "9:16"
	case "1024x1024":
		return "1:1"
	default:
		return "16:9"
	}
}

// artistlyAspectFromSize maps an OpenAI-style WxH size to an Artistly aspect ratio.
func artistlyAspectFromSize(size string) string {
	switch size {
	case "1024x1792", "1080x1920", "720x1280":
		return "9:16"
	case "1024x1024":
		return "1:1"
	case "1792x1024", "1920x1080", "1280x720":
		return "16:9"
	case "1536x1024":
		return "3:2"
	case "1024x1536":
		return "2:3"
	default:
		return "1:1"
	}
}

