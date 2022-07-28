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
		-package=mockproto \
		-destination=internal/mockproto/mux_mock.go \
		-source=internal/genproto/mux_grpc.pb.go MultiplexerServiceClient

.PHONY: test
test:
	go test -race ./...
.PHONY: lint
lint:
	golangci-lint run -v