// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "net/http"

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

type ReleaseInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Image     string `json:"image"`
}

func (s *Server) handleReleaseInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	jsonWrite(w, http.StatusOK, ReleaseInfo{
		Version:   version,
		Commit:    commit,
		BuildTime: buildTime,
		Image:     "gamebridge-panel",
	})
}
