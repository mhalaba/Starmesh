.PHONY: test build flutter build-pi
test:
	go test ./...
build:
	mkdir -p dist
	go build -o dist/starmesh ./hub/cmd/starmesh
build-pi:
	mkdir -p dist
	GOOS=linux GOARCH=arm64 go build -o dist/starmesh-linux-arm64 ./hub/cmd/starmesh
flutter:
	cd app && flutter pub get && flutter analyze
