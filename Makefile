.PHONY: proto
proto:
	cd api/proto && \
	protoc \
		--go_opt=Mmux.proto=github.com/bbralion/CTFloodBot/internal/genproto \
		--go-grpc_opt=Mmux.proto=github.com/bbralion/CTFloodBot/internal/genproto \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		--go_out=../../internal/genproto \
		--go-grpc_out=../../internal/genproto \
		mux.proto	

.PHONY: mocks
mocks:
	mockgen \
		-package=mocks \
		-destination=internal/mocks/mux_mock.go \
		github.com/bbralion/CTFloodBot/internal/genproto MultiplexerServiceClient,MultiplexerService_RegisterHandlerClient
	mockgen \
		-package mocks \
		-destination=internal/mocks/tgbotapi_mock.go \
		--mock_names HttpClient=MockTGBotAPIHTTPClient \
		github.com/go-telegram-bot-api/telegram-bot-api HttpClient

.PHONY: test
test:
	go test -v -race -count=1 ./...

.PHONY: cover
cover:
	go test -race -count=1 -covermode=atomic -coverprofile cover.tmp.out -coverpkg=./... -v ./... && \
	grep -v 'genproto\|mocks' cover.tmp.out > cover.out
	go tool cover -func cover.out && \
	go tool cover -html cover.out -o cover.html && \
	open cover.html && sleep 1 && \
	rm -f cover.tmp.out cover.out cover.html

.PHONY: lint
lint:
	golangci-lint run -v