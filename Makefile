.PHONY: build test vet fmt clean

build:
	go build -o gorrit ./cmd/gorrit

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -f gorrit
