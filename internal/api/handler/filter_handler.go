package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"telegram-message-forward/internal/api/dto"
	appfilter "telegram-message-forward/internal/app/filter"
	"telegram-message-forward/internal/domain/decision"
	"telegram-message-forward/internal/ruleengine/condition"
)

// FilterHandler 处理可复用过滤器相关请求。
type FilterHandler struct {
	svc *appfilter.Service
}

func NewFilterHandler(svc *appfilter.Service) *FilterHandler {
	return &FilterHandler{svc: svc}
}

// List GET /filters
func (h *FilterHandler) List(c *gin.Context) {
	fs, err := h.svc.List(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]dto.FilterDTO, 0, len(fs))
	stats, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	for _, f := range fs {
		item := dto.NewFilterDTO(f)
		item.Stats = stats[f.ID]
		if item.Stats.Since.IsZero() {
			item.Stats.Since = time.Now().Add(-24 * time.Hour)
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *FilterHandler) References(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	refs, err := h.svc.References(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": refs})
}

func (h *FilterHandler) Versions(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	items, count, err := h.svc.Versions(c.Request.Context(), id, parseIntDefault(c.Query("limit"), 30), parseIntDefault(c.Query("offset"), 0))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": count})
}

func (h *FilterHandler) Version(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	revision, err := strconv.Atoi(c.Param("revision"))
	if err != nil || revision <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "版本号必须为正整数"})
		return
	}
	version, err := h.svc.Version(c.Request.Context(), id, revision)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": version})
}

func (h *FilterHandler) Decisions(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	items, count, err := h.svc.Decisions(c.Request.Context(), decision.Query{FilterID: id, SourceID: parseInt64Default(c.Query("source_id"), 0), Status: c.Query("status"), Limit: parseIntDefault(c.Query("limit"), 30), Offset: parseIntDefault(c.Query("offset"), 0)})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": count})
}

func (h *FilterHandler) Preview(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	var req dto.FilterPreviewRequest
	if !bindJSON(c, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	in := appfilter.PreviewInput{Conditions: req.Conditions, CompareID: req.CompareID, MessageID: req.MessageID, SourceID: req.SourceID, From: req.From, To: req.To, BeforeID: req.BeforeID, Limit: req.Limit}
	if req.Message != nil {
		in.Message = req.Message.ToMessage()
	}
	result, err := h.svc.Preview(ctx, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *FilterHandler) Validate(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var req dto.FilterPreviewRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ValidateConditions(req.Conditions); err != nil {
		var validation *condition.ValidationError
		if errors.As(err, &validation) {
			respondError(c, err)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"valid": true}})
}

// Get GET /filters/:id
func (h *FilterHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	f, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dto.NewFilterDTO(f)})
}

// Create POST /filters
func (h *FilterHandler) Create(c *gin.Context) {
	var req dto.FilterRequest
	if !bindJSON(c, &req) {
		return
	}
	f, err := h.svc.Create(c.Request.Context(), req.ToInput())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": dto.NewFilterDTO(f)})
}

// Update PUT /filters/:id
func (h *FilterHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.FilterRequest
	if !bindJSON(c, &req) {
		return
	}
	f, err := h.svc.Update(c.Request.Context(), id, req.ToInput())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dto.NewFilterDTO(f)})
}

// Delete DELETE /filters/:id
func (h *FilterHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
