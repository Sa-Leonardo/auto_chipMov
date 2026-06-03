package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"chipmov/internal/auth"
	"chipmov/internal/config"
	"chipmov/internal/domain"
	"chipmov/internal/easy2use"
	"chipmov/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Provider interface {
	ListSubscribers(ctx context.Context) (easy2use.ListSubscribersResponse, []byte, int, error)
	LastRecharge(ctx context.Context, simCard string) (easy2use.LastRechargeResponse, []byte, int, error)
	DataBalance(ctx context.Context, simCardOrPhone string, realTime bool) (easy2use.DataBalanceResponse, []byte, int, error)
	DetailedConsumption(ctx context.Context, req easy2use.DetailedConsumptionRequest) (easy2use.DetailedConsumptionResponse, []byte, int, error)
	AddBalance(ctx context.Context, simCard string, quantity int) (easy2use.AddBalanceResponse, []byte, int, error)
}

type Server struct {
	cfg      config.Config
	store    *storage.Store
	provider Provider
	logger   *slog.Logger
	auth     *auth.Service
	limits   map[string][]time.Time
	limitsMu sync.Mutex
}

func NewServer(cfg config.Config, store *storage.Store, provider Provider, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, store: store, provider: provider, logger: logger, auth: auth.NewService(cfg, store), limits: map[string][]time.Time{}}
}

func (s *Server) Router() http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(s.securityHeaders())
	router.Use(s.cors())
	router.Use(s.rateLimit())
	router.Static("/assets", "./web/assets")
	router.Static("/app", "./webapp/dist")
	router.StaticFile("/app-ui", "./webapp/dist/index.html")
	router.StaticFile("/", "./webapp/dist/index.html")
	router.StaticFile("/legacy", "./web/index.html")
	router.StaticFile("/relatorios", "./web/index.html")
	router.GET("/health", s.health)
	router.POST("/api/auth/login", s.login)
	router.POST("/api/auth/refresh", s.refresh)
	router.POST("/api/auth/logout", s.logout)
	router.GET("/api/ws", s.websocket)

	protected := router.Group("/")
	protected.Use(s.authMiddleware())
	protected.GET("/api/me", s.me)
	protected.GET("/api/users", s.requirePermission("users:read"), s.listUsers)
	protected.POST("/api/users", s.requirePermission("users:write"), s.createUser)
	protected.PUT("/api/users/:id", s.requirePermission("users:write"), s.updateUser)
	protected.GET("/api/audit-logs", s.requirePermission("audit:read"), s.listAuditLogs)
	protected.GET("/api/dashboard/summary", s.requirePermission("iccids:read"), s.dashboardSummary)
	protected.GET("/api/attendance/search", s.requirePermission("attendance:read"), s.attendanceSearch)
	protected.GET("/api/attendance/iccids/:iccid", s.requirePermission("attendance:read"), s.attendanceDetail)
	protected.POST("/api/attendance/iccids/:iccid/recharge", s.requirePermission("attendance:recharge"), s.attendanceRecharge)
	protected.GET("/api/attendance/audit", s.requirePermission("attendance:read"), s.attendanceAudit)
	protected.POST("/sync/assinantes", s.syncSubscribers)
	protected.POST("/sync/ultima-recarga", s.syncLastRecharges)
	protected.GET("/iccids", s.requirePermission("iccids:read"), s.listICCIDs)
	protected.GET("/iccids/summary", s.requirePermission("iccids:read"), s.iccidSummary)
	protected.POST("/iccids/:iccid/saldo", s.requirePermission("recharge:write"), s.addBalanceManual)
	protected.POST("/automation/check-recharges", s.checkRecharges)
	protected.GET("/automation/next-run", s.nextRun)
	protected.GET("/recharge-approvals", s.requirePermission("approvals:read"), s.listApprovals)
	protected.POST("/recharge-approvals/:id/approve", s.requirePermission("approvals:write"), s.approveRecharge)
	protected.POST("/recharge-approvals/:id/reject", s.requirePermission("approvals:write"), s.rejectRecharge)
	protected.POST("/dev/iccids/:iccid/force-due", s.forceDueDev)
	protected.GET("/operacoes", s.requirePermission("operations:read"), s.listOperations)
	protected.POST("/dev/iccids/:iccid/force-status", s.forceStatusDev)

	return router
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		adminKey := c.GetHeader("X-Admin-Key")
		if adminKey == "" {
			adminKey = c.GetHeader("X-API-Key")
		}
		if adminKey != "" && adminKey == s.cfg.AdminKey {
			c.Set("auth_role", domain.RoleAdmin)
			c.Set("auth_email", "admin-key")
			c.Next()
			return
		}
		claims, err := s.auth.ValidateAccessToken(c.GetHeader("Authorization"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Set("auth_user_id", claims.UserID)
		c.Set("auth_email", claims.Email)
		c.Set("auth_role", claims.Role)
		c.Next()
	}
}

func (s *Server) requirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("auth_role")
		if auth.HasPermission(role.(domain.UserRole), permission) {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "permission": permission})
		c.Abort()
	}
}

func (s *Server) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	}
}

func (s *Server) cors() gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, origin := range s.cfg.CORSAllowedOrigins {
		allowed[origin] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-Admin-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) rateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if strings.HasPrefix(c.Request.URL.Path, "/api/auth/") {
			if !s.allowRequest(key+":auth", 20, time.Minute) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func (s *Server) allowRequest(key string, limit int, window time.Duration) bool {
	now := time.Now()
	cutoff := now.Add(-window)
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	events := s.limits[key]
	filtered := events[:0]
	for _, event := range events {
		if event.After(cutoff) {
			filtered = append(filtered, event)
		}
	}
	if len(filtered) >= limit {
		s.limits[key] = filtered
		return false
	}
	filtered = append(filtered, now)
	s.limits[key] = filtered
	return true
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	pair, err := s.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		_ = s.store.CreateAuditLog(c.Request.Context(), domain.AuditLog{
			Action: "auth.login_failed", Resource: "auth", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), Metadata: strings.ToLower(strings.TrimSpace(req.Email)),
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	userID := pair.User.ID
	_ = s.store.CreateAuditLog(c.Request.Context(), domain.AuditLog{
		UserID: &userID, Action: "auth.login", Resource: "auth", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	pair, err := s.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pair)
}

func (s *Server) logout(c *gin.Context) {
	var req refreshRequest
	_ = c.ShouldBindJSON(&req)
	if err := s.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) me(c *gin.Context) {
	userID, ok := c.Get("auth_user_id")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"user": gin.H{"email": "admin-key", "role": domain.RoleAdmin}})
		return
	}
	user, err := s.store.GetUserByID(c.Request.Context(), userID.(int64))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	user.PasswordHash = ""
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (s *Server) listUsers(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range users {
		users[i].PasswordHash = ""
	}
	c.JSON(http.StatusOK, gin.H{"items": users})
}

type userRequest struct {
	Name     string          `json:"name"`
	Email    string          `json:"email"`
	Password string          `json:"password"`
	Role     domain.UserRole `json:"role"`
	Active   *bool           `json:"active"`
}

func (s *Server) createUser(c *gin.Context) {
	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Email) == "" || len(req.Password) < 8 || !validRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, email, password>=8 and valid role are required"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	user, err := s.store.CreateUser(c.Request.Context(), domain.User{
		Name: req.Name, Email: req.Email, PasswordHash: hash, Role: req.Role, Active: active,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	user.PasswordHash = ""
	s.audit(c, "users.create", "users", strconv.FormatInt(user.ID, 10), user.Email)
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

func (s *Server) updateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" || !validRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and valid role are required"})
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	user, err := s.store.UpdateUser(c.Request.Context(), domain.User{ID: id, Name: req.Name, Role: req.Role, Active: active})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Password) != "" {
		if len(req.Password) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password must have at least 8 chars"})
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_ = s.store.UpdateUserPassword(c.Request.Context(), id, hash)
		_ = s.store.RevokeUserRefreshTokens(c.Request.Context(), id)
	}
	user.PasswordHash = ""
	s.audit(c, "users.update", "users", strconv.FormatInt(user.ID, 10), user.Email)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (s *Server) listAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	items, err := s.store.ListAuditLogs(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func validRole(role domain.UserRole) bool {
	switch role {
	case domain.RoleAdmin, domain.RoleSupervisor, domain.RoleOperator, domain.RoleViewer:
		return true
	default:
		return false
	}
}

func (s *Server) audit(c *gin.Context, action, resource, resourceID, metadata string) {
	var userID *int64
	if raw, ok := c.Get("auth_user_id"); ok {
		id := raw.(int64)
		userID = &id
	}
	_ = s.store.CreateAuditLog(c.Request.Context(), domain.AuditLog{
		UserID: userID, Action: action, Resource: resource, ResourceID: resourceID, Metadata: metadata, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
}

func (s *Server) auditMetadata(c *gin.Context, action, resource, resourceID string, metadata any) {
	var encoded string
	if metadata != nil {
		if bytes, err := json.Marshal(metadata); err == nil {
			encoded = string(bytes)
		}
	}
	s.audit(c, action, resource, resourceID, encoded)
}

func (s *Server) syncSubscribers(c *gin.Context) {
	ctx := c.Request.Context()
	resp, raw, statusCode, err := s.provider.ListSubscribers(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":                  err.Error(),
			"status_code":            statusCode,
			"provider_response_body": string(raw),
			"hint":                   "Confira EASY2USE_BASE_URL e EASY2USE_USER_TOKEN no .env",
		})
		return
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider returned non-success codigo_status_tip", "codigo_status_tip": resp.StatusCodeTip})
		return
	}

	totalContracts := 0
	allowedSubscribers := 0
	allowedContracts := 0
	saved := 0
	skipped := 0
	savedByCNPJ := map[string]int{}
	savedByStatus := map[string]int{}
	allowedContractsByCNPJ := map[string]int{}
	for _, subscriber := range resp.Results {
		cnpj := config.OnlyDigits(subscriber.Document)
		allowed, err := s.store.IsAllowedCNPJ(ctx, cnpj)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if allowed {
			allowedSubscribers++
		}
		for _, contract := range subscriber.Contracts {
			totalContracts++
			if !allowed || strings.TrimSpace(contract.SimCard) == "" {
				skipped++
				continue
			}
			allowedContracts++
			allowedContractsByCNPJ[cnpj]++
			if err := s.store.UpsertICCID(ctx, storage.UpsertICCIDParams{
				CNPJ:                   cnpj,
				SubscriberName:         subscriberDisplayName(subscriber),
				SimCard:                strings.TrimSpace(contract.SimCard),
				PhoneNumber:            strings.TrimSpace(contract.PhoneLine),
				ContractNumber:         strings.TrimSpace(contract.ContractNumber),
				ContractStatus:         strings.TrimSpace(contract.Status),
				PlanName:               strings.TrimSpace(contract.Plan),
				DefaultQuantity:        s.cfg.DefaultRechargeQuantity,
				RechargeIntervalMonths: s.cfg.RechargeIntervalMonths,
				SafetyWindowDays:       s.cfg.RechargeSafetyWindowDays,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			saved++
			savedByCNPJ[cnpj]++
			status := strings.TrimSpace(contract.Status)
			if status == "" {
				status = "(vazio)"
			}
			savedByStatus[status]++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_subscribers":         len(resp.Results),
		"total_contracts":           totalContracts,
		"allowed_subscribers":       allowedSubscribers,
		"allowed_contracts":         allowedContracts,
		"allowed_contracts_by_cnpj": allowedContractsByCNPJ,
		"saved":                     saved,
		"saved_by_cnpj":             savedByCNPJ,
		"saved_by_status":           savedByStatus,
		"skipped":                   skipped,
	})
}

func (s *Server) syncLastRecharges(c *gin.Context) {
	ctx := c.Request.Context()
	items, err := s.store.ListICCIDs(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated := 0
	failed := 0
	failures := []gin.H{}
	rateLimited := false
	for index, item := range items {
		if strings.TrimSpace(item.SimCard) == "" {
			continue
		}
		if index > 0 && s.cfg.ProviderRequestDelay > 0 {
			select {
			case <-ctx.Done():
				c.JSON(http.StatusRequestTimeout, gin.H{"error": ctx.Err().Error()})
				return
			case <-time.After(s.cfg.ProviderRequestDelay):
			}
		}
		resp, _, statusCode, err := s.provider.LastRecharge(ctx, item.SimCard)
		if err != nil {
			failed++
			failures = append(failures, gin.H{"sim_card": item.SimCard, "error": err.Error(), "status_code": statusCode})
			if statusCode == http.StatusTooManyRequests {
				rateLimited = true
				break
			}
			continue
		}
		if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
			failed++
			failures = append(failures, gin.H{"sim_card": item.SimCard, "codigo_status_tip": resp.StatusCodeTip})
			continue
		}
		lastRecharge, err := time.ParseInLocation("2006-01-02", resp.LastRecharge, time.Local)
		if err != nil {
			failed++
			failures = append(failures, gin.H{"sim_card": item.SimCard, "error": "invalid ultima_recarga: " + resp.LastRecharge})
			continue
		}
		if err := s.store.UpdateLastRecharge(ctx, item.SimCard, lastRecharge, item.RechargeIntervalMonths, item.SafetyWindowDays); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updated++
	}

	c.JSON(http.StatusOK, gin.H{
		"checked":      updated + failed,
		"total_iccids": len(items),
		"updated":      updated,
		"failed":       failed,
		"rate_limited": rateLimited,
		"failures":     failures,
	})
}

func (s *Server) listICCIDs(c *gin.Context) {
	items, err := s.store.ListICCIDs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) iccidSummary(c *gin.Context) {
	items, err := s.store.ICCIDSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) dashboardSummary(c *gin.Context) {
	ctx := c.Request.Context()
	iccids, err := s.store.ListICCIDs(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	approvals, err := s.store.ListApprovals(ctx, "pending", 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operations, err := s.store.ListOperations(ctx, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	next, actionable, err := s.store.NextRun(ctx, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	statusCounts := map[string]int{}
	due := 0
	for _, item := range iccids {
		statusCounts[strings.TrimSpace(item.ContractStatus)]++
		if item.NextRechargeDueAt != nil && !item.NextRechargeDueAt.After(time.Now()) && strings.EqualFold(item.ContractStatus, "EM USO") {
			due++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"total_iccids":            len(iccids),
		"status_counts":           statusCounts,
		"pending_approvals":       len(approvals),
		"due_recharges":           due,
		"next_recharge_due_at":    next,
		"actionable_iccids_count": actionable,
		"recent_operations":       operations,
		"important_alerts":        dashboardAlerts(statusCounts, len(approvals), due),
	})
}

func dashboardAlerts(statusCounts map[string]int, approvals int, due int) []gin.H {
	alerts := []gin.H{}
	if approvals > 0 {
		alerts = append(alerts, gin.H{"level": "warning", "message": fmt.Sprintf("%d aprovacao(oes) pendente(s)", approvals)})
	}
	if due > 0 {
		alerts = append(alerts, gin.H{"level": "danger", "message": fmt.Sprintf("%d ICCID(s) dentro da janela de recarga", due)})
	}
	if statusCounts["CANCELADO"] > 0 {
		alerts = append(alerts, gin.H{"level": "info", "message": fmt.Sprintf("%d contrato(s) cancelado(s) monitorado(s)", statusCounts["CANCELADO"])})
	}
	return alerts
}

type attendanceRechargeRequest struct {
	Quantity int    `json:"quantity"`
	DryRun   bool   `json:"dry_run"`
	Note     string `json:"note"`
}

func (s *Server) attendanceSearch(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	searchType := strings.TrimSpace(c.DefaultQuery("type", "auto"))
	period := strings.TrimSpace(c.DefaultQuery("period", time.Now().Format("2006-01")))
	if _, err := time.Parse("2006-01", period); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "period must use YYYY-MM format"})
		return
	}
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}
	resolvedType := attendanceSearchType(query, searchType)
	resp, raw, statusCode, err := s.provider.ListSubscribers(c.Request.Context())
	if err != nil {
		s.auditMetadata(c, "attendance.search_failed", "attendance", "", gin.H{"query_type": searchType, "status_code": statusCode, "error": err.Error()})
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "status_code": statusCode, "provider_response_body": string(raw)})
		return
	}
	results := filterAttendanceResults(resp.Results, query, searchType, 50)
	if len(results) == 0 && resolvedType == "name" {
		results = s.searchAttendanceByConsumptionName(c.Request.Context(), query, period, 50)
	}
	s.enrichAttendanceNames(c.Request.Context(), results, period)
	s.auditMetadata(c, "attendance.search", "attendance", "", gin.H{"query_type": resolvedType, "period": period, "results": len(results)})
	c.JSON(http.StatusOK, gin.H{"items": results, "count": len(results)})
}

func (s *Server) attendanceDetail(c *gin.Context) {
	simCard := strings.TrimSpace(c.Param("iccid"))
	period := strings.TrimSpace(c.DefaultQuery("period", time.Now().Format("2006-01")))
	if _, err := time.Parse("2006-01", period); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "period must use YYYY-MM format"})
		return
	}
	realTimeBalance := configBool(c.DefaultQuery("real_time", "false"))
	item, found, err := s.findProviderICCID(c.Request.Context(), simCard)
	if err != nil {
		s.auditMetadata(c, "attendance.iccid_view_failed", "attendance_iccid", simCard, gin.H{"error": err.Error()})
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !found {
		s.auditMetadata(c, "attendance.iccid_view_not_found", "attendance_iccid", simCard, nil)
		c.JSON(http.StatusNotFound, gin.H{"error": "iccid not found in provider"})
		return
	}
	var lastRecharge any
	lastResp, raw, statusCode, err := s.provider.LastRecharge(c.Request.Context(), simCard)
	if err != nil {
		lastRecharge = gin.H{"error": err.Error(), "status_code": statusCode, "provider_response_body": string(raw)}
	} else {
		lastRecharge = lastResp
	}
	balance := s.attendanceBalance(c, simCard, realTimeBalance)
	usageHistory := s.attendanceUsageHistory(c, item, period)
	if strings.TrimSpace(fmt.Sprint(item["subscriber_name"])) == "" {
		if name := firstConsumptionClientName(usageHistory); name != "" {
			item["subscriber_name"] = name
		}
	}
	operations, err := s.store.ListOperationsBySimCard(c.Request.Context(), simCard, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	audit, err := s.store.ListAuditLogsFiltered(c.Request.Context(), "attendance_iccid", simCard, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.auditMetadata(c, "attendance.iccid_viewed", "attendance_iccid", simCard, gin.H{"cnpj": item["cnpj"], "subscriber_name": item["subscriber_name"], "period": period, "real_time_balance": realTimeBalance})
	c.JSON(http.StatusOK, gin.H{
		"item":          item,
		"last_recharge": lastRecharge,
		"balance":       balance,
		"usage_history": usageHistory,
		"operations":    operations,
		"audit":         audit,
	})
}

func (s *Server) attendanceRecharge(c *gin.Context) {
	simCard := strings.TrimSpace(c.Param("iccid"))
	var req attendanceRechargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	if req.Quantity < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quantity must be at least 1"})
		return
	}
	item, found, err := s.findProviderICCID(c.Request.Context(), simCard)
	if err != nil {
		s.auditMetadata(c, "attendance.recharge_failed", "attendance_iccid", simCard, gin.H{"quantity": req.Quantity, "error": err.Error()})
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !found {
		s.auditMetadata(c, "attendance.recharge_failed", "attendance_iccid", simCard, gin.H{"quantity": req.Quantity, "error": "iccid not found in provider"})
		c.JSON(http.StatusNotFound, gin.H{"error": "iccid not found in provider"})
		return
	}
	status, _ := item["contract_status"].(string)
	if !strings.EqualFold(strings.TrimSpace(status), "EM USO") {
		s.auditMetadata(c, "attendance.recharge_blocked", "attendance_iccid", simCard, gin.H{"quantity": req.Quantity, "contract_status": status})
		c.JSON(http.StatusForbidden, gin.H{"error": "iccid is not active", "contract_status": status})
		return
	}
	if !req.DryRun && !s.cfg.EnableRealRecharge {
		s.auditMetadata(c, "attendance.recharge_blocked", "attendance_iccid", simCard, gin.H{"quantity": req.Quantity, "error": "real recharge is disabled"})
		c.JSON(http.StatusForbidden, gin.H{"error": "real recharge is disabled"})
		return
	}
	if req.DryRun {
		s.auditMetadata(c, "attendance.recharge_dry_run", "attendance_iccid", simCard, gin.H{"quantity": req.Quantity, "note": req.Note})
		c.JSON(http.StatusOK, gin.H{"status": "dry_run", "sim_card": simCard, "quantity": req.Quantity, "item": item})
		return
	}

	cnpj, _ := item["cnpj"].(string)
	requestPayload, _ := json.Marshal(gin.H{"quantity": req.Quantity, "note": strings.TrimSpace(req.Note), "module": "attendance"})
	opID, err := s.store.CreateOperation(c.Request.Context(), domain.GBOperation{
		SimCard:        simCard,
		CNPJ:           cnpj,
		Quantity:       req.Quantity,
		Status:         "pending",
		TriggerType:    "attendance",
		RequestPayload: string(requestPayload),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp, raw, statusCode, err := s.provider.AddBalance(c.Request.Context(), simCard, req.Quantity)
	code := statusCode
	responsePayload := string(raw)
	if err != nil {
		_ = s.store.FinishOperation(c.Request.Context(), opID, "failed", &code, "", responsePayload, err.Error())
		s.auditMetadata(c, "attendance.recharge_failed", "attendance_iccid", simCard, gin.H{"operation_id": opID, "quantity": req.Quantity, "error": err.Error(), "status_code": statusCode})
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "operation_id": opID, "provider_status_code": statusCode, "provider_response_body": responsePayload})
		return
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		_ = s.store.FinishOperation(c.Request.Context(), opID, "failed", &code, resp.UserMessage, responsePayload, "provider returned non-success codigo_status_tip")
		s.auditMetadata(c, "attendance.recharge_failed", "attendance_iccid", simCard, gin.H{"operation_id": opID, "quantity": req.Quantity, "codigo_status_tip": resp.StatusCodeTip, "message": resp.UserMessage})
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider returned non-success codigo_status_tip", "operation_id": opID, "provider_response": resp})
		return
	}
	_ = s.store.FinishOperation(c.Request.Context(), opID, "success", &code, resp.UserMessage, responsePayload, "")
	s.auditMetadata(c, "attendance.recharge_success", "attendance_iccid", simCard, gin.H{"operation_id": opID, "quantity": req.Quantity, "message": resp.UserMessage, "note": req.Note})
	c.JSON(http.StatusOK, gin.H{"status": "success", "operation_id": opID, "sim_card": simCard, "quantity": req.Quantity, "provider_response": resp})
}

func (s *Server) attendanceAudit(c *gin.Context) {
	resourceID := strings.TrimSpace(c.Query("iccid"))
	items, err := s.store.ListAuditLogsFiltered(c.Request.Context(), "attendance_iccid", resourceID, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) attendanceBalance(c *gin.Context, simCard string, realTime bool) gin.H {
	resp, raw, statusCode, err := s.provider.DataBalance(c.Request.Context(), simCard, realTime)
	if err != nil {
		s.auditMetadata(c, "attendance.balance_failed", "attendance_iccid", simCard, gin.H{"error": err.Error(), "status_code": statusCode})
		return gin.H{"available": false, "error": err.Error(), "status_code": statusCode, "provider_response_body": string(raw)}
	}
	s.auditMetadata(c, "attendance.balance_checked", "attendance_iccid", simCard, gin.H{"real_time": realTime, "balance": resp.Balance})
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		return gin.H{"available": false, "message": "provider returned non-success codigo_status_tip", "codigo_status_tip": resp.StatusCodeTip, "raw": resp}
	}
	return gin.H{"available": true, "value": resp.Balance, "unit": "GB", "real_time": realTime, "raw": resp}
}

func (s *Server) attendanceUsageHistory(c *gin.Context, item gin.H, period string) gin.H {
	simCard, _ := item["sim_card"].(string)
	document, _ := item["cnpj"].(string)
	phone, _ := item["phone_number"].(string)
	resp, raw, statusCode, err := s.provider.DetailedConsumption(c.Request.Context(), easy2use.DetailedConsumptionRequest{
		Period:        period,
		Document:      document,
		ContractPhone: providerPhoneNumber(phone),
	})
	if err != nil {
		s.auditMetadata(c, "attendance.usage_failed", "attendance_iccid", simCard, gin.H{"period": period, "error": err.Error(), "status_code": statusCode})
		return gin.H{"available": false, "period": period, "error": err.Error(), "status_code": statusCode, "provider_response_body": string(raw)}
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		s.auditMetadata(c, "attendance.usage_failed", "attendance_iccid", simCard, gin.H{"period": period, "codigo_status_tip": resp.StatusCodeTip})
		return gin.H{"available": false, "period": period, "message": "provider returned non-success codigo_status_tip", "codigo_status_tip": resp.StatusCodeTip, "raw": resp}
	}
	summary := summarizeConsumption(resp.Results)
	s.auditMetadata(c, "attendance.usage_checked", "attendance_iccid", simCard, gin.H{"period": period, "contracts": len(resp.Results), "internet_total": summary["internet_total"]})
	return gin.H{"available": true, "period": period, "summary": summary, "results": resp.Results}
}

func (s *Server) enrichAttendanceNames(ctx context.Context, items []gin.H, period string) {
	for _, item := range items {
		if strings.TrimSpace(fmt.Sprint(item["subscriber_name"])) != "" {
			continue
		}
		document, _ := item["cnpj"].(string)
		phone, _ := item["phone_number"].(string)
		resp, _, _, err := s.provider.DetailedConsumption(ctx, easy2use.DetailedConsumptionRequest{
			Period:        period,
			Document:      document,
			ContractPhone: providerPhoneNumber(phone),
		})
		if err != nil || !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
			continue
		}
		for _, result := range resp.Results {
			if strings.TrimSpace(result.ClientName) == "" {
				continue
			}
			item["subscriber_name"] = strings.TrimSpace(result.ClientName)
			break
		}
	}
}

func (s *Server) searchAttendanceByConsumptionName(ctx context.Context, query string, period string, limit int) []gin.H {
	clean := normalizeSearch(query)
	if clean == "" {
		return nil
	}
	results := []gin.H{}
	seen := map[string]bool{}
	for _, month := range recentPeriods(period, 6) {
		resp, _, _, err := s.provider.DetailedConsumption(ctx, easy2use.DetailedConsumptionRequest{Period: month})
		if err != nil || !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
			continue
		}
		for _, contract := range resp.Results {
			if !strings.Contains(normalizeSearch(contract.ClientName), clean) {
				continue
			}
			simCard := strings.TrimSpace(contract.ICCID)
			if simCard == "" || seen[simCard] {
				continue
			}
			seen[simCard] = true
			results = append(results, attendanceResultFromConsumption(contract))
			if len(results) >= limit {
				return results
			}
		}
		if len(results) > 0 {
			return results
		}
	}
	return results
}

func (s *Server) findProviderICCID(ctx context.Context, simCard string) (gin.H, bool, error) {
	resp, _, _, err := s.provider.ListSubscribers(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, subscriber := range resp.Results {
		for _, contract := range subscriber.Contracts {
			if strings.EqualFold(strings.TrimSpace(contract.SimCard), simCard) {
				return attendanceResult(subscriber, contract), true, nil
			}
		}
	}
	return nil, false, nil
}

func summarizeConsumption(results []easy2use.DetailedConsumptionContract) gin.H {
	internetTotal := 0.0
	uploadTotal := 0.0
	downloadTotal := 0.0
	voiceSeconds := 0
	voiceMinutes := 0
	smsCount := 0
	for _, contract := range results {
		smsCount += len(contract.SMS)
		for _, item := range contract.Internet {
			internetTotal += item.Total
			uploadTotal += item.Upload
			downloadTotal += item.Download
		}
		for _, item := range contract.Voice {
			voiceSeconds += item.Seconds
			voiceMinutes += item.Minutes
		}
	}
	return gin.H{
		"contracts":      len(results),
		"internet_total": internetTotal,
		"upload_total":   uploadTotal,
		"download_total": downloadTotal,
		"voice_seconds":  voiceSeconds,
		"voice_minutes":  voiceMinutes,
		"sms_count":      smsCount,
	}
}

func firstConsumptionClientName(usage gin.H) string {
	results, ok := usage["results"].([]easy2use.DetailedConsumptionContract)
	if !ok {
		return ""
	}
	for _, result := range results {
		if strings.TrimSpace(result.ClientName) != "" {
			return strings.TrimSpace(result.ClientName)
		}
	}
	return ""
}

func providerPhoneNumber(value string) string {
	digits := config.OnlyDigits(value)
	if len(digits) == 13 && strings.HasPrefix(digits, "55") {
		return digits[2:]
	}
	return digits
}

func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "sim":
		return true
	default:
		return false
	}
}

func attendanceSearchType(query string, searchType string) string {
	searchType = strings.TrimSpace(searchType)
	if searchType != "" && searchType != "auto" {
		return searchType
	}
	digits := config.OnlyDigits(query)
	switch {
	case len(digits) >= 18:
		return "iccid"
	case len(digits) == 11 || len(digits) == 14:
		return "document"
	default:
		return "name"
	}
}

func recentPeriods(period string, count int) []string {
	parsed, err := time.Parse("2006-01", period)
	if err != nil {
		parsed = time.Now()
	}
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, parsed.AddDate(0, -i, 0).Format("2006-01"))
	}
	return out
}

func filterAttendanceResults(subscribers []easy2use.Subscriber, query string, searchType string, limit int) []gin.H {
	clean := normalizeSearch(query)
	digits := config.OnlyDigits(query)
	searchType = attendanceSearchType(query, searchType)
	results := []gin.H{}
	seen := map[string]bool{}
	for _, subscriber := range subscribers {
		document := config.OnlyDigits(subscriber.Document)
		name := strings.ToLower(subscriberSearchText(subscriber))
		for _, contract := range subscriber.Contracts {
			simCard := strings.TrimSpace(contract.SimCard)
			if simCard == "" || seen[simCard] {
				continue
			}
			matches := false
			switch searchType {
			case "iccid":
				matches = strings.Contains(strings.ToLower(simCard), clean)
			case "document", "cpf_cnpj":
				matches = digits != "" && strings.Contains(document, digits)
			case "name":
				matches = clean != "" && strings.Contains(name, clean)
			default:
				matches = strings.Contains(strings.ToLower(simCard), clean) || (digits != "" && strings.Contains(document, digits)) || strings.Contains(name, clean)
			}
			if !matches {
				continue
			}
			seen[simCard] = true
			results = append(results, attendanceResult(subscriber, contract))
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
}

func attendanceResult(subscriber easy2use.Subscriber, contract easy2use.Contract) gin.H {
	name := subscriberDisplayName(subscriber)
	return gin.H{
		"subscriber_name":  name,
		"cnpj":             config.OnlyDigits(subscriber.Document),
		"document":         strings.TrimSpace(subscriber.Document),
		"person_type":      strings.TrimSpace(subscriber.PersonType),
		"sim_card":         strings.TrimSpace(contract.SimCard),
		"phone_number":     strings.TrimSpace(contract.PhoneLine),
		"contract_number":  strings.TrimSpace(contract.ContractNumber),
		"contract_status":  strings.TrimSpace(contract.Status),
		"plan_name":        strings.TrimSpace(contract.Plan),
		"recharge_allowed": strings.EqualFold(strings.TrimSpace(contract.Status), "EM USO"),
	}
}

func subscriberDisplayName(subscriber easy2use.Subscriber) string {
	for _, value := range []string{
		subscriber.Name,
		subscriber.LegalName,
		subscriber.TradeName,
		subscriber.ClientName,
		subscriber.CompanyName,
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func subscriberSearchText(subscriber easy2use.Subscriber) string {
	return normalizeSearch(strings.Join([]string{
		subscriber.Name,
		subscriber.LegalName,
		subscriber.TradeName,
		subscriber.ClientName,
		subscriber.CompanyName,
	}, " "))
}

func normalizeSearch(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "ê", "e", "è", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c",
	)
	return replacer.Replace(value)
}

func attendanceResultFromConsumption(contract easy2use.DetailedConsumptionContract) gin.H {
	status := strings.TrimSpace(contract.SimCardStatus)
	return gin.H{
		"subscriber_name":  strings.TrimSpace(contract.ClientName),
		"cnpj":             config.OnlyDigits(contract.ClientDocument),
		"document":         strings.TrimSpace(contract.ClientDocument),
		"person_type":      "",
		"sim_card":         strings.TrimSpace(contract.ICCID),
		"phone_number":     strings.TrimSpace(contract.Line),
		"contract_number":  strings.TrimSpace(contract.ContractNumber),
		"contract_status":  status,
		"plan_name":        strings.TrimSpace(contract.Plan),
		"recharge_allowed": strings.EqualFold(status, "EM USO"),
	}
}

type addBalanceRequest struct {
	Quantity int  `json:"quantity"`
	DryRun   bool `json:"dry_run"`
}

func (s *Server) addBalanceManual(c *gin.Context) {
	var req addBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	result, status, err := s.addBalance(c.Request.Context(), c.Param("iccid"), req.Quantity, "manual", req.DryRun)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error(), "operation": result})
		return
	}
	c.JSON(http.StatusOK, result)
}

type checkRechargesRequest struct {
	DryRun          bool `json:"dry_run"`
	CreateApprovals bool `json:"create_approvals"`
}

func (s *Server) checkRecharges(c *gin.Context) {
	var req checkRechargesRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
			return
		}
	}

	ctx := c.Request.Context()
	runID, err := s.store.CreateAutomationRun(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	due, err := s.store.ListDueICCIDs(ctx, time.Now())
	if err != nil {
		_ = s.store.FinishAutomationRun(ctx, runID, "failed", 0, 0, 0, 1, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.CreateApprovals {
		created := 0
		existing := 0
		results := []gin.H{}
		for _, item := range due {
			approval, wasCreated, err := s.store.UpsertPendingApproval(ctx, item, "ICCID dentro da janela de recarga preventiva")
			if err != nil {
				_ = s.store.FinishAutomationRun(ctx, runID, "failed", len(due), 0, 0, 1, err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if wasCreated {
				created++
			} else {
				existing++
			}
			results = append(results, gin.H{
				"approval": approval,
				"created":  wasCreated,
			})
		}
		summaryBytes, _ := json.Marshal(results)
		_ = s.store.FinishAutomationRun(ctx, runID, "approval_pending", len(due), 0, existing, 0, string(summaryBytes))
		c.JSON(http.StatusOK, gin.H{
			"run_id":             runID,
			"checked":            len(due),
			"created_approvals":  created,
			"existing_approvals": existing,
			"results":            results,
			"automation_state":   "approval_pending",
		})
		return
	}
	if !req.DryRun && !s.cfg.EnableRealRecharge {
		_ = s.store.FinishAutomationRun(ctx, runID, "blocked", len(due), 0, 0, 0, "real recharge is disabled")
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "real recharge is disabled",
			"hint":    "Use dry_run=true para testar ou configure ENABLE_REAL_RECHARGE=true no .env para permitir recarga real.",
			"checked": len(due),
		})
		return
	}

	recharged := 0
	failed := 0
	skipped := 0
	results := []gin.H{}

	for _, item := range due {
		if req.DryRun {
			skipped++
			results = append(results, gin.H{
				"sim_card":             item.SimCard,
				"cnpj":                 item.CNPJ,
				"subscriber_name":      item.SubscriberName,
				"contract_status":      item.ContractStatus,
				"last_recharge_at":     item.LastRechargeAt,
				"quantity":             item.DefaultQuantity,
				"next_recharge_due_at": item.NextRechargeDueAt,
				"dry_run":              true,
			})
			continue
		}
		result, _, err := s.addBalance(ctx, item.SimCard, item.DefaultQuantity, "automation", false)
		if err != nil {
			failed++
			results = append(results, gin.H{
				"sim_card":        item.SimCard,
				"cnpj":            item.CNPJ,
				"subscriber_name": item.SubscriberName,
				"error":           err.Error(),
				"operation":       result,
			})
			continue
		}
		recharged++
		results = append(results, gin.H{
			"sim_card":        item.SimCard,
			"cnpj":            item.CNPJ,
			"subscriber_name": item.SubscriberName,
			"operation":       result,
		})
	}

	status := "success"
	if failed > 0 && recharged > 0 {
		status = "partial"
	} else if failed > 0 {
		status = "failed"
	}
	summaryBytes, _ := json.Marshal(results)
	_ = s.store.FinishAutomationRun(ctx, runID, status, len(due), recharged, skipped, failed, string(summaryBytes))

	c.JSON(http.StatusOK, gin.H{
		"run_id":           runID,
		"dry_run":          req.DryRun,
		"checked":          len(due),
		"recharged":        recharged,
		"skipped":          skipped,
		"failed":           failed,
		"results":          results,
		"automation_state": status,
	})
}

func (s *Server) nextRun(c *gin.Context) {
	now := time.Now()
	next, actionable, err := s.store.NextRun(c.Request.Context(), now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	due, err := s.store.ListDueICCIDs(c.Request.Context(), now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	nextICCIDs := []domain.ICCID{}
	if next != nil {
		nextICCIDs, err = s.store.ListNextRunICCIDs(c.Request.Context(), *next)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"today":                   now.Format("2006-01-02"),
		"next_recharge_due_at":    next,
		"iccids_due_count":        len(due),
		"actionable_iccids_count": actionable,
		"next_recharge_iccids":    nextICCIDs,
	})
}

func (s *Server) listApprovals(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	items, err := s.store.ListApprovals(c.Request.Context(), c.Query("status"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) approveRecharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval id"})
		return
	}
	ctx := c.Request.Context()
	approval, err := s.store.GetApproval(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if approval.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "approval is not pending", "approval": approval})
		return
	}
	if !s.cfg.EnableRealRecharge {
		c.JSON(http.StatusForbidden, gin.H{
			"error":    "real recharge is disabled",
			"hint":     "Configure ENABLE_REAL_RECHARGE=true no .env para aprovar e executar recarga real.",
			"approval": approval,
		})
		return
	}
	if err := s.store.MarkApprovalApproved(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.MarkApprovalProcessing(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result, status, err := s.addBalance(ctx, approval.SimCard, approval.Quantity, "approval", false)
	operationID := operationIDFromResult(result)
	if err != nil {
		_ = s.store.FinishApproval(ctx, id, "failed", operationID)
		c.JSON(status, gin.H{"error": err.Error(), "approval_id": id, "operation": result})
		return
	}
	_ = s.store.FinishApproval(ctx, id, "success", operationID)
	c.JSON(http.StatusOK, gin.H{"approval_id": id, "operation": result, "status": "success"})
}

func (s *Server) rejectRecharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval id"})
		return
	}
	if err := s.store.RejectApproval(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"approval_id": id, "status": "rejected"})
}

func (s *Server) forceDueDev(c *gin.Context) {
	if !s.cfg.EnableDevRoutes {
		c.JSON(http.StatusNotFound, gin.H{"error": "dev routes are disabled"})
		return
	}
	item, err := s.store.ForceDueToday(c.Request.Context(), strings.TrimSpace(c.Param("iccid")), time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "iccid not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "ICCID marcado como elegivel para recarga hoje apenas no banco local",
		"iccid":   item,
	})
}

type forceStatusRequest struct {
	Status string `json:"status"`
}

func (s *Server) forceStatusDev(c *gin.Context) {
	if !s.cfg.EnableDevRoutes {
		c.JSON(http.StatusNotFound, gin.H{"error": "dev routes are disabled"})
		return
	}
	var req forceStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}
	item, err := s.store.ForceContractStatus(c.Request.Context(), strings.TrimSpace(c.Param("iccid")), status)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "iccid not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Status do ICCID alterado apenas no banco local para teste",
		"iccid":   item,
	})
}

func (s *Server) listOperations(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	ops, err := s.store.ListOperations(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": ops})
}

func (s *Server) websocket(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			if strings.HasPrefix(origin, "http://127.0.0.1:") || strings.HasPrefix(origin, "http://localhost:") {
				return true
			}
			for _, allowed := range s.cfg.CORSAllowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		},
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		snapshot := gin.H{"type": "heartbeat", "time": time.Now()}
		if err := conn.WriteJSON(snapshot); err != nil {
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) addBalance(ctx context.Context, simCard string, quantity int, triggerType string, dryRun bool) (gin.H, int, error) {
	simCard = strings.TrimSpace(simCard)
	if simCard == "" {
		return nil, http.StatusBadRequest, errors.New("iccid is required")
	}
	if quantity < 1 {
		return nil, http.StatusBadRequest, errors.New("quantity must be at least 1")
	}

	item, err := s.store.GetICCID(ctx, simCard)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, http.StatusForbidden, errors.New("iccid not found in local allowed database; run /sync/assinantes first")
	}
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	allowed, err := s.store.IsAllowedCNPJ(ctx, item.CNPJ)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if !allowed {
		return nil, http.StatusForbidden, errors.New("iccid belongs to a non-allowed cnpj")
	}
	if strings.EqualFold(strings.TrimSpace(item.ContractStatus), "CANCELADO") {
		return nil, http.StatusForbidden, errors.New("contract is cancelled")
	}
	if triggerType == "automation" && !item.AutoRechargeEnabled {
		return nil, http.StatusForbidden, errors.New("auto recharge is disabled for this iccid")
	}
	if !dryRun && !s.cfg.EnableRealRecharge {
		return gin.H{
			"sim_card":        item.SimCard,
			"cnpj":            item.CNPJ,
			"subscriber_name": item.SubscriberName,
			"dry_run_hint":    "envie {\"quantity\":1,\"dry_run\":true} para simular sem chamar o provedor",
		}, http.StatusForbidden, errors.New("real recharge is disabled; set ENABLE_REAL_RECHARGE=true to allow provider calls")
	}

	if dryRun {
		return gin.H{
			"dry_run":         true,
			"sim_card":        item.SimCard,
			"cnpj":            item.CNPJ,
			"subscriber_name": item.SubscriberName,
			"contract_status": item.ContractStatus,
			"quantity":        quantity,
			"status":          "dry_run",
			"message":         "Simulacao concluida. Nenhuma chamada foi enviada ao provedor.",
		}, http.StatusOK, nil
	}

	requestPayload := fmt.Sprintf(`{"quantity":%d}`, quantity)
	opID, err := s.store.CreateOperation(ctx, domain.GBOperation{
		SimCard:        item.SimCard,
		CNPJ:           item.CNPJ,
		Quantity:       quantity,
		Status:         "pending",
		TriggerType:    triggerType,
		RequestPayload: requestPayload,
	})
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	resp, raw, statusCode, err := s.provider.AddBalance(ctx, item.SimCard, quantity)
	code := statusCode
	responsePayload := string(raw)
	if err != nil {
		_ = s.store.FinishOperation(ctx, opID, "failed", &code, "", responsePayload, err.Error())
		return gin.H{
			"operation_id":            opID,
			"sim_card":                item.SimCard,
			"cnpj":                    item.CNPJ,
			"subscriber_name":         item.SubscriberName,
			"contract_status":         item.ContractStatus,
			"provider_status_code":    statusCode,
			"provider_response_body":  responsePayload,
			"provider_error_message":  err.Error(),
			"provider_request_target": "saldo/adicionar",
		}, http.StatusBadGateway, err
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		_ = s.store.FinishOperation(ctx, opID, "failed", &code, resp.UserMessage, responsePayload, "provider returned non-success codigo_status_tip")
		return gin.H{
			"operation_id":           opID,
			"sim_card":               item.SimCard,
			"cnpj":                   item.CNPJ,
			"subscriber_name":        item.SubscriberName,
			"contract_status":        item.ContractStatus,
			"provider_status_code":   statusCode,
			"provider_response":      resp,
			"provider_response_body": responsePayload,
		}, http.StatusBadGateway, errors.New("provider returned non-success codigo_status_tip")
	}

	now := time.Now()
	nextRecharge := domain.ComputeNextRecharge(now, item.RechargeIntervalMonths, item.SafetyWindowDays)
	if err := s.store.UpdateLastRecharge(ctx, item.SimCard, now, item.RechargeIntervalMonths, item.SafetyWindowDays); err != nil {
		_ = s.store.FinishOperation(ctx, opID, "failed", &code, resp.UserMessage, responsePayload, err.Error())
		return gin.H{"operation_id": opID}, http.StatusInternalServerError, err
	}
	if err := s.store.FinishOperation(ctx, opID, "success", &code, resp.UserMessage, responsePayload, ""); err != nil {
		return gin.H{"operation_id": opID}, http.StatusInternalServerError, err
	}

	return gin.H{
		"operation_id":         opID,
		"sim_card":             item.SimCard,
		"cnpj":                 item.CNPJ,
		"subscriber_name":      item.SubscriberName,
		"contract_status":      item.ContractStatus,
		"quantity":             quantity,
		"status":               "success",
		"last_recharge_at":     now,
		"next_recharge_due_at": nextRecharge,
		"provider_response":    resp,
	}, http.StatusOK, nil
}

func operationIDFromResult(result gin.H) *int64 {
	if result == nil {
		return nil
	}
	switch value := result["operation_id"].(type) {
	case int64:
		return &value
	case int:
		id := int64(value)
		return &id
	case float64:
		id := int64(value)
		return &id
	default:
		return nil
	}
}
