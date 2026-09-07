# Contributing

1. Fork the repository.
2. Create a feature branch.
3. Run `gofmt -w .`.
4. Run `go test ./...`.
5. Keep transport-specific logic under `internal/transports/`.
6. Do not commit real IPs, PSKs, credentials, or private user configs.
7. Open a pull request explaining the network behavior and test conditions.
