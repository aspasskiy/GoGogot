BIN   := gogogot
IMAGE := octagonlab/gogogot
TAG   := latest

.PHONY: run build clean lint generate docker-build docker-push docker-release deploy tag

# Refresh the embedded OpenRouter model catalog (network). Run on demand and
# commit the result; builds use the committed copy.
generate:
	curl -sf https://openrouter.ai/api/v1/models -o internal/llm/catalog/openrouter_models.json

run:
	go run ./cmd/gogogot $(ARGS)

build:
	go build -o $(BIN) ./cmd/gogogot

clean:
	rm -f $(BIN)

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...

docker-build:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE):$(TAG) -f deploy/Dockerfile .

docker-push:
	docker push $(IMAGE):$(TAG)

docker-release:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE):$(TAG) -f deploy/Dockerfile --push .

BUMP ?= patch

tag:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty, commit changes first"; exit 1; \
	fi
	$(eval LAST := $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0))
	$(eval MAJOR := $(shell echo $(LAST) | sed 's/^v//' | cut -d. -f1))
	$(eval MINOR := $(shell echo $(LAST) | sed 's/^v//' | cut -d. -f2))
	$(eval PATCH := $(shell echo $(LAST) | sed 's/^v//' | cut -d. -f3))
	$(eval NEXT := $(shell \
		if [ "$(BUMP)" = "major" ]; then echo v$$(($(MAJOR)+1)).0.0; \
		elif [ "$(BUMP)" = "minor" ]; then echo v$(MAJOR).$$(($(MINOR)+1)).0; \
		else echo v$(MAJOR).$(MINOR).$$(($(PATCH)+1)); fi))
	@echo "$(LAST) -> $(NEXT)"
	git tag -a $(NEXT) -m "Release $(NEXT)"
	git push origin $(NEXT)
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE):$(NEXT) -t $(IMAGE):latest \
		-f deploy/Dockerfile --push .

deploy:
	./deploy/deploy.sh
