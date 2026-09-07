// SPDX-License-Identifier: AGPL-3.0-only
package gamequic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	"crypto/tls"
	"github.com/devprogrmer/GameBridge/internal/config"
	"github.com/devprogrmer/GameBridge/internal/secure"
	"github.com/devprogrmer/GameBridge/internal/stats"
	quic "github.com/quic-go/quic-go"
)

const kindData = 1

func tlsServer() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "GameBridge"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(3650 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"gamebridge/1"}}, nil
}

func qcfg() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: 3 * time.Second,
		MaxIdleTimeout:  30 * time.Second,
	}
}

func Run(ctx context.Context, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats) error {
	var conn quic.Connection
	if c.Role == "server" {
		tlsCfg, err := tlsServer()
		if err != nil {
			return err
		}
		addr := net.JoinHostPort(c.ListenHost, fmt.Sprint(c.Ports[0]))
		ln, err := quic.ListenAddr(addr, tlsCfg, qcfg())
		if err != nil {
			return err
		}
		defer ln.Close()
		conn, err = ln.Accept(ctx)
		if err != nil {
			return err
		}
	} else {
		addr := net.JoinHostPort(c.PeerHost, fmt.Sprint(c.Ports[0]))
		tlsCfg := &tls.Config{
			InsecureSkipVerify: true, // transport is also authenticated with the configured AEAD key
			NextProtos:         []string{"gamebridge/1"},
		}
		var err error
		conn, err = quic.DialAddr(ctx, addr, tlsCfg, qcfg())
		if err != nil {
			return err
		}
	}
	st.SetActivePath("quic/" + fmt.Sprint(c.Ports[0]))
	return bridge(ctx, tun, conn, box, st)
}

func bridge(ctx context.Context, tun *os.File, conn quic.Connection, box *secure.Box, st *stats.Stats) error {
	errCh := make(chan error, 2)
	var seq uint64

	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := tun.Read(buf)
			if err != nil {
				errCh <- err
				return
			}
			seq++
			frame, err := box.Seal(kindData, seq, buf[:n])
			if err != nil {
				continue
			}
			if err := conn.SendDatagram(frame); err != nil {
				errCh <- err
				return
			}
			st.TxPackets.Add(1)
			st.TxBytes.Add(uint64(n))
		}
	}()

	go func() {
		for {
			frame, err := conn.ReceiveDatagram(ctx)
			if err != nil {
				errCh <- err
				return
			}
			kind, _, payload, err := box.Open(frame)
			if err != nil || kind != kindData {
				continue
			}
			if _, err := tun.Write(payload); err != nil {
				errCh <- err
				return
			}
			st.RxPackets.Add(1)
			st.RxBytes.Add(uint64(len(payload)))
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}
