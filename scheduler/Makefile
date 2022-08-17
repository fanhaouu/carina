
IMAGE_REPOSITORY=harbor.shopeemobile.com/ecp
VERSION ?= 1.0
DATE=$(shell date '+%Y%m%d%H%M%S')
ARCH ?= linux/arm64,linux/amd64

# Run tests
test:
	go test -v ./... -coverprofile cover.out

# Build the docker image
docker-build:
	go mod vendor
	docker build -t $(IMAGE_REPOSITORY)/local-storage-csi-scheduler:$(VERSION) .
	rm -rf vendor

# Push the docker image
release:
	go mod vendor
	docker buildx build -t $(IMAGE_REPOSITORY)/local-storage-csi-scheduler:$(VERSION) --platform=$(ARCH) . --push
	rm -rf vendor

