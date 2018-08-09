build:
	dep ensure
	env GOOS=linux go build -ldflags="-s -w" -o bin/service service/main.go
	sls deploy