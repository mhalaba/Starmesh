.PHONY: test build
test:
	go test ./...
build:
	mkdir -p dist
	go build -o dist/starmesh ./hub/cmd/starmesh
