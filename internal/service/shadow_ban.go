package service

import (
	"context"
	"fmt"
	"strings"
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
	records, err := s.List(ctx)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if strings.TrimSpace(user.Trip) != "" && user.Trip == record.Trip {
			return true, nil
		}
		if user.Name != "" && user.Name == record.Name {
			return true, nil
		}
		if user.Hash != "" && user.Hash == record.Hash {
			return true, nil
		}
	}
	return false, nil
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
func (s *ShadowBanService) Remove(ctx context.Context, target string) error {
	r, err := s.repository()
	if err != nil {
		return err
	}
	changed, err := r.RemoveShadowBanBySourceTarget(ctx, target)
	if err == nil && changed > 0 {
		common.RecordCommittedMutation(ctx)
	}
	return err
}
func (s *ShadowBanService) RemoveAll(ctx context.Context) error {
	r, err := s.repository()
	if err != nil {
		return err
	}
	changed, err := r.RemoveAllShadowBans(ctx)
	if err == nil && changed > 0 {
		common.RecordCommittedMutation(ctx)
	}
	return err
}
