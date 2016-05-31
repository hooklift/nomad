HOOKLIFT_VERSION=0.4.0-5
IMAGE=nomad:$(HOOKLIFT_VERSION)

hooklift-build:
	docker build --build-arg VERSION=$(HOOKLIFT_VERSION) -t $(IMAGE) .

hooklift-pkg: hooklift-clean hooklift-build
	id=$$(docker create $(IMAGE)) && \
	docker cp $$id:/work/src/github.com/hashicorp/nomad/bin/nomad bin/nomad \
	&& docker rm $$id && \
	cd bin && zip ../nomad.zip nomad

hooklift-release: hooklift-pkg
	@latest_tag=$$(git describe --tags `git rev-list --tags --max-count=1`); \
	comparison="$$latest_tag..HEAD"; \
	if [ -z "$$latest_tag" ]; then comparison=""; fi; \
	changelog=$$(git log $$comparison --oneline --no-merges --reverse); \
	github-release hooklift/nomad $(HOOKLIFT_VERSION) "$$(git rev-parse --abbrev-ref HEAD)" "**Changelog**<br/>$$changelog" 'nomad.zip'; \
	git pull downstream hooklift

hooklift-clean:
	rm -rf nomad.zip
