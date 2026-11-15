.PHONY: build test lint vuln check check-qml test-ui test-service stage
build:
	mkdir -p bin
	CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/omarchy-sonos .
test:
	go test -race ./...
lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo 'gofmt: files need formatting' >&2; exit 1; }
	go mod tidy -diff
	go tool staticcheck ./...
vuln:
	go tool govulncheck ./...
check: lint test test-ui test-service
	omarchy plugin validate .
check-qml:
	./scripts/check-qml.sh
test-ui:
	./scripts/test-ui.sh
test-service:
	./scripts/test-service.sh
stage: build
	./scripts/stage-plugin.sh dist/sgruendel.sonos
