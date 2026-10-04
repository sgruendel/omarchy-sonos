.PHONY: build test check stage
build:
	mkdir -p bin
	CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/omarchy-sonos .
test:
	go test -race ./...
check: test
	go vet ./...
	omarchy plugin validate .
stage: build
	./scripts/stage-plugin.sh dist/sgruendel.sonos
