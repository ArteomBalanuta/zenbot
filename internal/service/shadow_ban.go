package service

import (
	"context"
	"fmt"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

// ShadowBanService exposes only the local source-compatible records required
// by public moderation commands. It has no agent integration.
type ShadowBanService struct {
	Repo repository.ShadowBanCommandRepository
}

func (s *ShadowBanService) Matches(ctx context.Context, user *model.User) (bool, error) {
	if user == nil {
		return false, nil
	}
	r, err := s.repository()
	if err != nil {
		return false, err
	}
	return r.HasShadowBanMatch(ctx, user.Trip, user.Name, user.Hash)
}

func (s *ShadowBanService) repository() (repository.ShadowBanCommandRepository, error) {
	if s == nil || s.Repo == nil {
		return nil, fmt.Errorf("shadow-ban repository unavailable")
	}
	return s.Repo, nil
}

func (s *ShadowBanService) Persist(ctx context.Context, record repository.ShadowBanRecord) error {
	r, err := s.repository()
	if err != nil {
		return err
	}
	err = r.PersistShadowBanRecord(ctx, record)
	if err == nil {
		common.RecordCommittedMutation(ctx)
	}
	return err
}
func (s *ShadowBanService) List(ctx context.Context) ([]repository.ShadowBanRecord, error) {
	r, err := s.repository()
	if err != nil {
		return nil, err
	}
	return r.ListShadowBans(ctx)
}
func (s *ShadowBanService) Remove(ctx context.Context, target string) (int64, error) {
	r, err := s.repository()
	if err != nil {
		return 0, err
	}
	changed, err := r.RemoveShadowBanBySourceTarget(ctx, target)
	if err == nil && changed > 0 {
		common.RecordCommittedMutation(ctx)
	}
	return changed, err
}
func (s *ShadowBanService) RemoveAll(ctx context.Context) (int64, error) {
	r, err := s.repository()
	if err != nil {
		return 0, err
	}
	changed, err := r.RemoveAllShadowBans(ctx)
	if err == nil && changed > 0 {
		common.RecordCommittedMutation(ctx)
	}
	return changed, err
}
