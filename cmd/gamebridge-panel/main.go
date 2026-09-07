// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"flag"
	"github.com/devprogrmer/GameBridge/internal/panel"
	"log"
	"os"
)

var version = "dev"

func main() {
	listen := flag.String("listen", env("GAMEBRIDGE_PANEL_LISTEN", "0.0.0.0:8088"), "listen")
	state := flag.String("state", env("GAMEBRIDGE_PANEL_STATE", "/var/lib/gamebridge/panel-state.json"), "state")
	sk := flag.String("session-key", env("GAMEBRIDGE_PANEL_SESSION_KEY", "/etc/gamebridge/panel-session.key"), "session key")
	mk := flag.String("master-key", env("GAMEBRIDGE_PANEL_MASTER_KEY", "/etc/gamebridge/panel-master.key"), "master key")
	secure := flag.Bool("cookie-secure", env("GAMEBRIDGE_COOKIE_SECURE", "") == "1", "secure cookie")
	flag.Parse()
	log.Printf("GameBridge Panel %s starting", version)
	s, e := panel.New(panel.Config{Listen: *listen, StatePath: *state, SessionKeyPath: *sk, MasterKeyPath: *mk, CookieSecure: *secure})
	if e != nil {
		log.Fatal(e)
	}
	log.Fatal(s.ListenAndServe())
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
