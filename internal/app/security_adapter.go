package app

import (
	"fmt"
	"net/http"
)

type appSecurityService struct {
	app *App
}

func (s appSecurityService) ChangePassword(r *http.Request) (SessionResponse, error) {
	if !s.app.isAuthenticated(r) {
		return SessionResponse{Authenticated: false}, fmt.Errorf("请先登录")
	}

	payload, err := decodePasswordChangeRequest(r)
	if err != nil {
		return SessionResponse{Authenticated: true}, err
	}
	if !s.app.verifyPassword(payload.CurrentPassword) {
		return SessionResponse{Authenticated: true}, fmt.Errorf("当前管理员密码错误")
	}

	passwordHash, err := HashPassword(payload.NewPassword)
	if err != nil {
		return SessionResponse{Authenticated: true}, err
	}
	if err := s.app.saveAdminPasswordHash(r.Context(), passwordHash); err != nil {
		return SessionResponse{Authenticated: true}, fmt.Errorf("保存管理员密码失败: %w", err)
	}

	s.app.mu.Lock()
	s.app.passwordHash = passwordHash
	s.app.sessionToken = ""
	s.app.mu.Unlock()

	return SessionResponse{Authenticated: false}, nil
}

func (s appSecurityService) ReloadIPData(_ *http.Request) (IPDataStatus, error) {
	return s.app.reloadIPDataFromSettings(), nil
}
