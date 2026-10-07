package dispatch

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"heatpulse/internal/network"
	"heatpulse/internal/station"
)

type Planner struct {
	catalog *network.Catalog
	targets *station.TargetService
	mu      sync.RWMutex
	plans   map[string]Plan
}

func NewPlanner(catalog *network.Catalog, targets *station.TargetService) *Planner {
	return &Planner{catalog: catalog, targets: targets, plans: make(map[string]Plan)}
}

// CatalogVersion 返回调度计划引用的资产目录版本。
func (p *Planner) CatalogVersion() string {
	return p.catalog.Version()
}

func (p *Planner) Begin(ctx context.Context, requested map[string]float64, delay time.Duration) (Plan, error) {
	if len(requested) == 0 {
		return Plan{}, fmt.Errorf("at least one station target is required")
	}
	stationIDs := make([]string, 0, len(requested))
	for stationID := range requested {
		stationIDs = append(stationIDs, stationID)
	}
	sort.Strings(stationIDs)
	for _, stationID := range stationIDs {
		if _, ok := p.catalog.StationZone(stationID); !ok {
			return Plan{}, fmt.Errorf("station %s not present in catalog %s", stationID, p.catalog.Version())
		}
	}
	plan := Plan{
		ID: uuid.NewString(), CatalogVersion: p.catalog.Version(), SourceRevision: p.targets.Revision(),
		Targets: make(map[string]float64, len(requested)),
		Status:  PlanCalculating, CreatedAt: time.Now().UTC(),
	}
	for _, stationID := range stationIDs {
		plan.Targets[stationID] = requested[stationID]
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
