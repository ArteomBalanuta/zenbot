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
	rm -rf ./target/*
	go build -o ./target/ ./... 
.PHONY:build% 

run: 
	./target/zenbot

test:
	go test -v ./...
.PHONY: test