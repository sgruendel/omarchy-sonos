.PHONY: build test check check-qml test-ui test-service stage
build:
	mkdir -p bin
	CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/omarchy-sonos .
test:
	go test -race ./...
check: test test-ui test-service
	go vet ./...
	omarchy plugin validate .
check-qml:
	./scripts/check-qml.sh
test-ui:
	./scripts/test-ui.sh
test-service:
	./scripts/test-service.sh
stage: build
	./scripts/stage-plugin.sh dist/sgruendel.sonos
