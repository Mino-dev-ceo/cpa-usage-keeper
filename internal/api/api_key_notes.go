package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	servicedto "cpa-usage-keeper/internal/service/dto"
	"github.com/gin-gonic/gin"
)

type apiKeyNoteProvider interface {
	ListAPIKeyNotes(context.Context) ([]servicedto.APIKeyNote, error)
	UpsertAPIKeyNote(context.Context, string, string) (servicedto.APIKeyNote, error)
	DeleteAPIKeyNote(context.Context, string) error
}

type apiKeyNotePayload struct {
	APIKey string `json:"api_key"`
	Note   string `json:"note"`
}

type apiKeyNotesResponse struct {
	Notes []apiKeyNotePayload `json:"notes"`
}

type apiKeyNoteRequest struct {
	Note string `json:"note"`
}

func registerAPIKeyNoteRoutes(router gin.IRoutes, provider any) {
	noteProvider, ok := provider.(apiKeyNoteProvider)
	if !ok || noteProvider == nil {
		return
	}

	router.GET("/api-key-notes", func(c *gin.Context) {
		notes, err := noteProvider.ListAPIKeyNotes(c.Request.Context())
		if err != nil {
			writeInternalError(c, "list api key notes failed", err)
			return
		}
		payload := make([]apiKeyNotePayload, 0, len(notes))
		for _, note := range notes {
			payload = append(payload, apiKeyNotePayload{
				APIKey: note.APIAlias,
				Note:   note.Note,
			})
		}
		c.JSON(http.StatusOK, apiKeyNotesResponse{Notes: payload})
	})

	router.PUT("/api-key-notes/:api_key", func(c *gin.Context) {
		var req apiKeyNoteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		note, err := noteProvider.UpsertAPIKeyNote(c.Request.Context(), c.Param("api_key"), req.Note)
		if err != nil {
			writeAPIKeyNoteError(c, err)
			return
		}
		c.JSON(http.StatusOK, apiKeyNotePayload{APIKey: note.APIAlias, Note: note.Note})
	})

	router.DELETE("/api-key-notes/:api_key", func(c *gin.Context) {
		if err := noteProvider.DeleteAPIKeyNote(c.Request.Context(), c.Param("api_key")); err != nil {
			writeAPIKeyNoteError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
}

func writeAPIKeyNoteError(c *gin.Context, err error) {
	if err == nil {
		c.Status(http.StatusOK)
		return
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "invalid api key note"
	}
	if errors.Is(err, context.Canceled) {
		c.JSON(http.StatusRequestTimeout, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": message})
}
