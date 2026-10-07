package api

import "net/http"

// getCatalog 返回当前生效的资产目录（分区、分支、站点、换热器、泵、阀门、边界泄压设备）
// 及其版本。恢复、联锁、调度、运维页面统一从这里读取同一版本目录。
func (a *App) getCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.Catalog.Snapshot())
}
