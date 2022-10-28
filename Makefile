.PHONY: setup

GO = /usr/local/go/bin/go
SRC := $(wildcard pkg/tripplite)
VERSION = 1.0.0

all: dist/upsmon-server dist/upsmon-client

docker: $(SRC)
	docker build -t upsmon:$(VERSION) .

dist/upsmon-server: cmd/server/main.go $(SRC)
	mkdir -p dist
	(cd cmd/server; go build -o ../../$@ .)

dist/upsmon-client: cmd/client/main.go $(SRC)
	mkdir -p dist
	(cd cmd/client; go build -o ../../$@ .)

