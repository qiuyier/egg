package inventory

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/qiuyier/egg/internal/app"
	"github.com/qiuyier/egg/internal/httpapi"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errStockOutOfRange = errors.New("stock out of range")

type item struct {
	ID    int64  `json:"id"`
	SKU   string `json:"sku"`
	Name  string `json:"name"`
	Stock int64  `json:"stock"`
}

func (item) TableName() string { return "inventory_items" }

type handler struct{ db *gorm.DB }

// Register mounts the inventory example under the caller's v1 group.
func Register(routes *gin.RouterGroup, deps app.Dependencies) {
	h := handler{db: deps.DB}
	items := routes.Group("/items")
	items.POST("", h.create)
	items.GET("", h.list)
	items.PATCH("/:id/stock", h.adjustStock)
}

func (h handler) create(c *gin.Context) {
	var req struct {
		SKU  string `json:"sku"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Respond(c, http.StatusBadRequest, "invalid_request", "expected a JSON item", nil)
		return
	}
	sku, name := strings.TrimSpace(req.SKU), strings.TrimSpace(req.Name)
	if sku == "" || name == "" || utf8.RuneCountInString(sku) > 64 || utf8.RuneCountInString(name) > 200 ||
		strings.ContainsRune(sku, 0) || strings.ContainsRune(name, 0) {
		httpapi.Respond(c, http.StatusBadRequest, "invalid_item", "sku and name are required and may be at most 64 and 200 characters", nil)
		return
	}
	created := item{SKU: sku, Name: name}
	if err := h.db.WithContext(c.Request.Context()).Create(&created).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "inventory_items_sku_unique" {
			httpapi.Respond(c, http.StatusConflict, "duplicate_sku", "sku already exists", nil)
			return
		}
		httpapi.Respond(c, http.StatusInternalServerError, "internal_error", "could not create item", nil)
		return
	}
	httpapi.Respond(c, http.StatusCreated, "ok", "item created", created)
}

func (h handler) list(c *gin.Context) {
	page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, perPageErr := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if pageErr != nil || perPageErr != nil || page < 1 || page > 1_000_000 || perPage < 1 || perPage > 100 {
		httpapi.Respond(c, http.StatusBadRequest, "invalid_pagination", "page must be 1–1000000 and per_page must be 1–100", nil)
		return
	}
	items := make([]item, 0)
	if err := h.db.WithContext(c.Request.Context()).Order("id").
		Limit(perPage).Offset((page - 1) * perPage).Find(&items).Error; err != nil {
		httpapi.Respond(c, http.StatusInternalServerError, "internal_error", "could not list items", nil)
		return
	}
	httpapi.Respond(c, http.StatusOK, "ok", "items listed", gin.H{"items": items, "page": page, "per_page": perPage})
}

func (h handler) adjustStock(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		httpapi.Respond(c, http.StatusBadRequest, "invalid_id", "id must be a positive integer", nil)
		return
	}
	var req struct {
		Delta int64 `json:"delta"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Delta == 0 {
		httpapi.Respond(c, http.StatusBadRequest, "invalid_delta", "delta must be a nonzero integer", nil)
		return
	}
	var updated item
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updated, id).Error; err != nil {
			return err
		}
		if req.Delta < -updated.Stock || req.Delta > math.MaxInt64-updated.Stock {
			return errStockOutOfRange
		}
		return tx.Model(&updated).Update("stock", updated.Stock+req.Delta).Error
	})
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		httpapi.Respond(c, http.StatusNotFound, "item_not_found", "item not found", nil)
	case errors.Is(err, errStockOutOfRange):
		httpapi.Respond(c, http.StatusConflict, "stock_out_of_range", "adjustment would make stock negative or exceed its maximum", nil)
	case err != nil:
		httpapi.Respond(c, http.StatusInternalServerError, "internal_error", "could not adjust stock", nil)
	default:
		httpapi.Respond(c, http.StatusOK, "ok", "stock adjusted", updated)
	}
}
