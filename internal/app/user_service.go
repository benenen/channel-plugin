package app

import (
	"context"

	"github.com/benenen/channel-plugin/internal/domain"
)

type UserService struct {
	users domain.UserRepository
}

func NewUserService(users domain.UserRepository) *UserService {
	return &UserService{users: users}
}

func (s *UserService) ResolveUser(ctx context.Context, externalUserID string) (domain.User, error) {
	return s.users.FindOrCreateByExternalUserID(ctx, externalUserID)
}
