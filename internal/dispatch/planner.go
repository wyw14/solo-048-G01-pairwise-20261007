package dispatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"heatpulse/internal/assets"
	"heatpulse/internal/station"
)

type Planner struct {
	targets *station.TargetService
	catalog *assets.Catalog
	mu      sync.RWMutex
	plans   map[string]Plan
}

// NewPlanner 基于唯一资产目录构建调度计划器：计划目标必须是目录内站点，
// 每份计划都携带编制时的目录版本，发布与页面展示引用同一版本。
func NewPlanner(targets *station.TargetService, catalog *assets.Catalog) *Planner {
	return &Planner{targets: targets, catalog: catalog, plans: make(map[string]Plan)}
}

// CatalogVersion 返回调度器引用的目录版本。
func (p *Planner) CatalogVersion() assets.Version { return p.catalog.Version() }

func (p *Planner) Begin(ctx context.Context, requested map[string]float64, delay time.Duration) (Plan, error) {
	if len(requested) == 0 {
		return Plan{}, fmt.Errorf("at least one station target is required")
	}
	for stationID := range requested {
		if _, ok := p.catalog.Station(stationID); !ok {
			return Plan{}, fmt.Errorf("station %s not found in asset catalog", stationID)
		}
	}
	plan := Plan{
		ID: uuid.NewString(), SourceRevision: p.targets.Revision(), CatalogVersion: p.catalog.Version(),
		Targets: make(map[string]float64, len(requested)),
		Status:  PlanCalculating, CreatedAt: time.Now().UTC(),
	}
	for stationID, celsius := range requested {
		plan.Targets[stationID] = celsius
	}
	p.save(plan)
	select {
	case <-ctx.Done():
		plan.Status = PlanFailed
		plan.Message = ctx.Err().Error()
		p.save(plan)
		return plan, ctx.Err()
	case <-time.After(delay):
	}
	return plan, nil
}

func (p *Planner) save(plan Plan) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plans[plan.ID] = plan.Clone()
}

func (p *Planner) Plan(planID string) (Plan, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	plan, ok := p.plans[planID]
	return plan.Clone(), ok
}

func (p *Planner) Plans() []Plan {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]Plan, 0, len(p.plans))
	for _, plan := range p.plans {
		result = append(result, plan.Clone())
	}
	return result
}
