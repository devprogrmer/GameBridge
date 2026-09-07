// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"flag"
	"github.com/devprogrmer/GameBridge/internal/agent"
	"log"
	"os"
)

var version = "dev"

func main() {
	listen := flag.String("listen", env("GAMEBRIDGE_AGENT_LISTEN", "0.0.0.0:8089"), "listen")
	token := flag.String("token", env("GAMEBRIDGE_AGENT_TOKEN", ""), "token")
	cert := flag.String("tls-cert", env("GAMEBRIDGE_AGENT_TLS_CERT", ""), "TLS cert")
	key := flag.String("tls-key", env("GAMEBRIDGE_AGENT_TLS_KEY", ""), "TLS key")
	flag.Parse()
	s, e := agent.New(agent.Config{Listen: *listen, Token: *token, TLSCert: *cert, TLSKey: *key, Version: version})
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
