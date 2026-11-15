module github.com/localitas/localitas-app-email

go 1.27.1

require (
	github.com/emersion/go-imap v1.2.1
	github.com/emersion/go-message v0.18.2
	github.com/localitas/localitas-go v0.1.12
	github.com/mattn/go-sqlite3 v1.14.47
	github.com/urfave/cli/v3 v3.9.1
)

replace github.com/localitas/localitas-go => ../localitas-go

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	honnef.co/go/tools v0.8.2 // indirect
)

tool honnef.co/go/tools/cmd/staticcheck
