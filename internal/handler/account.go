package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/basketikun/chatgpt2api/internal/repository"
	"github.com/basketikun/chatgpt2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountHandler struct{ service *service.AccountService }

func NewAccountHandler(service *service.AccountService) *AccountHandler {
	return &AccountHandler{service: service}
}

func (h *AccountHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/accounts", h.list)
	router.GET("/accounts/summary", h.summary)
	router.POST("/accounts", h.add)
	router.DELETE("/accounts", h.delete)
	router.POST("/accounts/update", h.update)
	router.POST("/accounts/quota/refresh", h.refreshQuota)
}

func (h *AccountHandler) list(c *gin.Context) {
	page, pageSize := intQuery(c.Query("page"), 1), intQuery(c.Query("page_size"), 10)
	items, total, types, err := h.service.List(c.Request.Context(), page, pageSize, c.Query("query"), c.Query("type"), c.Query("status"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	responses := make([]AccountResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, toAccountResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"items": responses, "total": total, "page": page, "page_size": pageSize, "types": types})
}

func (h *AccountHandler) summary(c *gin.Context) {
	result, err := h.service.Summary(c.Request.Context())
	writeResult(c, result, err)
}

func (h *AccountHandler) add(c *gin.Context) {
	var body []AccountRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, errors.New("invalid account data format"))
		return
	}
	items := make([]repository.Account, 0, len(body))
	for _, item := range body {
		items = append(items, accountFromRequest(item))
	}
	added, skipped, refreshed, failures, err := h.service.Add(c.Request.Context(), items)
	writeResult(c, gin.H{"added": added, "skipped": skipped, "refreshed": refreshed, "errors": failures}, err)
}

func (h *AccountHandler) delete(c *gin.Context) {
	var body DeleteAccountsRequest
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Tokens) == 0 {
		writeError(c, http.StatusBadRequest, errors.New("please select the account(s) to delete"))
		return
	}
	removed, err := h.service.Delete(c.Request.Context(), body.Tokens)
	writeResult(c, gin.H{"removed": removed}, err)
}

func (h *AccountHandler) update(c *gin.Context) {
	var body UpdateAccountRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, errors.New("please select the account(s) to modify"))
		return
	}
	values := map[string]any{}
	if body.Type != nil {
		values["type"] = *body.Type
	}
	if body.Status != nil {
		values["status"] = *body.Status
	}
	if body.Quota != nil {
		values["quota"] = *body.Quota
	}
	if body.Proxy != nil {
		values["proxy"] = *body.Proxy
	}
	if len(values) == 0 {
		writeError(c, http.StatusBadRequest, errors.New("no changes detected yet; please make changes before saving"))
		return
	}
	item, err := h.service.Update(c.Request.Context(), body.AccessToken, values)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(c, http.StatusNotFound, errors.New("account does not exist"))
		return
	}
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	items, _, _, err := h.service.List(c.Request.Context(), 1, 1000000, "", "", "")
	responses := make([]AccountResponse, 0, len(items))
	for _, account := range items {
		responses = append(responses, toAccountResponse(account))
	}
	writeResult(c, gin.H{"item": toAccountResponse(item), "items": responses}, err)
}

func (h *AccountHandler) refreshQuota(c *gin.Context) {
	var body AccountQuotaRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, errors.New("please select the account(s) to check quota for"))
		return
	}
	item, err := h.service.RefreshQuota(c.Request.Context(), body.AccessToken)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(c, http.StatusNotFound, errors.New("account does not exist"))
		return
	}
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": toAccountResponse(item)})
}

func accountFromRequest(r AccountRequest) repository.Account {
	return repository.Account{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, IDToken: r.IDToken, Email: r.Email, UserID: r.UserID, AccountID: r.AccountID, Type: r.Type, IsCodex: r.IsCodex, Status: r.Status, Quota: r.Quota, Proxy: r.Proxy, Remaining: r.Remaining, ResetAfter: r.ResetAfter, Success: r.Success, Fail: r.Fail, LastUsedAt: r.LastUsedAt, LastRefreshAt: r.LastRefreshAt, LastError: r.LastError, CreatedAt: r.CreatedAt}
}

func toAccountResponse(item repository.Account) AccountResponse {
	// The long-lived refresh_token and id_token are never sent back by the panel
	// (UpdateAccountRequest carries only type/status/quota/proxy), so we strip them
	// from the response and expose only a has_* flag. access_token stays because the
	// panel uses it as the account identifier for update/delete/quota calls.
	response := AccountResponse{AccountRequest: AccountRequest{AccessToken: item.AccessToken, Email: item.Email, UserID: item.UserID, AccountID: item.AccountID, Type: item.Type, IsCodex: item.IsCodex, Status: item.Status, Quota: item.Quota, Proxy: item.Proxy, Remaining: item.Remaining, ResetAfter: item.ResetAfter, Success: item.Success, Fail: item.Fail, LastUsedAt: item.LastUsedAt, LastRefreshAt: item.LastRefreshAt, LastError: item.LastError, CreatedAt: item.CreatedAt}, ImageInflight: item.ImageInflight, HasRefreshToken: item.RefreshToken != "", HasIDToken: item.IDToken != ""}
	return response
}

func intQuery(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	result, err := strconv.Atoi(value)
	if err != nil || result < 1 {
		return fallback
	}
	return result
}

// AccountRequest is the payload used to import or create an account.
type AccountRequest struct {
	AccessToken   string  `json:"access_token" binding:"required"`
	RefreshToken  string  `json:"refresh_token"`
	IDToken       string  `json:"id_token"`
	Email         *string `json:"email"`
	UserID        *string `json:"user_id"`
	AccountID     string  `json:"account_id"`
	Type          string  `json:"type"`
	IsCodex       bool    `json:"is_codex"`
	Status        string  `json:"status"`
	Quota         int     `json:"quota"`
	Proxy         string  `json:"proxy"`
	Remaining     int     `json:"remaining"`
	ResetAfter    *string `json:"reset_after"`
	Success       int     `json:"success"`
	Fail          int     `json:"fail"`
	LastUsedAt    *string `json:"last_used_at"`
	LastRefreshAt *string `json:"last_refresh_at"`
	LastError     *string `json:"last_error"`
	CreatedAt     string  `json:"created_at"`
}

// AccountResponse is the account representation returned by the API.
type AccountResponse struct {
	AccountRequest
	ImageInflight   int  `json:"image_inflight"`
	HasRefreshToken bool `json:"has_refresh_token"`
	HasIDToken      bool `json:"has_id_token"`
}

// DeleteAccountsRequest contains the access tokens of accounts to remove.
type DeleteAccountsRequest struct {
	Tokens []string `json:"tokens" binding:"required"`
}

type AccountQuotaRequest struct {
	AccessToken string `json:"access_token" binding:"required"`
}

// UpdateAccountRequest contains editable account fields.
type UpdateAccountRequest struct {
	AccessToken string  `json:"access_token" binding:"required"`
	Type        *string `json:"type"`
	Status      *string `json:"status"`
	Quota       *int    `json:"quota"`
	Proxy       *string `json:"proxy"`
}
