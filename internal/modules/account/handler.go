package account

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mthidayat/dompet-cerdas-go/internal/middleware"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/response"
)

// Handler exposes the shared-account HTTP endpoints.
type Handler struct {
	service    *Service
	repository *Repository
}

// NewHandler constructs the account HTTP handler. The repository is needed to
// invalidate the category cache the bot and query paths read from.
func NewHandler(service *Service, repository *Repository) *Handler {
	return &Handler{service: service, repository: repository}
}

// Register mounts the shared-account routes. The supplied router group must
// already have the Firebase auth middleware applied.
func (h *Handler) Register(rg *gin.RouterGroup) {
	shared := rg.Group("/shared-accounts")
	{
		shared.POST("", h.CreateSharedAccount)
		shared.POST("/convert", h.ShareExistingAccount)
		shared.POST("/join", h.JoinSharedAccount)
		shared.DELETE("/:id/access", h.RemoveSharedAccountAccess)
		shared.POST("/:id/invite-code", h.CreateSharedInviteCode)
	}

	rg.POST("/categories/refresh-cache", h.RefreshCategoryCache)
}

func mapAccountError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidArgument):
		response.BadRequest(c, "Data tidak valid.", "INVALID_ARGUMENT", nil)
	case errors.Is(err, ErrAccountNotFound), errors.Is(err, ErrInviteNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, ErrPermissionDenied):
		response.Forbidden(c, "Akses ditolak.")
	case errors.Is(err, ErrNotSharedAccount):
		response.FailedPrecondition(c, "Akun ini bukan akun bersama.", "NOT_SHARED")
	case errors.Is(err, ErrInviteExpired):
		response.FailedPrecondition(c, "Kode gabung sudah kadaluarsa.", "INVITE_EXPIRED")
	case errors.Is(err, ErrInviteExhausted):
		response.Fail(c, http.StatusTooManyRequests, "Gagal membuat kode gabung baru.", "INVITE_EXHAUSTED", nil)
	case errors.Is(err, ErrLastAccountRequired):
		response.FailedPrecondition(c, "Minimal harus ada satu Akun Keuangan yang tersisa.", "LAST_ACCOUNT")
	case errors.Is(err, ErrMembersStillPresent):
		response.FailedPrecondition(c, err.Error(), "MEMBERS_PRESENT")
	case errors.Is(err, ErrAlreadyShared):
		response.FailedPrecondition(c, "Akun sudah dibagikan.", "ALREADY_SHARED")
	default:
		response.InternalError(c)
	}
}

// CreateSharedAccount creates a shared workspace from scratch.
func (h *Handler) CreateSharedAccount(c *gin.Context) {
	userID, ok := middleware.RequireAuth(c)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Nama akun wajib diisi.", "INVALID_REQUEST", err.Error())
		return
	}
	email, _ := middleware.UserEmail(c)
	name, _ := middleware.UserName(c)
	result, err := h.service.CreateSharedAccount(c.Request.Context(), userID, req.Name, email, name)
	if err != nil {
		mapAccountError(c, err)
		return
	}
	response.Created(c, "Akun bersama berhasil dibuat", result)
}

// ShareExistingAccount converts a private account into a shared workspace.
func (h *Handler) ShareExistingAccount(c *gin.Context) {
	userID, ok := middleware.RequireAuth(c)
	if !ok {
		return
	}

	var req struct {
		AccountID string `json:"accountId" binding:"required"`
		Name      string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Data tidak valid", "INVALID_REQUEST", err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Keuangan Bersama"
	}

	sharedAcc, err := h.service.StartSharedAccountMigration(c.Request.Context(), userID, req.AccountID, name)
	if err != nil {
		mapAccountError(c, err)
		return
	}

	response.OK(c, "Proses migrasi akun bersama dimulai", sharedAcc)
}

// JoinSharedAccount joins a shared workspace using an invite code.
func (h *Handler) JoinSharedAccount(c *gin.Context) {
	userID, ok := middleware.RequireAuth(c)
	if !ok {
		return
	}
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Kode gabung wajib diisi.", "INVALID_REQUEST", err.Error())
		return
	}
	email, _ := middleware.UserEmail(c)
	name, _ := middleware.UserName(c)
	result, err := h.service.JoinSharedAccountByCode(c.Request.Context(), userID, req.Code, email, name)
	if err != nil {
		mapAccountError(c, err)
		return
	}
	response.OK(c, "Berhasil bergabung ke akun bersama", result)
}

// RemoveSharedAccountAccess deletes the workspace (owner) or leaves it (member).
func (h *Handler) RemoveSharedAccountAccess(c *gin.Context) {
	userID, ok := middleware.RequireAuth(c)
	if !ok {
		return
	}
	accountID := c.Param("id")
	result, err := h.service.RemoveSharedAccountAccess(c.Request.Context(), userID, accountID)
	if err != nil {
		mapAccountError(c, err)
		return
	}
	response.OK(c, "Akses akun bersama diperbarui", result)
}

// CreateSharedInviteCode issues a time-limited invite code for a workspace.
func (h *Handler) CreateSharedInviteCode(c *gin.Context) {
	userID, ok := middleware.RequireAuth(c)
	if !ok {
		return
	}
	accountID := c.Param("id")
	result, err := h.service.CreateSharedInviteCode(c.Request.Context(), userID, accountID)
	if err != nil {
		mapAccountError(c, err)
		return
	}
	response.Created(c, "Kode undangan berhasil dibuat", result)
}

// RefreshCategoryCache drops the cached category list for the caller's account,
// replacing the refreshCategoryCache Firebase callable.
//
// The web app calls this right after editing categories, so the Telegram bot and
// the next query do not keep answering from a stale list.
func (h *Handler) RefreshCategoryCache(c *gin.Context) {
	userID, ok := middleware.RequireAuth(c)
	if !ok {
		return
	}

	var req struct {
		AccountID string `json:"accountId"`
	}
	// The body is optional: with no accountId the user's active account is used.
	_ = c.ShouldBindJSON(&req)

	ac, err := h.service.GetAccountContext(c.Request.Context(), userID, req.AccountID)
	if err != nil {
		response.InternalError(c)
		return
	}

	h.repository.InvalidateCategoryCache(ac)
	response.OK(c, "Cache kategori diperbarui", nil)
}
