package api

import "net/http"

// catalogHandler 返回当前生效的资产目录快照。
// 恢复、联锁、调度、运维页面通过同一端点读取同一版本目录。
func (a *App) catalogHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.Catalog.Snapshot())
}
