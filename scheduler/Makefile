
IMAGE_REPOSITORY=harbor.shopeemobile.com/ecp/local-storage-csi-scheduler
VERSION ?= 1.0
DATE=$(shell date '+%Y%m%d%H%M%S')
ARCH ?= linux/amd64

# Run tests
test:
	go test -v ./... -coverprofile cover.out

# Build the docker image
docker-build:
	go mod vendor
	docker build -t $(IMAGE_REPOSITORY):$(VERSION) --platform=$(ARCH) .
	rm -rf vendor

# Push the docker image
release:
	docker push $(IMAGE_REPOSITORY):$(VERSION)
