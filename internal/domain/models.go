package domain

import "time"

type ICCID struct {
	ID                     int64      `json:"id"`
	CNPJ                   string     `json:"cnpj"`
	SubscriberName         string     `json:"subscriber_name"`
	SimCard                string     `json:"sim_card"`
	PhoneNumber            string     `json:"phone_number"`
	ContractNumber         string     `json:"contract_number"`
	ContractStatus         string     `json:"contract_status"`
	PlanName               string     `json:"plan_name"`
	LastRechargeAt         *time.Time `json:"last_recharge_at,omitempty"`
	NextRechargeDueAt      *time.Time `json:"next_recharge_due_at,omitempty"`
	DefaultQuantity        int        `json:"default_quantity"`
	RechargeIntervalMonths int        `json:"recharge_interval_months"`
	SafetyWindowDays       int        `json:"safety_window_days"`
	AutoRechargeEnabled    bool       `json:"auto_recharge_enabled"`
	LastSyncAt             time.Time  `json:"last_sync_at"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type GBOperation struct {
	ID                  int64      `json:"id"`
	SimCard             string     `json:"sim_card"`
	CNPJ                string     `json:"cnpj"`
	Quantity            int        `json:"quantity"`
	Status              string     `json:"status"`
	TriggerType         string     `json:"trigger_type"`
	Easy2UseStatusCode  *int       `json:"easy2use_status_code,omitempty"`
	Easy2UseUserMessage string     `json:"easy2use_user_message,omitempty"`
	RequestPayload      string     `json:"request_payload,omitempty"`
	ResponsePayload     string     `json:"response_payload,omitempty"`
	ErrorMessage        string     `json:"error_message,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
}

type AutomationRun struct {
	ID             int64      `json:"id"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Status         string     `json:"status"`
	CheckedCount   int        `json:"checked_count"`
	RechargedCount int        `json:"recharged_count"`
	SkippedCount   int        `json:"skipped_count"`
	FailedCount    int        `json:"failed_count"`
	Summary        string     `json:"summary,omitempty"`
}

type RechargeApproval struct {
	ID                int64      `json:"id"`
	SimCard           string     `json:"sim_card"`
	CNPJ              string     `json:"cnpj"`
	SubscriberName    string     `json:"subscriber_name"`
	ContractStatus    string     `json:"contract_status"`
	Quantity          int        `json:"quantity"`
	Status            string     `json:"status"`
	Reason            string     `json:"reason"`
	LastRechargeAt    *time.Time `json:"last_recharge_at,omitempty"`
	NextRechargeDueAt *time.Time `json:"next_recharge_due_at,omitempty"`
	OperationID       *int64     `json:"operation_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ApprovedAt        *time.Time `json:"approved_at,omitempty"`
	RejectedAt        *time.Time `json:"rejected_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
}

type UserRole string

const (
	RoleAdmin      UserRole = "admin"
	RoleSupervisor UserRole = "supervisor"
	RoleOperator   UserRole = "operator"
	RoleViewer     UserRole = "viewer"
)

type User struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Role         UserRole   `json:"role"`
	Active       bool       `json:"active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	PasswordHash string     `json:"-"`
}

type RefreshToken struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type AuditLog struct {
	ID         int64     `json:"id"`
	UserID     *int64    `json:"user_id,omitempty"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	ResourceID string    `json:"resource_id,omitempty"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	Metadata   string    `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type RechargeDecision struct {
	ICCID  ICCID  `json:"iccid"`
	Reason string `json:"reason"`
}

func ComputeNextRecharge(lastRechargeAt time.Time, intervalMonths int, safetyWindowDays int) time.Time {
	return lastRechargeAt.AddDate(0, intervalMonths, -safetyWindowDays)
}
