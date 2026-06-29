.PHONY: server docker_build docker_run compose_up compose_down

server:
	go run ./cmd/server

docker_build: # build container image (override with `make docker_build DOCKER=podman`)
	$(DOCKER) build -t files-md --build-arg VERSION=$$(git rev-parse --short HEAD) .

docker_run: # run container, host 80 -> container 8080
	$(DOCKER) run --rm -it -p 80:8080 \
		-v files-md-storage:/app/storage \
		-v files-md-tokens:/app/tokens \
		-e APP_URL=http://localhost \
		-e STORAGE_DIR=/app/storage \
		-e TOKENS_DIR=/app/tokens \
		-e CERT_DIR= \
		files-md

compose_up: # build + start via compose.yaml, logs attached
	$(DOCKER) compose up --build

compose_down: # stop and remove the compose container (named volumes survive)
	$(DOCKER) compose down

DOCKER ?= docker

test:
	go test ./...

check:
	go fmt ./... && go vet ./... && go test ./...

lint:
	golangci-lint run

format:
	gofumpt -w .

e2e: # make e2e test="create and move"
	killall server || true
	go run ./cmd/server > /tmp/server_err 2>&1 & \
	cd tests && npm run test $(if $(test),-g "$(test)")

e2eh: # headed e2e tests
	killall server || true
	go run ./cmd/server & \
	cd tests && npm run test:headed $(if $(test),-g "$(test)")

e2es: # run single test, e2es test="name"
	cd tests && npm run test -- $(if $(test),-g "$(test)")

e2esh: # run single test headed, e2esh test="name"
	cd tests && npm run test:headed -- $(if $(test),-g "$(test)")

perf:
	cd tests && PERF=1 npm run test -- perf.spec.js

sync:
	killall server || true
	go run ./cmd/server & \
	cd tests && npm run test --g "sync"

synch:
	killall server || true
	go run ./cmd/server & \
	cd tests && npm run test:headed --g "sync"

report:
	cd tests && npx playwright show-report

define ENV_FILE
BOT_API_TOKEN=
API_HOST=$(apihost)
APP_HOST=app.files.md
STORAGE_DIR=/app/storage
CERT_DIR=/opt/files.md
TOKENS_DIR=/opt/files.md/tokens
LOG_FILE=/var/log/files.md/server.log
endef

define SERVICE_FILE
[Unit]
Description=Files.md Server
After=network.target

[Service]
User=www-data
ExecStart=/app/server
WorkingDirectory=/app
Environment=TOKENS_SALT=$(salt)
Restart=always
RestartSec=5
StandardOutput=append:/app/log
StandardError=append:/app/err
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
endef

# make init_server host=root@1.2.3.4 salt=my-secret-salt apihost=api.example.com
export ENV_FILE SERVICE_FILE
init_server: # create directories and configuration files on the server
	ssh $(host) 'sudo mkdir -p /app/storage /var/log/files.md /opt/files.md /opt/files.md/tokens && \
		sudo chown -R www-data:www-data /app /var/log/files.md /opt/files.md'
	echo "$$ENV_FILE" | ssh $(host) 'sudo tee /app/.env > /dev/null && sudo chown www-data:www-data /app/.env'
	echo "$$SERVICE_FILE" | ssh $(host) 'sudo tee /etc/systemd/system/filesmd.service > /dev/null'
	@echo 'Directories created and permissions set successfully.'

deploy_systemd: # deploy as systemd service
	@GREEN='\e[32m'; \
	YELLOW='\e[33m'; \
	RESET='\e[0m'; \
	COMMIT_HASH=$$(git rev-parse --short HEAD); \
	printf "$${YELLOW}Building (version=$${COMMIT_HASH})...$${RESET}\n" && \
	make check && \
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$${COMMIT_HASH}" -o /tmp/server ./cmd/server && \
	printf "$${GREEN}Build Completed$${RESET}\n" && \
	scp /tmp/server $(host):/tmp/server.new && printf "$${GREEN}The binary is copied on the server$${RESET}\n" && \
	ssh $(host) "sudo mv /tmp/server.new /app/server && sudo systemctl daemon-reload && sudo systemctl restart filesmd.service" && \
	rm /tmp/server && \
	printf "$${YELLOW}Version: $${COMMIT_HASH}$${RESET}\n" && \
	TMPWEB=$$(mktemp -d) && \
	cp -r web "$${TMPWEB}/web" && \
	find "$${TMPWEB}/web" -name "*.html" -exec grep -l "?v=" {} \; | xargs sed -i '' 's/?v=/?v='"$${COMMIT_HASH}"'/g' && \
	tar --no-xattrs --disable-copyfile --no-fflags -czf "$${TMPWEB}/web.tar.gz" -C "$${TMPWEB}" web && \
	scp "$${TMPWEB}/web.tar.gz" $(host):/app/ && \
	ssh $(host) "cd /app && tar -xzf web.tar.gz && rm web.tar.gz" && \
	rm -rf "$${TMPWEB}" && \
	printf "$${GREEN}Successfully deployed!$${RESET}\n"

deploy_binary: # deploy as regular binary, kinda deprecated, but ok for simple setup
	@GREEN='\e[32m'; \
	YELLOW='\e[33m'; \
	RESET='\e[0m'; \
	COMMIT_HASH=$$(git rev-parse --short HEAD); \
	printf "$${YELLOW}Building (version=$${COMMIT_HASH})...$${RESET}\n" && \
	make check && \
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$${COMMIT_HASH}" -o /tmp/server ./cmd/server && \
	printf "$${GREEN}Build Completed$${RESET}\n" && \
	ssh $(host) "sudo killall server || true" && \
	scp /tmp/server $(host):/tmp/server.new && printf "$${GREEN}The binary is copied on the server$${RESET}\n" && \
	ssh $(host) "sudo mv /tmp/server.new /app/server && sudo setcap 'cap_net_bind_service=+ep' /app/server" && \
	ssh $(host) "sudo su -c \"cd /app && nohup ./server >> /app/log 2>>/app/err &\" -s /bin/sh www-data" && \
	rm /tmp/server && \
	printf "$${GREEN}Successfully deployed!$${RESET}\n"

lite:
	go run .

lite_build: # current-OS binary, stripped
	go build -trimpath -ldflags "-s -w" -o lite .

lite_win: # Windows .exe, no console window (silent GUI)
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o lite.exe .

lite_win_console: # Windows .exe, keeps console for debugging
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o lite.exe .

