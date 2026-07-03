.PHONY: dev build test fmt check

dev:
	wails dev

build:
	wails build

test:
	go test ./...

fmt:
	gofmt -w .

check:
	cd frontend && npm run check
