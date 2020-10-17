# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=meta-go
BINARY_UNIX=$(BINARY_NAME)_static

LDFLAGS=-ldflags '-X=main.Version=$(VERSION) -X=main.Build=$(BUILD) -extldflags "-static"'

all: build
build: 
	make deps
	$(GOBUILD) -o $(BINARY_NAME) -v
clean: 
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
deps:
	$(GOGET) "github.com/micro/go-config"
	$(GOGET) "github.com/micro/go-config/source/file"
	$(GOGET) "github.com/minio/minio-go"
	$(GOGET) "github.com/rs/zerolog"
	$(GOGET) "github.com/rs/zerolog/log"
	$(GOGET) "github.com/segmentio/kafka-go"
	$(GOGET) "gopkg.in/avro.v0"

# Static compilation
build-linux-static:
	make deps
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) $(LDFLAGS) -v
