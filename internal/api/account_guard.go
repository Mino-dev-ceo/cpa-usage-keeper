package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type accountGuardCleanupRequest struct {
	DryRun *bool `json:"dry_run"`
}

func registerAccountGuardRoutes(router gin.IRoutes, provider AccountGuardProvider) {
	router.POST("/account-guard/cleanup-banned", func(c *gin.Context) {
		if provider == nil {
			writeInternalError(c, "account guard provider is not configured", nil)
			return
		}

		var request accountGuardCleanupRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		dryRun := true
		if request.DryRun != nil {
			dryRun = *request.DryRun
		}

		result, err := provider.CleanupBanned(c.Request.Context(), dryRun)
		if err != nil {
			slog.Error("account guard cleanup failed", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "CPA management API unavailable; make sure CLIProxyAPI is running and CPA_BASE_URL is reachable"})
			return
		}
		c.JSON(http.StatusOK, result)
	})
}
