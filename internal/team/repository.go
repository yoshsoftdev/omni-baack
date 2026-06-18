package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var (
	ErrNotFound       = errors.New("team not found")
	ErrMemberNotFound = errors.New("team member not found")
	ErrAlreadyMember  = errors.New("user is already a team member")
	ErrInvitationExists = errors.New("invitation already pending")
)

type ListMembersFilter struct {
	Search string
	Role   string
}

type Repository interface {
	Create(ctx context.Context, team *Team) error
	FindByID(ctx context.Context, id uuid.UUID) (*Team, error)
	Update(ctx context.Context, team *Team) error
	CreateMember(ctx context.Context, member *Member) error
	FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*Member, error)
	FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*Member, error)
	FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*Member, error)
	ListMembers(ctx context.Context, teamID uuid.UUID) ([]MemberWithUser, error)
	ListMembersFiltered(ctx context.Context, teamID uuid.UUID, filter ListMembersFilter) ([]MemberWithUser, error)
	UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role Role, roleID uuid.UUID) error
	UpdateMemberStatus(ctx context.Context, teamID, memberID uuid.UUID, status MemberStatus) error
	SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error
	FindRoleIDByName(ctx context.Context, name string) (uuid.UUID, error)
	GetWorkspaceDetail(ctx context.Context, teamID uuid.UUID) (*WorkspaceDetail, error)
	CreateInvitation(ctx context.Context, inv *Invitation) error
	ListInvitations(ctx context.Context, teamID uuid.UUID) ([]Invitation, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, team *Team) error {
	query := `
		INSERT INTO teams (id, name, slug, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		team.ID,
		team.Name,
		team.Slug,
		team.CreatedAt,
		team.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("team slug already exists")
		}
		return fmt.Errorf("insert team: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	query := `
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM teams
		WHERE id = $1 AND deleted_at IS NULL
	`
	return r.scanTeam(r.db.Pool.QueryRow(ctx, query, id))
}

func (r *PostgresRepository) Update(ctx context.Context, team *Team) error {
	query := `
		UPDATE teams
		SET name = $1, slug = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, team.Name, team.Slug, team.UpdatedAt, team.ID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("team slug already exists")
		}
		return fmt.Errorf("update team: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateMember(ctx context.Context, member *Member) error {
	query := `
		INSERT INTO team_members (id, team_id, user_id, role, role_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		member.ID,
		member.TeamID,
		member.UserID,
		string(member.Role),
		member.RoleID,
		string(member.Status),
		member.CreatedAt,
		member.UpdatedAt,
	)
	if err != nil {
		if isLegacyTeamSchemaErr(err) {
			legacyQuery := `
				INSERT INTO team_members (id, team_id, user_id, role, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6)
			`
			_, legacyErr := r.db.Pool.Exec(ctx, legacyQuery,
				member.ID,
				member.TeamID,
				member.UserID,
				string(member.Role),
				member.CreatedAt,
				member.UpdatedAt,
			)
			if legacyErr == nil {
				return nil
			}
			err = legacyErr
		}
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrAlreadyMember
		}
		return fmt.Errorf("insert team member: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*Member, error) {
	query := `
		SELECT id, team_id, user_id, role, role_id, status, created_at, updated_at, deleted_at
		FROM team_members
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT 1
	`
	member, err := r.scanMember(r.db.Pool.QueryRow(ctx, query, userID))
	if err == nil {
		return member, nil
	}
	if isLegacyTeamSchemaErr(err) {
		legacyQuery := `
			SELECT id, team_id, user_id, role, created_at, updated_at, deleted_at
			FROM team_members
			WHERE user_id = $1 AND deleted_at IS NULL
			ORDER BY created_at ASC
			LIMIT 1
		`
		return r.scanLegacyMember(r.db.Pool.QueryRow(ctx, legacyQuery, userID))
	}
	return nil, err
}

func (r *PostgresRepository) FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*Member, error) {
	query := `
		SELECT id, team_id, user_id, role, role_id, status, created_at, updated_at, deleted_at
		FROM team_members
		WHERE team_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`
	member, err := r.scanMember(r.db.Pool.QueryRow(ctx, query, teamID, userID))
	if err == nil {
		return member, nil
	}
	if isLegacyTeamSchemaErr(err) {
		legacyQuery := `
			SELECT id, team_id, user_id, role, created_at, updated_at, deleted_at
			FROM team_members
			WHERE team_id = $1 AND user_id = $2 AND deleted_at IS NULL
		`
		return r.scanLegacyMember(r.db.Pool.QueryRow(ctx, legacyQuery, teamID, userID))
	}
	return nil, err
}

func (r *PostgresRepository) FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*Member, error) {
	query := `
		SELECT id, team_id, user_id, role, role_id, status, created_at, updated_at, deleted_at
		FROM team_members
		WHERE team_id = $1 AND id = $2 AND deleted_at IS NULL
	`
	member, err := r.scanMember(r.db.Pool.QueryRow(ctx, query, teamID, memberID))
	if err == nil {
		return member, nil
	}
	if isLegacyTeamSchemaErr(err) {
		legacyQuery := `
			SELECT id, team_id, user_id, role, created_at, updated_at, deleted_at
			FROM team_members
			WHERE team_id = $1 AND id = $2 AND deleted_at IS NULL
		`
		return r.scanLegacyMember(r.db.Pool.QueryRow(ctx, legacyQuery, teamID, memberID))
	}
	return nil, err
}

func (r *PostgresRepository) ListMembers(ctx context.Context, teamID uuid.UUID) ([]MemberWithUser, error) {
	return r.ListMembersFiltered(ctx, teamID, ListMembersFilter{})
}

func (r *PostgresRepository) ListMembersFiltered(ctx context.Context, teamID uuid.UUID, filter ListMembersFilter) ([]MemberWithUser, error) {
	query := `
		SELECT tm.id, tm.user_id, tm.role, tm.role_id, tm.status, tm.created_at, u.email, u.name, u.updated_at
		FROM team_members tm
		INNER JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = $1 AND tm.deleted_at IS NULL AND u.deleted_at IS NULL
	`
	args := []interface{}{teamID}
	argIdx := 2

	if filter.Role != "" {
		query += fmt.Sprintf(" AND tm.role = $%d", argIdx)
		args = append(args, filter.Role)
		argIdx++
	}
	if filter.Search != "" {
		query += fmt.Sprintf(" AND (LOWER(u.name) LIKE $%d OR LOWER(u.email) LIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		argIdx++
	}
	query += " ORDER BY tm.created_at ASC"

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		if isLegacyTeamSchemaErr(err) {
			legacyQuery := `
				SELECT tm.id, tm.user_id, tm.role, tm.created_at, u.email, u.name, u.updated_at
				FROM team_members tm
				INNER JOIN users u ON u.id = tm.user_id
				WHERE tm.team_id = $1 AND tm.deleted_at IS NULL AND u.deleted_at IS NULL
			`
			legacyArgs := []interface{}{teamID}
			legacyArgIdx := 2
			if filter.Role != "" {
				legacyQuery += fmt.Sprintf(" AND tm.role = $%d", legacyArgIdx)
				legacyArgs = append(legacyArgs, filter.Role)
				legacyArgIdx++
			}
			if filter.Search != "" {
				legacyQuery += fmt.Sprintf(" AND (LOWER(u.name) LIKE $%d OR LOWER(u.email) LIKE $%d)", legacyArgIdx, legacyArgIdx)
				legacyArgs = append(legacyArgs, "%"+strings.ToLower(filter.Search)+"%")
				legacyArgIdx++
			}
			legacyQuery += " ORDER BY tm.created_at ASC"

			rows, err = r.db.Pool.Query(ctx, legacyQuery, legacyArgs...)
			if err == nil {
				defer rows.Close()
				var members []MemberWithUser
				for rows.Next() {
					var m MemberWithUser
					var role string
					if scanErr := rows.Scan(&m.ID, &m.UserID, &role, &m.CreatedAt, &m.Email, &m.Name, &m.LastActiveAt); scanErr != nil {
						return nil, fmt.Errorf("scan legacy team member row: %w", scanErr)
					}
					m.Role = Role(role)
					m.Status = MemberStatusActive
					members = append(members, m)
				}
				return members, rows.Err()
			}
		}
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()

	var members []MemberWithUser
	for rows.Next() {
		var m MemberWithUser
		var role, status string
		if err := rows.Scan(
			&m.ID, &m.UserID, &role, &m.RoleID, &status, &m.CreatedAt,
			&m.Email, &m.Name, &m.LastActiveAt,
		); err != nil {
			return nil, fmt.Errorf("scan team member row: %w", err)
		}
		m.Role = Role(role)
		m.Status = MemberStatus(status)
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *PostgresRepository) UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role Role, roleID uuid.UUID) error {
	query := `
		UPDATE team_members
		SET role = $1, role_id = $2, updated_at = NOW()
		WHERE team_id = $3 AND id = $4 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, string(role), roleID, teamID, memberID)
	if err != nil {
		if isLegacyTeamSchemaErr(err) {
			legacyQuery := `
				UPDATE team_members
				SET role = $1, updated_at = NOW()
				WHERE team_id = $2 AND id = $3 AND deleted_at IS NULL
			`
			tag, err = r.db.Pool.Exec(ctx, legacyQuery, string(role), teamID, memberID)
			if err == nil {
				if tag.RowsAffected() == 0 {
					return ErrMemberNotFound
				}
				return nil
			}
		}
		return fmt.Errorf("update member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateMemberStatus(ctx context.Context, teamID, memberID uuid.UUID, status MemberStatus) error {
	query := `
		UPDATE team_members
		SET status = $1, updated_at = NOW()
		WHERE team_id = $2 AND id = $3 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, string(status), teamID, memberID)
	if err != nil {
		if isLegacyTeamSchemaErr(err) {
			// Legacy schema has no status column; keep backward compatibility.
			return nil
		}
		return fmt.Errorf("update member status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error {
	query := `
		UPDATE team_members
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE team_id = $1 AND id = $2 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, teamID, memberID)
	if err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (r *PostgresRepository) FindRoleIDByName(ctx context.Context, name string) (uuid.UUID, error) {
	query := `SELECT id FROM roles WHERE LOWER(name) = LOWER($1) AND deleted_at IS NULL`
	var id uuid.UUID
	err := r.db.Pool.QueryRow(ctx, query, name).Scan(&id)
	if err != nil {
		if isLegacyTeamSchemaErr(err) {
			return uuid.Nil, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("find role id: %w", err)
	}
	return id, nil
}

func (r *PostgresRepository) GetWorkspaceDetail(ctx context.Context, teamID uuid.UUID) (*WorkspaceDetail, error) {
	query := `
		SELECT t.id, t.name, t.slug, t.created_at, u.id, u.name
		FROM teams t
		INNER JOIN team_members tm ON tm.team_id = t.id AND tm.role = 'owner' AND tm.deleted_at IS NULL
		INNER JOIN users u ON u.id = tm.user_id
		WHERE t.id = $1 AND t.deleted_at IS NULL
		LIMIT 1
	`
	var detail WorkspaceDetail
	err := r.db.Pool.QueryRow(ctx, query, teamID).Scan(
		&detail.ID, &detail.Name, &detail.Slug, &detail.CreatedAt, &detail.OwnerID, &detail.OwnerName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workspace detail: %w", err)
	}
	return &detail, nil
}

func (r *PostgresRepository) CreateInvitation(ctx context.Context, inv *Invitation) error {
	query := `
		INSERT INTO team_invitations (id, team_id, email, name, role_id, status, invited_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		inv.ID, inv.TeamID, inv.Email, inv.Name, inv.RoleID, inv.Status, inv.InvitedBy, inv.CreatedAt, inv.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrInvitationExists
		}
		return fmt.Errorf("insert invitation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListInvitations(ctx context.Context, teamID uuid.UUID) ([]Invitation, error) {
	query := `
		SELECT ti.id, ti.team_id, ti.email, ti.name, ti.role_id, ro.name, ti.status, ti.invited_by, ti.created_at
		FROM team_invitations ti
		INNER JOIN roles ro ON ro.id = ti.role_id
		WHERE ti.team_id = $1 AND ti.deleted_at IS NULL AND ti.status = 'pending'
		ORDER BY ti.created_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()

	var invitations []Invitation
	for rows.Next() {
		var inv Invitation
		var roleName string
		if err := rows.Scan(
			&inv.ID, &inv.TeamID, &inv.Email, &inv.Name, &inv.RoleID, &roleName,
			&inv.Status, &inv.InvitedBy, &inv.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		inv.Role = Role(roleName)
		invitations = append(invitations, inv)
	}
	return invitations, rows.Err()
}

func (r *PostgresRepository) scanTeam(row pgx.Row) (*Team, error) {
	var team Team
	var deletedAt *time.Time
	err := row.Scan(
		&team.ID,
		&team.Name,
		&team.Slug,
		&team.CreatedAt,
		&team.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan team: %w", err)
	}
	team.DeletedAt = deletedAt
	return &team, nil
}

func (r *PostgresRepository) scanMember(row pgx.Row) (*Member, error) {
	var member Member
	var role, status string
	var deletedAt *time.Time
	err := row.Scan(
		&member.ID,
		&member.TeamID,
		&member.UserID,
		&role,
		&member.RoleID,
		&status,
		&member.CreatedAt,
		&member.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMemberNotFound
		}
		return nil, fmt.Errorf("scan team member: %w", err)
	}
	member.Role = Role(role)
	member.Status = MemberStatus(status)
	member.DeletedAt = deletedAt
	return &member, nil
}

func (r *PostgresRepository) scanLegacyMember(row pgx.Row) (*Member, error) {
	var member Member
	var role string
	var deletedAt *time.Time
	err := row.Scan(
		&member.ID,
		&member.TeamID,
		&member.UserID,
		&role,
		&member.CreatedAt,
		&member.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMemberNotFound
		}
		return nil, fmt.Errorf("scan legacy team member: %w", err)
	}
	member.Role = Role(role)
	member.Status = MemberStatusActive
	member.DeletedAt = deletedAt
	return &member, nil
}

func isLegacyTeamSchemaErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "role_id") && strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "status") && strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "relation \"roles\" does not exist")
}
