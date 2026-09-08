// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "net/http"

func (s *Server) registerDashboardRoutes() {
    s.mux.HandleFunc("/api/dashboard/stats", s.handleDashboardStats)
}

func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
    jsonWrite(w, http.StatusOK, map[string]any{
        "status": "ok",
    })
}
