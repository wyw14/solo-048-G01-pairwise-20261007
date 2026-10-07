package dispatch

import (
	"fmt"
	"time"

	"heatpulse/internal/assets"
	"heatpulse/internal/audit"
	"heatpulse/internal/station"
)

type Publisher struct {
	planner *Planner
	targets *station.TargetService
	catalog *assets.Catalog
	audit   *audit.Recorder
}

func NewPublisher(planner *Planner, targets *station.TargetService, catalog *assets.Catalog, recorder *audit.Recorder) *Publisher {
	return &Publisher{planner: planner, targets: targets, catalog: catalog, audit: recorder}
}

func (p *Publisher) Publish(planID, actor string) (Plan, error) {
	plan, ok := p.planner.Plan(planID)
	if !ok {
		return Plan{}, fmt.Errorf("plan %s not found", planID)
	}
	if plan.Status != PlanCalculating {
		return plan, fmt.Errorf("plan %s is %s", plan.ID, plan.Status)
	}
	// 目录在计划编制后若发生修订，则该计划基于过期拓扑，拒绝发布并标记为 stale。
	if current := p.catalog.Version(); plan.CatalogVersion != current {
		err := fmt.Errorf("plan %s catalog version %s is stale (current %s)", plan.ID, plan.CatalogVersion.String(), current.String())
		plan.Status = PlanStale
		plan.Message = err.Error()
		p.planner.save(plan)
		_ = p.audit.Rejected("dispatch.publish", plan.ID, actor, err.Error(), map[string]any{"plan_catalog": plan.CatalogVersion.String(), "current_catalog": current.String()})
		return plan, err
	}
	if err := p.targets.CompareAndPublish(plan.SourceRevision, plan.Targets); err != nil {
		plan.Status = PlanStale
		plan.Message = err.Error()
		p.planner.save(plan)
		_ = p.audit.Rejected("dispatch.publish", plan.ID, actor, err.Error(), map[string]any{"source_revision": plan.SourceRevision})
		return plan, err
	}
	plan.Status = PlanPublished
	plan.PublishedAt = time.Now().UTC()
	p.planner.save(plan)
	if err := p.audit.Accepted("dispatch.publish", plan.ID, actor, "dispatch targets published", map[string]any{"source_revision": plan.SourceRevision, "catalog": plan.CatalogVersion.String()}); err != nil {
		return plan, err
	}
	return plan, nil
}
