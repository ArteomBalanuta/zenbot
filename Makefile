.DEFAULT_GOAL := build

fmt:
	go fmt ./...
.PHONY:fmt

lint: fmt
	golint ./...
.PHONY:lint

vet: fmt
	go vet ./...
.PHONY:vet

build: vet
	rm -rf ./target/
	mkdir ./target
	(cd main && go build -o ../target/)
.PHONY:build% 

run: 
	./target/main