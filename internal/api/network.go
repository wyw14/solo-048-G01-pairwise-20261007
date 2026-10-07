package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (a *App) listZones(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.Network.List())
}

func (a *App) setPumpStage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Stage int    `json:"stage"`
		Actor string `json:"actor"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	zone, command, err := a.Pumps.SetStage(r.Context(), chi.URLParam(r, "zoneID"), input.Stage, input.Actor)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"zone": zone, "command": command})
}

func (a *App) setBypass(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ValveID string `json:"valve_id"`
		Open    bool   `json:"open"`
		Actor   string `json:"actor"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	zone, command, err := a.Valves.SetBypass(r.Context(), chi.URLParam(r, "zoneID"), input.ValveID, input.Open, input.Actor)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"zone": zone, "command": command})
}

func (a *App) isolateSection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Actor string `json:"actor"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.Isolation.Isolate(r.Context(), chi.URLParam(r, "sectionID"), input.Actor)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (a *App) overview(w http.ResponseWriter, _ *http.Request) {
	auditSummary, _ := a.AuditQuery.Summary(time.Now().Add(-24 * time.Hour))
	eventCount, _ := a.Events.Count()
	interlocks := make(map[string]any)
	pumpCommands := make(map[string]any)
	sections := make(map[string]any)
	for _, zone := range a.Network.List() {
		candidate, _ := a.Interlock.Snapshot(zone.ID)
		interlocks[zone.ID] = candidate
		pumpCommands[zone.ID] = a.Pumps.CommandsForZone(zone.ID)
		sections[zone.ID] = a.Topology.SectionsForZone(zone.ID)
	}
	// 分支泄压/边界阀实时状态全部按目录枚举，不再硬编码单个分支的设备 ID。
	reliefOpen := make(map[string]bool)
	boundariesClosed := make(map[string]bool)
	for _, branch := range a.Catalog.Branches() {
		reliefOpen[branch.Relief] = a.Relief.IsOpen(branch.Relief)
		boundariesClosed[branch.SupplyBoundary] = a.Boundaries.IsClosed(branch.SupplyBoundary)
		boundariesClosed[branch.ReturnBoundary] = a.Boundaries.IsClosed(branch.ReturnBoundary)
	}
	stationDetails := make(map[string]any)
	for _, item := range a.Stations.List() {
		lastCommand, _ := a.StationCommands.Last(item.ID)
		stationDetails[item.ID] = map[string]any{
			"flow_receipts":         a.FlowReceipts.ForStation(item.ID),
			"last_command":          lastCommand,
			"dispatch_pending":      a.DispatchBuffer.Pending(item.ID),
			"antifreeze_cycles":     a.Circulation.Cycles(item.ID),
			"frost_valve_available": a.Frost.Available(item.ID + "-return"),
		}
	}
	primaryAdmission, primaryOpen := a.Primary.Admission("xincheng", "B")
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog_version": a.Catalog.Version(),
		"zones":           a.Network.List(), "stations": a.Stations.List(), "incidents": a.Protection.List(),
		"isolations": a.Isolation.List(), "plans": a.Planner.Plans(), "audit_summary": auditSummary,
		"event_count": eventCount, "hydraulic_threshold": a.Balancer.Threshold(),
		"interlocks": interlocks, "pump_commands": pumpCommands, "sections": sections,
		"station_details": stationDetails, "hydraulic_incidents": a.LeakDetector.Incidents(),
		"relief_open": reliefOpen, "boundaries_closed": boundariesClosed,
		"target_primary_admission": map[string]any{"open": primaryOpen, "proof": primaryAdmission},
	})
}
