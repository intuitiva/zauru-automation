build:
	dep ensure -v
	env GOOS=linux go build -ldflags="-s -w" -o bin/get-due-clients-send-pymt-req get-due-clients-send-pymt-req/main.go

.PHONY: clean
clean:
	rm -rf ./bin ./vendor Gopkg.lock

.PHONY: deploy
deploy: clean build
	sls deploy --verbose
