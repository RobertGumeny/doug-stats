.PHONY: all build frontend-build test clean

all: build

frontend-build:
	cd frontend && npm ci && npm run build

build: frontend-build
	go build -o doug-stats .

test: frontend-build
	go test ./...

clean:
	rm -rf frontend/dist frontend/node_modules doug-stats
