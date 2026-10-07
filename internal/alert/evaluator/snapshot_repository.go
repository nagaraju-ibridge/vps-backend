package evaluator

import (
	"context"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/alert/models"
	appModels "vpsmonitoring-backend/internal/application/models"
	metricModels "vpsmonitoring-backend/internal/metric/models"
	serverModels "vpsmonitoring-backend/internal/server/models"
)

type GORMSnapshotProvider struct{ db *gorm.DB }

func NewGORMSnapshotProvider(db *gorm.DB) *GORMSnapshotProvider { return &GORMSnapshotProvider{db: db} }

func uniqueTargets(rules []models.AlertRule) ([]int64, []int64) {
	servers, apps := map[int64]struct{}{}, map[int64]struct{}{}
	for _, rule := range rules {
		servers[rule.ServerID] = struct{}{}
		if rule.ApplicationID != nil {
			apps[*rule.ApplicationID] = struct{}{}
		}
	}
	serverIDs := make([]int64, 0, len(servers))
	for id := range servers {
		serverIDs = append(serverIDs, id)
	}
	appIDs := make([]int64, 0, len(apps))
	for id := range apps {
		appIDs = append(appIDs, id)
	}
	return serverIDs, appIDs
}

func (p *GORMSnapshotProvider) Load(ctx context.Context, rules []models.AlertRule) (*Snapshot, error) {
	snapshot := &Snapshot{Servers: map[int64]ServerState{}, Applications: map[int64]ApplicationState{}}
	serverIDs, appIDs := uniqueTargets(rules)
	serverOwners := make(map[int64]int64, len(rules))
	for _, rule := range rules {
		serverOwners[rule.ServerID] = rule.UserID
	}
	validServers := map[int64]bool{}
	if len(serverIDs) > 0 {
		var servers []serverModels.Server
		if err := p.db.WithContext(ctx).Where("id IN ?", serverIDs).Find(&servers).Error; err != nil {
			return nil, err
		}
		for _, server := range servers {
			if server.UserID == serverOwners[server.ID] {
				validServers[server.ID] = true
				snapshot.Servers[server.ID] = ServerState{Exists: true, AgentStatus: server.AgentStatus}
			}
		}
		var metrics []metricModels.Metric
		latest := p.db.Table("metrics").Select("server_id, MAX(collected_at) AS collected_at").Where("server_id IN ?", serverIDs).Group("server_id")
		if err := p.db.WithContext(ctx).Preload("Disks").Joins("JOIN (?) latest ON latest.server_id = metrics.server_id AND latest.collected_at = metrics.collected_at", latest).Find(&metrics).Error; err != nil {
			return nil, err
		}
		for i := range metrics {
			state, ok := snapshot.Servers[metrics[i].ServerID]
			if !ok {
				continue
			}
			state.Metric = &metrics[i]
			snapshot.Servers[metrics[i].ServerID] = state
		}
	}
	if len(appIDs) > 0 {
		var apps []appModels.Application
		if err := p.db.WithContext(ctx).Where("id IN ?", appIDs).Find(&apps).Error; err != nil {
			return nil, err
		}
		for _, app := range apps {
			for _, rule := range rules {
				if rule.ApplicationID != nil && *rule.ApplicationID == app.ID && rule.ServerID == app.ServerID && validServers[app.ServerID] {
					snapshot.Applications[app.ID] = ApplicationState{Exists: true}
					break
				}
			}
		}
		var metrics []appModels.ApplicationMetric
		latest := p.db.Table("application_metrics").Select("application_id, MAX(collected_at) AS collected_at").Where("application_id IN ?", appIDs).Group("application_id")
		if err := p.db.WithContext(ctx).Joins("JOIN (?) latest ON latest.application_id = application_metrics.application_id AND latest.collected_at = application_metrics.collected_at", latest).Find(&metrics).Error; err != nil {
			return nil, err
		}
		for i := range metrics {
			state, ok := snapshot.Applications[metrics[i].ApplicationID]
			if !ok {
				continue
			}
			state.Metric = &metrics[i]
			snapshot.Applications[metrics[i].ApplicationID] = state
		}
		var events []appModels.ApplicationEvent
		latestEvents := p.db.Table("application_events").Select("application_id, MAX(event_time) AS event_time").Where("application_id IN ? AND event_type = ?", appIDs, "restart_detected").Group("application_id")
		if err := p.db.WithContext(ctx).Joins("JOIN (?) latest ON latest.application_id = application_events.application_id AND latest.event_time = application_events.event_time", latestEvents).Where("application_events.event_type = ?", "restart_detected").Order("application_events.application_id ASC, application_events.id DESC").Find(&events).Error; err != nil {
			return nil, err
		}
		for _, event := range events {
			state, ok := snapshot.Applications[event.ApplicationID]
			if !ok {
				continue
			}
			if state.LatestRestartID == 0 {
				state.LatestRestartID = event.ID
				snapshot.Applications[event.ApplicationID] = state
			}
		}
	}
	return snapshot, nil
}
