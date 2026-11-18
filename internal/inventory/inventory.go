package inventory

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/qiuyier/egg/internal/app"
	"github.com/qiuyier/egg/internal/httpapi"
)

var errStockOutOfRange = errors.New("stock out of range")

type item struct {
	ID    int64  `db:"id" json:"id"`
	SKU   string `db:"sku" json:"sku"`
	Name  string `db:"name" json:"name"`
	Stock int64  `db:"stock" json:"stock"`
}

type handler struct{ db *sqlx.DB }

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
	var created item
	if err := h.db.GetContext(c.Request.Context(), &created, `
		INSERT INTO inventory_items (sku, name) VALUES ($1, $2)
		RETURNING id, sku, name, stock`, sku, name); err != nil {
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
	if err := h.db.SelectContext(c.Request.Context(), &items, `
		SELECT id, sku, name, stock FROM inventory_items
		ORDER BY id LIMIT $1 OFFSET $2`, perPage, (page-1)*perPage); err != nil {
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
	updated, err := h.changeStock(c.Request.Context(), id, req.Delta)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		httpapi.Respond(c, http.StatusNotFound, "item_not_found", "item not found", nil)
	case errors.Is(err, errStockOutOfRange):
		httpapi.Respond(c, http.StatusConflict, "stock_out_of_range", "adjustment would make stock negative or exceed its maximum", nil)
	case err != nil:
		httpapi.Respond(c, http.StatusInternalServerError, "internal_error", "could not adjust stock", nil)
	default:
		httpapi.Respond(c, http.StatusOK, "ok", "stock adjusted", updated)
	}
}

func (h handler) changeStock(ctx context.Context, id, delta int64) (item, error) {
	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		return item{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var updated item
	if err := tx.GetContext(ctx, &updated, `
		SELECT id, sku, name, stock FROM inventory_items WHERE id = $1 FOR UPDATE`, id); err != nil {
		return item{}, err
	}
	if delta < -updated.Stock || delta > math.MaxInt64-updated.Stock {
		return item{}, errStockOutOfRange
	}
	updated.Stock += delta
	if _, err := tx.ExecContext(ctx, `UPDATE inventory_items SET stock = $1 WHERE id = $2`, updated.Stock, id); err != nil {
		return item{}, err
	}
	if err := tx.Commit(); err != nil {
		return item{}, err
	}
	return updated, nil
}
