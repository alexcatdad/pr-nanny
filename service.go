package main

import (
	"context"
	"sync"

	appcore "github.com/alexcatdad/pr-nanny/internal/app"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Service is the entire application-specific renderer API. Infrastructure and
// lifecycle controls are deliberately not promoted from the controller.
type Service struct {
	controller *appcore.Controller
	native     *nativePlatform
	smoke      bool
	ready      chan struct{}
	readyOnce  sync.Once
}

func (s *Service) GetSnapshot() (model.Snapshot, error) {
	s.readyOnce.Do(func() {
		if s.ready != nil {
			close(s.ready)
		}
	})
	return s.controller.GetSnapshot()
}
func (s *Service) SaveSettings(v model.Settings) (model.Snapshot, error) {
	return s.controller.SaveSettings(v)
}
func (s *Service) ConnectPAT(token string) (model.Snapshot, error) {
	if s.smoke {
		return model.Snapshot{}, appcore.ErrAuthentication
	}
	return s.controller.ConnectPAT(token)
}
func (s *Service) ConnectCLI() (model.Snapshot, error) {
	if s.smoke {
		return model.Snapshot{}, appcore.ErrAuthentication
	}
	return s.controller.ConnectCLI()
}
func (s *Service) SetBell(id string, v bool) (model.Snapshot, error) {
	return s.controller.SetBell(id, v)
}
func (s *Service) SetLayout(v string) (model.Snapshot, error) { return s.controller.SetLayout(v) }
func (s *Service) Refresh() error {
	if s.smoke {
		return nil
	}
	return s.controller.Refresh()
}
func (s *Service) OpenPR(id string) error { return s.controller.OpenPR(id) }
func (s *Service) HidePopup()             { s.controller.HidePopup() }
func (s *Service) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	if !s.smoke {
		s.native.startNotifications(ctx, options)
		s.controller.ApplyStartup()
		go s.controller.Run()
	}
	return nil
}
func (s *Service) ServiceShutdown() error {
	s.controller.Stop()
	if !s.smoke {
		_ = s.native.notifications.ServiceShutdown()
	}
	return nil
}
