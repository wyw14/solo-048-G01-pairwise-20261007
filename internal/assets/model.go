// Package assets 维护供热网络资产目录：分区、站点、换热器、泵、阀门与边界泄压设备，
// 以及分支（供/回水）的拓扑关系。目录在进程启动时构建并整体校验，构建成功后不可变，
// 恢复、联锁（安全联锁）、调度与运维各页面统一引用同一份带版本号的目录。
package assets

// Kind 是资产类型标识。
type Kind string

const (
	KindZone      Kind = "zone"      // 分区
	KindStation   Kind = "station"   // 换热站
	KindExchanger Kind = "exchanger" // 换热器
	KindPump      Kind = "pump"      // 循环泵
	KindValve     Kind = "valve"     // 普通阀门（旁通/一次网/手动等）
	KindBoundary  Kind = "boundary"  // 边界隔断阀
	KindRelief    Kind = "relief"    // 边界泄压设备
)

// Conduit 标识分支内的一根管线：供水或回水。
type Conduit string

const (
	Supply Conduit = "supply"
	Return Conduit = "return"
)

// 普通阀门角色。
const (
	RoleBypass  = "bypass"
	RolePrimary = "primary"
	RoleManual  = "manual"
	RoleFrost   = "frost"
)

// Zone 是热力分区（支线/环路）。
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Branches 为本分区下挂的分支 ID。
	Branches []string `json:"branches"`
	// Pumps 为分区级循环泵 ID（参与泵级联锁）。
	Pumps []string `json:"pumps"`
}

// Branch 是一条供回水分支：由供水端、回水端和其间的站点组成，
// 并配置安全隔离器件（供回水两侧边界隔断阀 + 泄压设备）。
type Branch struct {
	ID     string `json:"id"`
	ZoneID string `json:"zone_id"`
	Name   string `json:"name"`
	// 供回水端点：指向 Node.ID，构建期校验必须一供一回且均属于本分支。
	SupplyNode string `json:"supply_node"`
	ReturnNode string `json:"return_node"`
	// 分支沿线接入的换热站。
	Stations []string `json:"stations"`
	// 安全隔离器件：供/回水两侧的边界隔断阀，以及泄压设备。
	SupplyBoundary string `json:"supply_boundary"`
	ReturnBoundary string `json:"return_boundary"`
	Relief         string `json:"relief"`
}

// Node 是供回水端点节点。
type Node struct {
	ID       string  `json:"id"`
	BranchID string  `json:"branch_id"`
	Conduit  Conduit `json:"conduit"`
	Name     string  `json:"name"`
}

// Station 是换热站资产记录。
type Station struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ZoneID     string   `json:"zone_id"`
	BranchID   string   `json:"branch_id"`
	Exchangers []string `json:"exchangers"`
	Valves     []string `json:"valves"`
	Required   bool     `json:"required"` // 分区恢复时必须就位
}

// Exchanger 是换热器资产记录。
type Exchanger struct {
	ID        string `json:"id"`
	StationID string `json:"station_id"`
	// Train 为列编号（A/B…），与联控系统中的 exchanger id 对齐。
	Train string `json:"train"`
	Name  string `json:"name"`
}

// Pump 是循环泵资产记录。
type Pump struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	ZoneID  string `json:"zone_id"`
	Station string `json:"station_id,omitempty"` // 站内泵可选挂站点
	Stage   int    `json:"stage"`                // 当前泵级（目录只做身份与挂接，运行态另存）
}

// Valve 是普通阀门资产记录（非边界安全器件）。
type Valve struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StationID string `json:"station_id"`
	Role      string `json:"role"`
}

// BoundaryValve 是分支边界隔断阀。
type BoundaryValve struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	BranchID string  `json:"branch_id"`
	Conduit  Conduit `json:"conduit"`
}

// ReliefDevice 是分支边界泄压设备。
type ReliefDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BranchID string `json:"branch_id"`
}

// Definition 是构建目录的输入：各表的平面清单，拓扑关系用 ID 引用。
type Definition struct {
	SchemaVersion int             `json:"schema_version"`
	Zones         []Zone          `json:"zones"`
	Branches      []Branch        `json:"branches"`
	Nodes         []Node          `json:"nodes"`
	Stations      []Station       `json:"stations"`
	Exchangers    []Exchanger     `json:"exchangers"`
	Pumps         []Pump          `json:"pumps"`
	Valves        []Valve         `json:"valves"`
	Boundaries    []BoundaryValve `json:"boundaries"`
	Reliefs       []ReliefDevice  `json:"reliefs"`
}
