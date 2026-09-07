// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/devprogrmer/GameBridge/internal/config"
	"github.com/devprogrmer/GameBridge/internal/secure"
	"github.com/devprogrmer/GameBridge/internal/stats"
	"github.com/devprogrmer/GameBridge/internal/system"
	"github.com/devprogrmer/GameBridge/internal/transports/echogame"
	"github.com/devprogrmer/GameBridge/internal/transports/gamequic"
	"github.com/devprogrmer/GameBridge/internal/transports/pulseudp"
	"github.com/devprogrmer/GameBridge/internal/transports/swiftkcp"
	"github.com/devprogrmer/GameBridge/internal/tun"
)

var version = "dev"

func keygen() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	fmt.Println(hex.EncodeToString(b))
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "keygen":
			keygen()
			return
		case "version", "--version", "-version":
			fmt.Println("GameBridge", version)
			return
		}
	}

	configPath := flag.String("config", "/etc/gamebridge/default.json", "path to JSON config")
	flag.Parse()

	c, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	box, err := secure.New(c.KeyHex)
	if err != nil {
		log.Fatal(err)
	}

	dev, err := tun.Open(c.Interface)
	if err != nil {
		log.Fatalf("open TUN: %v", err)
	}
	defer dev.Close()

	if err := tun.Configure(c.Interface, c.LocalCIDR, c.PeerIP, c.MTU); err != nil {
		log.Fatalf("configure TUN: %v", err)
	}
	system.EnableNAT(c)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	st := stats.New(c.Name, c.Transport, c.Role)
	statusPath := filepath.Join("/run", "gamebridge-"+c.Name+".json")
	go st.WriteLoop(ctx, statusPath)

	log.Printf("GameBridge %s starting: name=%s transport=%s role=%s interface=%s mtu=%d",
		version, c.Name, c.Transport, c.Role, c.Interface, c.MTU)

	switch strings.ToLower(c.Transport) {
	case "pulseudp", "pulseudp-mp":
		err = pulseudp.Run(ctx, dev, c, box, st)
	case "kcp":
		err = swiftkcp.Run(ctx, dev, c, st)
	case "quic":
		err = gamequic.Run(ctx, dev, c, box, st)
	case "icmp":
		err = echogame.Run(ctx, dev, c, box, st)
	default:
		err = fmt.Errorf("unsupported transport: %s", c.Transport)
	}
	if err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
