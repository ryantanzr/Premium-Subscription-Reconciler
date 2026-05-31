.PHONY: up down test e2e build tidy

up:
	docker-compose up -d --build

down:
	docker-compose down

test: tidy
	go test -v ./...

e2e:
	bash scripts/e2e_test.sh

tidy:
	go mod tidy

build:
	go build -o bin/reconciler main.go

clean:
	rm -rf bin/
	docker-compose down -v