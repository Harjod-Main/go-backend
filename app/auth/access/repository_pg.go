package access

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepoer interface {
	GetUserByEmail(ctx context.Context, hashedEmail string) (User, error)
}

type postgresRepo struct {
	pg *pgxpool.Pool
}

var _ PostgresRepoer = (*postgresRepo)(nil)

func NewPostgresRepo(pg *pgxpool.Pool) PostgresRepoer {
	return &postgresRepo{pg: pg}
}

func (r *postgresRepo) GetUserByEmail(ctx context.Context, hashedEmail string) (User, error) {
	if len(hashedEmail) == 0 {
		return User{}, fmt.Errorf("hashed email is required")
	}

	query := `
		SELECT user_id, username, email, email_hashed, role, country, created_at, updated_at, deleted_at
		FROM "user"
		WHERE email_hashed = $1 AND deleted_at IS NULL
		LIMIT 1
	`

	var user User
	err := r.pg.QueryRow(ctx, query, hashedEmail).Scan(
		&user.UserID,
		&user.Username,
		&user.Email,
		&user.EmailHashed,
		&user.Role,
		&user.Country,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("failed to query user: %w", err)
	}

	return user, nil
}

type UserRole string

const (
	UserRoleAdmin UserRole = "ADMIN"
	UserRoleUser  UserRole = "USER"
	UserRoleBot   UserRole = "BOT"
)

type User struct {
	UserID      uuid.UUID  `db:"user_id" json:"userId"`
	Username    string     `db:"username" json:"username"`
	Email       string     `db:"email" json:"email"`
	EmailHashed string     `db:"email_hashed" json:"emailHashed"`
	Role        *UserRole  `db:"role" json:"role,omitempty"`
	Country     *string    `db:"country" json:"country,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt   *time.Time `db:"updated_at" json:"updatedAt,omitempty"`
	DeletedAt   *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
}

func (u *User) GetID() uuid.UUID {
	return u.UserID
}

type OrganizationMemberRoleType string

const (
	OrganizationMemberRoleAdmin OrganizationMemberRoleType = "ADMIN"
	OrganizationMemberRoleUser  OrganizationMemberRoleType = "USER"
)

type OrganizationMemberStatusType string

const (
	OrganizationMemberStatusActive  OrganizationMemberStatusType = "ACTIVE"
	OrganizationMemberStatusInvited OrganizationMemberStatusType = "INVITED"
	OrganizationMemberStatusRemoved OrganizationMemberStatusType = "REMOVED"
)

type OrganizationMember struct {
	ID             string                       `db:"id" json:"id"`
	OrganizationID string                       `db:"organization_id" json:"organizationId"`
	MemberID       string                       `db:"member_id" json:"memberId"`
	Role           OrganizationMemberRoleType   `db:"role" json:"role"`
	Status         OrganizationMemberStatusType `db:"status" json:"status"`
	JoinedAt       *time.Time                   `db:"joined_at" json:"joinedAt,omitempty"`
	CreatedAt      time.Time                    `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time                    `db:"updated_at" json:"updatedAt"`
	DeletedAt      *time.Time                   `db:"deleted_at" json:"deletedAt,omitempty"`
}

func (o *OrganizationMember) GetOrganizationID() uuid.UUID {
	return uuid.MustParse(o.OrganizationID)
}

func (o *OrganizationMember) GetMemberID() uuid.UUID {
	return uuid.MustParse(o.MemberID)
}
