ARCH?=amd64
REPO?=#your repository here 
VERSION?=0.1

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -o ./bin/finops-composition-definition-parser main.go

container:
	docker build -t $(REPO)finops-composition-definition-parser:$(VERSION) .
	docker push $(REPO)finops-composition-definition-parser:$(VERSION)

container-multi:
	docker buildx build --tag $(REPO)finops-composition-definition-parser:$(VERSION) --push --platform linux/amd64,linux/arm64 .