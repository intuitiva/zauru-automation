build:
	dep ensure -v
	env GOOS=linux go build -ldflags="-s -w" -o bin/start start/main.go
	env GOOS=linux go build -ldflags="-s -w" -o bin/mail mail/main.go

.PHONY: clean
clean:
	rm -rf ./bin ./vendor Gopkg.lock

.PHONY: deploy
deploy: clean build
	sls deploy --verbose
