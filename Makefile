.PHONY: test verify image compose

test:
	go test ./...

verify:
	gofmt -w cmd internal
	go test ./...
	go test -race ./...
	go vet ./...

image:
	docker build -t whatsmeow-mqtt-bridge .

compose:
	docker compose up --build
