package services

import (
	"context"

	db "cf-multi-tunnel/backend/internal/repositories/db"
)

type DashboardService struct {
	accounts *db.AccountRepository
	tunnels  *db.TunnelRepository
	apps     *db.AppRepository
	users    *db.UserRepository
}

func NewDashboardService(accounts *db.AccountRepository, tunnels *db.TunnelRepository, apps *db.AppRepository, users *db.UserRepository) *DashboardService {
	return &DashboardService{accounts: accounts, tunnels: tunnels, apps: apps, users: users}
}

type DashboardStats struct {
	Accounts   int64 `json:"accounts"`
	Apps       int64 `json:"apps"`
	AppsSynced int64 `json:"apps_synced"`
	AppsError  int64 `json:"apps_error"`
	Users      int64 `json:"users"`

	// Local tunnels are hosted by this panel; remote ones live on another
	// device and are only observed through the Cloudflare API.
	Tunnels         int64 `json:"tunnels"`
	TunnelsRunning  int64 `json:"tunnels_running"`
	TunnelsError    int64 `json:"tunnels_error"`
	LocalTunnels    int64 `json:"local_tunnels"`
	LocalRunning    int64 `json:"local_running"`
	RemoteTunnels   int64 `json:"remote_tunnels"`
	RemoteConnected int64 `json:"remote_connected"`

	ConnectorMode   string `json:"connector_mode"`
	DockerAvailable bool   `json:"docker_available"`
}

func (s *DashboardService) Stats(ctx context.Context, caller Caller) (*DashboardStats, error) {
	actor := caller.Actor()
	stats := &DashboardStats{}

	var err error
	if stats.Accounts, err = s.accounts.Count(ctx, actor); err != nil {
		return nil, err
	}
	if stats.Tunnels, err = s.tunnels.Count(ctx, actor); err != nil {
		return nil, err
	}
	if stats.TunnelsRunning, err = s.tunnels.CountByStatus(ctx, actor, "running"); err != nil {
		return nil, err
	}
	if stats.TunnelsError, err = s.tunnels.CountByStatus(ctx, actor, "error"); err != nil {
		return nil, err
	}
	if stats.LocalTunnels, err = s.tunnels.CountByLocation(ctx, actor, "local"); err != nil {
		return nil, err
	}
	if stats.LocalRunning, err = s.tunnels.CountByLocationStatus(ctx, actor, "local", "running"); err != nil {
		return nil, err
	}
	if stats.RemoteTunnels, err = s.tunnels.CountByLocation(ctx, actor, "remote"); err != nil {
		return nil, err
	}
	if stats.RemoteConnected, err = s.tunnels.CountByLocationStatus(ctx, actor, "remote", "running"); err != nil {
		return nil, err
	}
	if stats.Apps, err = s.apps.Count(ctx, actor); err != nil {
		return nil, err
	}
	if caller.Admin {
		if stats.Users, err = s.users.Count(ctx); err != nil {
			return nil, err
		}
	}
	return stats, nil
}
