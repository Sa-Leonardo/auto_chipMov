package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"chipmov/internal/domain"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	db *sql.DB
}

func Open(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	schema := []string{
		`create table if not exists allowed_cnpjs (
			id bigserial primary key,
			cnpj text not null unique,
			name text,
			active boolean not null default true,
			created_at timestamptz not null default now()
		)`,
		`create table if not exists iccids (
			id bigserial primary key,
			cnpj text not null,
			subscriber_name text,
			sim_card text not null unique,
			phone_number text,
			contract_number text,
			contract_status text,
			plan_name text,
			last_recharge_at date,
			next_recharge_due_at date,
			default_quantity integer not null default 1,
			recharge_interval_months integer not null default 11,
			safety_window_days integer not null default 10,
			auto_recharge_enabled boolean not null default true,
			last_sync_at timestamptz not null,
			created_at timestamptz not null,
			updated_at timestamptz not null
		)`,
		`create index if not exists idx_iccids_cnpj on iccids(cnpj)`,
		`create index if not exists idx_iccids_contract_status on iccids(contract_status)`,
		`create index if not exists idx_iccids_next_recharge_due_at on iccids(next_recharge_due_at)`,
		`create table if not exists gb_operations (
			id bigserial primary key,
			sim_card text not null,
			cnpj text,
			quantity integer not null,
			status text not null,
			trigger_type text not null,
			easy2use_status_code integer,
			easy2use_user_message text,
			request_payload text,
			response_payload text,
			error_message text,
			created_at timestamptz not null,
			finished_at timestamptz
		)`,
		`create index if not exists idx_gb_operations_sim_card on gb_operations(sim_card)`,
		`create index if not exists idx_gb_operations_status on gb_operations(status)`,
		`create index if not exists idx_gb_operations_created_at on gb_operations(created_at desc)`,
		`create table if not exists automation_runs (
			id bigserial primary key,
			started_at timestamptz not null,
			finished_at timestamptz,
			status text not null,
			checked_count integer not null default 0,
			recharged_count integer not null default 0,
			skipped_count integer not null default 0,
			failed_count integer not null default 0,
			summary text
		)`,
		`create table if not exists last_recharge_syncs (
			id bigserial primary key,
			started_at timestamptz not null,
			finished_at timestamptz,
			status text not null,
			items_found integer not null default 0,
			items_updated integer not null default 0,
			error_message text
		)`,
		`create table if not exists recharge_approvals (
			id bigserial primary key,
			sim_card text not null,
			cnpj text not null,
			subscriber_name text,
			contract_status text,
			quantity integer not null,
			status text not null,
			reason text,
			last_recharge_at date,
			next_recharge_due_at date,
			operation_id bigint references gb_operations(id),
			created_at timestamptz not null,
			approved_at timestamptz,
			rejected_at timestamptz,
			finished_at timestamptz
		)`,
		`create unique index if not exists idx_recharge_approvals_open_sim_card
			on recharge_approvals(sim_card)
			where status in ('pending', 'approved', 'processing')`,
		`create index if not exists idx_recharge_approvals_status on recharge_approvals(status)`,
		`create index if not exists idx_recharge_approvals_created_at on recharge_approvals(created_at desc)`,
		`create table if not exists users (
			id bigserial primary key,
			name text not null,
			email text not null unique,
			password_hash text not null,
			role text not null check (role in ('admin', 'supervisor', 'operator', 'viewer')),
			active boolean not null default true,
			last_login_at timestamptz,
			created_at timestamptz not null,
			updated_at timestamptz not null
		)`,
		`create index if not exists idx_users_role on users(role)`,
		`create table if not exists refresh_tokens (
			id bigserial primary key,
			user_id bigint not null references users(id) on delete cascade,
			token_hash text not null unique,
			expires_at timestamptz not null,
			revoked_at timestamptz,
			created_at timestamptz not null
		)`,
		`create index if not exists idx_refresh_tokens_user_id on refresh_tokens(user_id)`,
		`create table if not exists audit_logs (
			id bigserial primary key,
			user_id bigint references users(id) on delete set null,
			action text not null,
			resource text not null,
			resource_id text,
			ip text,
			user_agent text,
			metadata text,
			created_at timestamptz not null
		)`,
		`create index if not exists idx_audit_logs_created_at on audit_logs(created_at desc)`,
		`create index if not exists idx_audit_logs_user_id on audit_logs(user_id)`,
	}
	for _, statement := range schema {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertAllowedCNPJs(ctx context.Context, cnpjs []string) error {
	for _, cnpj := range cnpjs {
		if _, err := s.db.ExecContext(ctx, `insert into allowed_cnpjs (cnpj, active, created_at)
			values ($1, true, $2)
			on conflict(cnpj) do update set active = true`, cnpj, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) IsAllowedCNPJ(ctx context.Context, cnpj string) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, `select active from allowed_cnpjs where cnpj = $1`, cnpj).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return active, nil
}

type UpsertICCIDParams struct {
	CNPJ                   string
	SubscriberName         string
	SimCard                string
	PhoneNumber            string
	ContractNumber         string
	ContractStatus         string
	PlanName               string
	DefaultQuantity        int
	RechargeIntervalMonths int
	SafetyWindowDays       int
}

func (s *Store) UpsertICCID(ctx context.Context, p UpsertICCIDParams) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `insert into iccids (
			cnpj, subscriber_name, sim_card, phone_number, contract_number, contract_status,
			plan_name, default_quantity, recharge_interval_months, safety_window_days,
			auto_recharge_enabled, last_sync_at, created_at, updated_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true, $11, $12, $13)
		on conflict(sim_card) do update set
			cnpj = excluded.cnpj,
			subscriber_name = excluded.subscriber_name,
			phone_number = excluded.phone_number,
			contract_number = excluded.contract_number,
			contract_status = excluded.contract_status,
			plan_name = excluded.plan_name,
			default_quantity = excluded.default_quantity,
			recharge_interval_months = excluded.recharge_interval_months,
			safety_window_days = excluded.safety_window_days,
			last_sync_at = excluded.last_sync_at,
			updated_at = excluded.updated_at`,
		p.CNPJ, p.SubscriberName, p.SimCard, p.PhoneNumber, p.ContractNumber, p.ContractStatus,
		p.PlanName, p.DefaultQuantity, p.RechargeIntervalMonths, p.SafetyWindowDays, now, now, now)
	return err
}

func (s *Store) ListICCIDs(ctx context.Context) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids order by next_recharge_due_at is null desc, next_recharge_due_at asc, sim_card asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

func (s *Store) ICCIDSummary(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `select cnpj, coalesce(contract_status, ''), count(*)
		from iccids
		group by cnpj, contract_status
		order by cnpj asc, contract_status asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var cnpj string
		var status string
		var count int
		if err := rows.Scan(&cnpj, &status, &count); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"cnpj":            cnpj,
			"contract_status": status,
			"count":           count,
		})
	}
	return items, rows.Err()
}

func (s *Store) ListDueICCIDs(ctx context.Context, now time.Time) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids
		where auto_recharge_enabled = true
		  and next_recharge_due_at is not null
		  and next_recharge_due_at <= $1
		  and upper(trim(contract_status)) = 'EM USO'
		order by next_recharge_due_at asc`, dateOnly(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

func (s *Store) GetICCID(ctx context.Context, simCard string) (domain.ICCID, error) {
	row := s.db.QueryRowContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids where sim_card = $1`, simCard)
	return scanICCID(row)
}

func (s *Store) UpdateLastRecharge(ctx context.Context, simCard string, lastRecharge time.Time, intervalMonths int, safetyWindowDays int) error {
	next := domain.ComputeNextRecharge(lastRecharge, intervalMonths, safetyWindowDays)
	_, err := s.db.ExecContext(ctx, `update iccids set last_recharge_at = $1, next_recharge_due_at = $2, updated_at = $3 where sim_card = $4`,
		dateOnly(lastRecharge), dateOnly(next), time.Now().UTC(), simCard)
	return err
}

func (s *Store) ForceDueToday(ctx context.Context, simCard string, now time.Time) (domain.ICCID, error) {
	result, err := s.db.ExecContext(ctx, `update iccids set next_recharge_due_at = $1, updated_at = $2 where sim_card = $3`,
		dateOnly(now), time.Now().UTC(), simCard)
	if err != nil {
		return domain.ICCID{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.ICCID{}, err
	}
	if affected == 0 {
		return domain.ICCID{}, sql.ErrNoRows
	}
	return s.GetICCID(ctx, simCard)
}

func (s *Store) ForceContractStatus(ctx context.Context, simCard string, status string) (domain.ICCID, error) {
	result, err := s.db.ExecContext(ctx, `update iccids set contract_status = $1, updated_at = $2 where sim_card = $3`,
		status, time.Now().UTC(), simCard)
	if err != nil {
		return domain.ICCID{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.ICCID{}, err
	}
	if affected == 0 {
		return domain.ICCID{}, sql.ErrNoRows
	}
	return s.GetICCID(ctx, simCard)
}

func (s *Store) CreateOperation(ctx context.Context, op domain.GBOperation) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `insert into gb_operations (
		sim_card, cnpj, quantity, status, trigger_type, request_payload, created_at
	) values ($1, $2, $3, $4, $5, $6, $7) returning id`,
		op.SimCard, op.CNPJ, op.Quantity, op.Status, op.TriggerType, op.RequestPayload, time.Now().UTC()).Scan(&id)
	return id, err
}

func (s *Store) FinishOperation(ctx context.Context, id int64, status string, statusCode *int, userMessage string, responsePayload string, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `update gb_operations set status = $1, easy2use_status_code = $2, easy2use_user_message = $3,
		response_payload = $4, error_message = $5, finished_at = $6 where id = $7`,
		status, statusCode, userMessage, responsePayload, errorMessage, time.Now().UTC(), id)
	return err
}

func (s *Store) ListOperations(ctx context.Context, limit int) ([]domain.GBOperation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `select id, sim_card, coalesce(cnpj, ''), quantity, status, trigger_type, easy2use_status_code,
		coalesce(easy2use_user_message, ''), coalesce(request_payload, ''), coalesce(response_payload, ''), coalesce(error_message, ''), created_at, finished_at
		from gb_operations order by id desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ops := []domain.GBOperation{}
	for rows.Next() {
		var op domain.GBOperation
		var code sql.NullInt64
		var created, finished sql.NullTime
		if err := rows.Scan(&op.ID, &op.SimCard, &op.CNPJ, &op.Quantity, &op.Status, &op.TriggerType, &code,
			&op.Easy2UseUserMessage, &op.RequestPayload, &op.ResponsePayload, &op.ErrorMessage, &created, &finished); err != nil {
			return nil, err
		}
		if code.Valid {
			c := int(code.Int64)
			op.Easy2UseStatusCode = &c
		}
		if created.Valid {
			op.CreatedAt = created.Time
		}
		if finished.Valid {
			op.FinishedAt = &finished.Time
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *Store) ListOperationsBySimCard(ctx context.Context, simCard string, limit int) ([]domain.GBOperation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `select id, sim_card, coalesce(cnpj, ''), quantity, status, trigger_type, easy2use_status_code,
		coalesce(easy2use_user_message, ''), coalesce(request_payload, ''), coalesce(response_payload, ''), coalesce(error_message, ''), created_at, finished_at
		from gb_operations where sim_card = $1 order by id desc limit $2`, simCard, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOperations(rows)
}

func (s *Store) CreateAutomationRun(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `insert into automation_runs (started_at, status) values ($1, 'running') returning id`, time.Now().UTC()).Scan(&id)
	return id, err
}

func (s *Store) FinishAutomationRun(ctx context.Context, id int64, status string, checked int, recharged int, skipped int, failed int, summary string) error {
	_, err := s.db.ExecContext(ctx, `update automation_runs set finished_at = $1, status = $2, checked_count = $3, recharged_count = $4,
		skipped_count = $5, failed_count = $6, summary = $7 where id = $8`,
		time.Now().UTC(), status, checked, recharged, skipped, failed, summary, id)
	return err
}

func (s *Store) UpsertPendingApproval(ctx context.Context, item domain.ICCID, reason string) (domain.RechargeApproval, bool, error) {
	var lastRecharge any
	if item.LastRechargeAt != nil {
		lastRecharge = dateOnly(*item.LastRechargeAt)
	}
	var nextDue any
	if item.NextRechargeDueAt != nil {
		nextDue = dateOnly(*item.NextRechargeDueAt)
	}
	result, err := s.db.ExecContext(ctx, `insert into recharge_approvals (
			sim_card, cnpj, subscriber_name, contract_status, quantity, status, reason,
			last_recharge_at, next_recharge_due_at, created_at
		) values ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, $9)
		on conflict (sim_card) where status in ('pending', 'approved', 'processing') do nothing`,
		item.SimCard, item.CNPJ, item.SubscriberName, item.ContractStatus, item.DefaultQuantity,
		reason, lastRecharge, nextDue, time.Now().UTC())
	if err != nil {
		return domain.RechargeApproval{}, false, err
	}
	created, _ := result.RowsAffected()
	approval, err := s.GetOpenApprovalBySimCard(ctx, item.SimCard)
	return approval, created > 0, err
}

func (s *Store) ListApprovals(ctx context.Context, status string, limit int) ([]domain.RechargeApproval, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{}
	query := `select id, sim_card, cnpj, coalesce(subscriber_name, ''), coalesce(contract_status, ''), quantity, status, coalesce(reason, ''),
		last_recharge_at, next_recharge_due_at, operation_id, created_at, approved_at, rejected_at, finished_at
		from recharge_approvals`
	if strings.TrimSpace(status) != "" {
		args = append(args, status)
		query += fmt.Sprintf(` where status = $%d`, len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(` order by created_at desc, id desc limit $%d`, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApprovals(rows)
}

func (s *Store) GetApproval(ctx context.Context, id int64) (domain.RechargeApproval, error) {
	row := s.db.QueryRowContext(ctx, `select id, sim_card, cnpj, coalesce(subscriber_name, ''), coalesce(contract_status, ''), quantity, status, coalesce(reason, ''),
		last_recharge_at, next_recharge_due_at, operation_id, created_at, approved_at, rejected_at, finished_at
		from recharge_approvals where id = $1`, id)
	return scanApproval(row)
}

func (s *Store) GetOpenApprovalBySimCard(ctx context.Context, simCard string) (domain.RechargeApproval, error) {
	row := s.db.QueryRowContext(ctx, `select id, sim_card, cnpj, coalesce(subscriber_name, ''), coalesce(contract_status, ''), quantity, status, coalesce(reason, ''),
		last_recharge_at, next_recharge_due_at, operation_id, created_at, approved_at, rejected_at, finished_at
		from recharge_approvals where sim_card = $1 and status in ('pending', 'approved', 'processing') order by id desc limit 1`, simCard)
	return scanApproval(row)
}

func (s *Store) MarkApprovalApproved(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = 'approved', approved_at = $1 where id = $2 and status = 'pending'`,
		time.Now().UTC(), id)
	return err
}

func (s *Store) MarkApprovalProcessing(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = 'processing' where id = $1 and status = 'approved'`, id)
	return err
}

func (s *Store) FinishApproval(ctx context.Context, id int64, status string, operationID *int64) error {
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = $1, operation_id = $2, finished_at = $3 where id = $4`,
		status, operationID, time.Now().UTC(), id)
	return err
}

func (s *Store) RejectApproval(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = 'rejected', rejected_at = $1, finished_at = $2 where id = $3 and status = 'pending'`,
		now, now, id)
	return err
}

func (s *Store) NextRun(ctx context.Context, now time.Time) (*time.Time, int, error) {
	var value sql.NullTime
	var count int
	err := s.db.QueryRowContext(ctx, `select min(next_recharge_due_at), count(*) from iccids
		where auto_recharge_enabled = true
		  and next_recharge_due_at is not null
		  and next_recharge_due_at >= $1
		  and upper(trim(contract_status)) = 'EM USO'`, dateOnly(now)).Scan(&value, &count)
	if err != nil {
		return nil, 0, err
	}
	if !value.Valid {
		return nil, count, nil
	}
	t := dateOnly(value.Time)
	return &t, count, nil
}

func (s *Store) ListNextRunICCIDs(ctx context.Context, next time.Time) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids
		where auto_recharge_enabled = true
		  and next_recharge_due_at = $1
		  and upper(trim(contract_status)) = 'EM USO'
		order by cnpj asc, sim_card asc`, dateOnly(next))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

func (s *Store) UpsertBootstrapAdmin(ctx context.Context, name string, email string, passwordHash string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `insert into users (name, email, password_hash, role, active, created_at, updated_at)
		values ($1, $2, $3, 'admin', true, $4, $5)
		on conflict(email) do update set
			name = excluded.name,
			password_hash = excluded.password_hash,
			role = 'admin',
			active = true,
			updated_at = excluded.updated_at`,
		name, email, passwordHash, now, now)
	return err
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	row := s.db.QueryRowContext(ctx, `select id, name, email, password_hash, role, active, last_login_at, created_at, updated_at
		from users where lower(email) = lower($1)`, email)
	return scanUser(row)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (domain.User, error) {
	row := s.db.QueryRowContext(ctx, `select id, name, email, password_hash, role, active, last_login_at, created_at, updated_at
		from users where id = $1`, id)
	return scanUser(row)
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `select id, name, email, password_hash, role, active, last_login_at, created_at, updated_at
		from users order by name asc, email asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.User{}
	for rows.Next() {
		item, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	now := time.Now().UTC()
	var id int64
	err := s.db.QueryRowContext(ctx, `insert into users (name, email, password_hash, role, active, created_at, updated_at)
		values ($1, lower($2), $3, $4, $5, $6, $7) returning id`,
		user.Name, user.Email, user.PasswordHash, string(user.Role), user.Active, now, now).Scan(&id)
	if err != nil {
		return domain.User{}, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) UpdateUser(ctx context.Context, user domain.User) (domain.User, error) {
	_, err := s.db.ExecContext(ctx, `update users set name = $1, role = $2, active = $3, updated_at = $4 where id = $5`,
		user.Name, string(user.Role), user.Active, time.Now().UTC(), user.ID)
	if err != nil {
		return domain.User{}, err
	}
	return s.GetUserByID(ctx, user.ID)
}

func (s *Store) UpdateUserPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `update users set password_hash = $1, updated_at = $2 where id = $3`,
		passwordHash, time.Now().UTC(), id)
	return err
}

func (s *Store) MarkUserLogin(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `update users set last_login_at = $1, updated_at = $2 where id = $3`,
		time.Now().UTC(), time.Now().UTC(), id)
	return err
}

func (s *Store) CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `insert into refresh_tokens (user_id, token_hash, expires_at, created_at)
		values ($1, $2, $3, $4)`, userID, tokenHash, expiresAt.UTC(), time.Now().UTC())
	return err
}

func (s *Store) GetRefreshToken(ctx context.Context, tokenHash string) (domain.RefreshToken, error) {
	row := s.db.QueryRowContext(ctx, `select id, user_id, token_hash, expires_at, revoked_at, created_at
		from refresh_tokens where token_hash = $1`, tokenHash)
	return scanRefreshToken(row)
}

func (s *Store) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `update refresh_tokens set revoked_at = $1 where token_hash = $2 and revoked_at is null`,
		time.Now().UTC(), tokenHash)
	return err
}

func (s *Store) RevokeUserRefreshTokens(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `update refresh_tokens set revoked_at = $1 where user_id = $2 and revoked_at is null`,
		time.Now().UTC(), userID)
	return err
}

func (s *Store) CreateAuditLog(ctx context.Context, log domain.AuditLog) error {
	_, err := s.db.ExecContext(ctx, `insert into audit_logs (user_id, action, resource, resource_id, ip, user_agent, metadata, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		log.UserID, log.Action, log.Resource, log.ResourceID, log.IP, log.UserAgent, log.Metadata, time.Now().UTC())
	return err
}

func (s *Store) ListAuditLogs(ctx context.Context, limit int) ([]domain.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `select id, user_id, action, resource, coalesce(resource_id, ''), coalesce(ip, ''),
		coalesce(user_agent, ''), coalesce(metadata, ''), created_at
		from audit_logs order by id desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AuditLog{}
	for rows.Next() {
		var item domain.AuditLog
		var userID sql.NullInt64
		if err := rows.Scan(&item.ID, &userID, &item.Action, &item.Resource, &item.ResourceID, &item.IP, &item.UserAgent, &item.Metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		if userID.Valid {
			id := userID.Int64
			item.UserID = &id
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListAuditLogsFiltered(ctx context.Context, resource string, resourceID string, limit int) ([]domain.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{}
	query := `select id, user_id, action, resource, coalesce(resource_id, ''), coalesce(ip, ''),
		coalesce(user_agent, ''), coalesce(metadata, ''), created_at
		from audit_logs`
	clauses := []string{}
	if strings.TrimSpace(resource) != "" {
		args = append(args, resource)
		clauses = append(clauses, fmt.Sprintf("resource = $%d", len(args)))
	}
	if strings.TrimSpace(resourceID) != "" {
		args = append(args, resourceID)
		clauses = append(clauses, fmt.Sprintf("resource_id = $%d", len(args)))
	}
	if len(clauses) > 0 {
		query += " where " + strings.Join(clauses, " and ")
	}
	args = append(args, limit)
	query += fmt.Sprintf(" order by id desc limit $%d", len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanICCID(row scanner) (domain.ICCID, error) {
	var item domain.ICCID
	var lastRecharge, nextDue, lastSync, created, updated sql.NullTime
	var subscriberName, phoneNumber, contractNumber, contractStatus, planName sql.NullString
	err := row.Scan(&item.ID, &item.CNPJ, &subscriberName, &item.SimCard, &phoneNumber, &contractNumber,
		&contractStatus, &planName, &lastRecharge, &nextDue, &item.DefaultQuantity,
		&item.RechargeIntervalMonths, &item.SafetyWindowDays, &item.AutoRechargeEnabled, &lastSync, &created, &updated)
	if err != nil {
		return item, err
	}
	item.SubscriberName = subscriberName.String
	item.PhoneNumber = phoneNumber.String
	item.ContractNumber = contractNumber.String
	item.ContractStatus = contractStatus.String
	item.PlanName = planName.String
	if lastRecharge.Valid {
		t := dateOnly(lastRecharge.Time)
		item.LastRechargeAt = &t
	}
	if nextDue.Valid {
		t := dateOnly(nextDue.Time)
		item.NextRechargeDueAt = &t
	}
	if lastSync.Valid {
		item.LastSyncAt = lastSync.Time
	}
	if created.Valid {
		item.CreatedAt = created.Time
	}
	if updated.Valid {
		item.UpdatedAt = updated.Time
	}
	return item, nil
}

func scanICCIDs(rows *sql.Rows) ([]domain.ICCID, error) {
	items := []domain.ICCID{}
	for rows.Next() {
		item, err := scanICCID(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanOperations(rows *sql.Rows) ([]domain.GBOperation, error) {
	ops := []domain.GBOperation{}
	for rows.Next() {
		var op domain.GBOperation
		var code sql.NullInt64
		var created, finished sql.NullTime
		if err := rows.Scan(&op.ID, &op.SimCard, &op.CNPJ, &op.Quantity, &op.Status, &op.TriggerType, &code,
			&op.Easy2UseUserMessage, &op.RequestPayload, &op.ResponsePayload, &op.ErrorMessage, &created, &finished); err != nil {
			return nil, err
		}
		if code.Valid {
			c := int(code.Int64)
			op.Easy2UseStatusCode = &c
		}
		if created.Valid {
			op.CreatedAt = created.Time
		}
		if finished.Valid {
			op.FinishedAt = &finished.Time
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func scanApproval(row scanner) (domain.RechargeApproval, error) {
	var item domain.RechargeApproval
	var lastRecharge, nextDue, created, approved, rejected, finished sql.NullTime
	var operationID sql.NullInt64
	err := row.Scan(&item.ID, &item.SimCard, &item.CNPJ, &item.SubscriberName, &item.ContractStatus,
		&item.Quantity, &item.Status, &item.Reason, &lastRecharge, &nextDue, &operationID,
		&created, &approved, &rejected, &finished)
	if err != nil {
		return item, err
	}
	if lastRecharge.Valid {
		t := dateOnly(lastRecharge.Time)
		item.LastRechargeAt = &t
	}
	if nextDue.Valid {
		t := dateOnly(nextDue.Time)
		item.NextRechargeDueAt = &t
	}
	if operationID.Valid {
		id := operationID.Int64
		item.OperationID = &id
	}
	if created.Valid {
		item.CreatedAt = created.Time
	}
	if approved.Valid {
		item.ApprovedAt = &approved.Time
	}
	if rejected.Valid {
		item.RejectedAt = &rejected.Time
	}
	if finished.Valid {
		item.FinishedAt = &finished.Time
	}
	return item, nil
}

func scanApprovals(rows *sql.Rows) ([]domain.RechargeApproval, error) {
	items := []domain.RechargeApproval{}
	for rows.Next() {
		item, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanUser(row scanner) (domain.User, error) {
	var item domain.User
	var role string
	var lastLogin sql.NullTime
	err := row.Scan(&item.ID, &item.Name, &item.Email, &item.PasswordHash, &role, &item.Active, &lastLogin, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.Role = domain.UserRole(role)
	if lastLogin.Valid {
		item.LastLoginAt = &lastLogin.Time
	}
	return item, nil
}

func scanRefreshToken(row scanner) (domain.RefreshToken, error) {
	var item domain.RefreshToken
	var revoked sql.NullTime
	err := row.Scan(&item.ID, &item.UserID, &item.TokenHash, &item.ExpiresAt, &revoked, &item.CreatedAt)
	if err != nil {
		return item, err
	}
	if revoked.Valid {
		item.RevokedAt = &revoked.Time
	}
	return item, nil
}

func scanAuditLogs(rows *sql.Rows) ([]domain.AuditLog, error) {
	items := []domain.AuditLog{}
	for rows.Next() {
		var item domain.AuditLog
		var userID sql.NullInt64
		if err := rows.Scan(&item.ID, &userID, &item.Action, &item.Resource, &item.ResourceID, &item.IP, &item.UserAgent, &item.Metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		if userID.Valid {
			id := userID.Int64
			item.UserID = &id
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func dateOnly(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
