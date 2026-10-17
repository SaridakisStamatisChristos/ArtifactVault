.PHONY: fmt vet test race build run migrate-up migrate-down
fmt:
	gofmt -w $$(find . -name '*.go' -type f)
vet:
	go vet ./...
test:
	go test ./...
race:
	go test -race ./...
build:
	mkdir -p bin && go build -trimpath -o bin/artifactvault ./cmd/artifactvault
run: build
	./bin/artifactvault serve
migrate-up: build
	./bin/artifactvault migrate up
migrate-down: build
	./bin/artifactvault migrate down
