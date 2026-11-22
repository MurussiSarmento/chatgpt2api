package handler

import (
	"net/http"
	"slices"

	"github.com/basketikun/chatgpt2api/internal/config"
	"github.com/basketikun/chatgpt2api/internal/service"
	"github.com/gin-gonic/gin"
)

func Identity(auth *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")
		if authorization == "" && c.GetHeader("x-api-key") != "" {
			authorization = "Bearer " + c.GetHeader("x-api-key")
		}
		identity, err := auth.Authenticate(c.Request.Context(), authorization, true)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": gin.H{"error": service.ErrUnauthorized.Error()}})
			return
		}
		c.Set("identity", identity)
		c.Next()
	}
}

func Admin(c *gin.Context) {
	identity, exists := c.Get("identity")
	if !exists || identity.(service.Identity).Role != "admin" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"detail": gin.H{"error": "administrator privileges are required to perform this operation"}})
		return
	}
	c.Next()
}

func LimitBody(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cfg.MaxUploadMB<<20)
		c.Next()
	}
}

func CORS(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	if c.Request.Method == http.MethodOptions {
		c.AbortWithStatus(http.StatusNoContent)
		return
	}
	c.Next()
}

func identity(c *gin.Context) service.Identity {
	value, _ := c.Get("identity")
	return value.(service.Identity)
}

// allowVeta reports whether the caller may use Veta IA endpoints: the admin key
// always may; a user API key must carry the "veta" route.
func allowVeta(id service.Identity) bool {
	return id.Role == "admin" || slices.Contains(id.Routes, "veta")
}

// requireVeta aborts with 403 unless the caller is allowed to use Veta.
func requireVeta(c *gin.Context) bool {
	if allowVeta(identity(c)) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"detail": gin.H{"error": "this key is not allowed to use Veta IA; add the 'veta' route to the key"}})
	return false
}

func writeResult(c *gin.Context, value any, err error) {
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, value)
}

func writeError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"detail": gin.H{"error": err.Error()}})
}
