package services

import (
	"context"
	"errors"
	"strings"

	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

type UserService struct {
	users *db.UserRepository
	audit *AuditService
}

func NewUserService(users *db.UserRepository, audit *AuditService) *UserService {
	return &UserService{users: users, audit: audit}
}

type CreateUserInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

type UpdateUserInput struct {
	Name     *string `json:"name"`
	Role     *string `json:"role"`
	IsActive *bool   `json:"is_active"`
	Password *string `json:"password"`
}

func (s *UserService) List(ctx context.Context, search string, p db.Pagination) ([]models.PublicUser, int64, error) {
	users, total, err := s.users.List(ctx, search, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.PublicUser, 0, len(users))
	for i := range users {
		out = append(out, users[i].Public())
	}
	return out, total, nil
}

func (s *UserService) Get(ctx context.Context, id string) (*models.PublicUser, error) {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, NotFound("user not found")
		}
		return nil, err
	}
	pub := user.Public()
	return &pub, nil
}

func (s *UserService) Create(ctx context.Context, caller Caller, in CreateUserInput) (*models.PublicUser, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	fields := map[string]string{}
	if email == "" {
		fields["email"] = "required"
	}
	if len(in.Password) < 8 {
		fields["password"] = "must be at least 8 characters"
	}
	if strings.TrimSpace(in.Name) == "" {
		fields["name"] = "required"
	}
	role := in.Role
	if role == "" {
		role = models.RoleUser
	}
	if role != models.RoleAdmin && role != models.RoleUser {
		fields["role"] = "must be admin or user"
	}
	if len(fields) > 0 {
		return nil, Validation(fields)
	}

	if _, err := s.users.FindByEmail(ctx, email); err == nil {
		return nil, Conflict("a user with that email already exists")
	} else if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	hashed, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	user := &models.User{
		Email:    email,
		Password: hashed,
		Name:     strings.TrimSpace(in.Name),
		Role:     role,
		IsActive: true,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "user.create",
		Entity: "user", EntityID: user.ID, Detail: email,
	})
	pub := user.Public()
	return &pub, nil
}

func (s *UserService) Update(ctx context.Context, caller Caller, id string, in UpdateUserInput) (*models.PublicUser, error) {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, NotFound("user not found")
		}
		return nil, err
	}

	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, Validation(map[string]string{"name": "required"})
		}
		user.Name = strings.TrimSpace(*in.Name)
	}
	if in.Role != nil {
		if *in.Role != models.RoleAdmin && *in.Role != models.RoleUser {
			return nil, Validation(map[string]string{"role": "must be admin or user"})
		}
		if user.ID == caller.ID && *in.Role != models.RoleAdmin {
			return nil, Forbidden("you cannot remove your own admin role")
		}
		// Never allow the last admin to be demoted: the panel would be locked out.
		if user.Role == models.RoleAdmin && *in.Role != models.RoleAdmin {
			admins, err := s.users.CountByRole(ctx, models.RoleAdmin)
			if err != nil {
				return nil, err
			}
			if admins <= 1 {
				return nil, Forbidden("cannot demote the last admin")
			}
		}
		user.Role = *in.Role
	}
	if in.IsActive != nil {
		if user.ID == caller.ID && !*in.IsActive {
			return nil, Forbidden("you cannot deactivate your own account")
		}
		user.IsActive = *in.IsActive
	}
	if in.Password != nil {
		if len(*in.Password) < 8 {
			return nil, Validation(map[string]string{"password": "must be at least 8 characters"})
		}
		hashed, err := HashPassword(*in.Password)
		if err != nil {
			return nil, err
		}
		user.Password = hashed
	}

	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "user.update",
		Entity: "user", EntityID: user.ID,
	})
	pub := user.Public()
	return &pub, nil
}

func (s *UserService) Delete(ctx context.Context, caller Caller, id string) error {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return NotFound("user not found")
		}
		return err
	}
	if user.ID == caller.ID {
		return Forbidden("you cannot delete your own account")
	}
	if user.Role == models.RoleAdmin {
		admins, err := s.users.CountByRole(ctx, models.RoleAdmin)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return Forbidden("cannot delete the last admin")
		}
	}
	if err := s.users.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "user.delete",
		Entity: "user", EntityID: id, Detail: user.Email,
	})
	return nil
}
