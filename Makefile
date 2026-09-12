.PHONY: test build flutter
test:
	go test ./...
build:
	mkdir -p dist
	go build -o dist/starmesh ./hub/cmd/starmesh
flutter:
	cd app && flutter pub get && flutter analyze
