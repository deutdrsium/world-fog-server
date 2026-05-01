.PHONY: build run test tidy docker docker-run

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/world-fog ./cmd/server

run:
	WF_JWT_SECRET=dev-secret-32-bytes-change-me \
	WF_APPLE_TEAM_ID=ABCDE12345 \
	WF_APPLE_BUNDLE_ID=cn.xuefz.worldfog \
	go run ./cmd/server --config configs/config.yaml

test:
	go test ./...

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

docker:
	docker build -t world-fog:latest .

docker-run:
	docker run -p 8443:8443 \
	  -e WF_JWT_SECRET=change-me-32-bytes-minimum \
	  -e WF_SERVER_TLS_CERT=/certs/cert.pem \
	  -e WF_SERVER_TLS_KEY=/certs/key.pem \
	  -e WF_APPLE_TEAM_ID=ABCDE12345 \
	  -e WF_APPLE_BUNDLE_ID=cn.xuefz.worldfog \
	  -v $(PWD)/certs:/certs \
	  -v $(PWD)/data:/var/lib/world-fog \
	  world-fog:latest
