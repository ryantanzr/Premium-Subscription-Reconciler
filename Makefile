.PHONY: up down test build tidy

up:
	docker-compose up -d --build

down:
	docker-compose down

test: tidy
	go test -v ./...

tidy:
	go mod tidy

build:
	go build -o bin/reconciler main.go

clean:
	rm -rf bin/
	docker-compose down -v