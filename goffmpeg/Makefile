# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=goforffmpeg
BINARY_UNIX=$(BINARY_NAME)_unix

LDFLAGS=-ldflags '-X=main.Version=$(VERSION) -X=main.Build=$(BUILD) -extldflags "-static"'

all: build
build: 
	$(GOBUILD) -o $(BINARY_NAME) -v
#test: 
#	$(GOTEST) -v ./...
clean: 
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
#run:
#	$(GOBUILD) -o $(BINARY_NAME) -v ./...
#	./$(BINARY_NAME)
deps:
	$(GOGET) "github.com/micro/go-config"
	$(GOGET) "github.com/micro/go-config/source/file"
	$(GOGET) "github.com/rs/zerolog/log"
	$(GOGET) "github.com/segmentio/kafka-go"

# Cross compilation
build-linux-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) $(LDFLAGS) -v
