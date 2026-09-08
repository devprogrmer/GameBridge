// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "net/http"

func (s *Server) registerAuthRoutes() {
	s.mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		_ = recordSecurityEvent("auth_me_access")

		jsonWrite(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})
}
