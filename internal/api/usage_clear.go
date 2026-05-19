package api

import (
	"context"
	"net/http"
	"strings"

	servicedto "cpa-usage-keeper/internal/service/dto"
	"github.com/gin-gonic/gin"
)

type usageClearProvider interface {
	ClearUsage(context.Context, servicedto.ClearUsageInput) (*servicedto.ClearUsageResult, error)
}

type clearUsageRequest struct {
	APIAlias string `json:"api_alias"`
	APIKey   string `json:"api_key"`
	All      bool   `json:"all"`
}

type clearUsageResponse struct {
	DeletedEvents  int64    `json:"deleted_events"`
	ClearedAliases []string `json:"cleared_aliases"`
	All            bool     `json:"all"`
}

func registerUsageClearRoute(router gin.IRoutes, provider any) {
	router.POST("/usage/clear", func(c *gin.Context) {
		clearProvider, ok := provider.(usageClearProvider)
		if !ok || clearProvider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "usage clear provider is not configured"})
			return
		}

		var request clearUsageRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		alias := strings.TrimSpace(request.APIAlias)
		if alias == "" {
			alias = strings.TrimSpace(request.APIKey)
		}
		if alias == "" && !request.All {
			c.JSON(http.StatusBadRequest, gin.H{"error": "api_alias is required"})
			return
		}

		result, err := clearProvider.ClearUsage(c.Request.Context(), servicedto.ClearUsageInput{
			APIAlias: alias,
			All:      request.All,
		})
		if err != nil {
			if strings.Contains(err.Error(), "required") {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			writeInternalError(c, "clear usage failed", err)
			return
		}

		c.JSON(http.StatusOK, clearUsageResponse{
			DeletedEvents:  result.DeletedEvents,
			ClearedAliases: result.ClearedAliases,
			All:            result.All,
		})
	})
}
