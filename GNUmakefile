ifndef BINDIR
BINDIR := $(CURDIR)/build
else
BINDIR := $(abspath $(BINDIR))
endif
REPO=github.com/mcdexio/mai3-trade-mining-watcher
BUILDER_REPO=mai3-trade-mining-watcher/ci-base-alpine
GO_LDFLAGS=
ifeq ($(DOCKER),true)
GO_LDFLAGS += -linkmode external -extldflags \"-static\"
endif

V ?= 0
AT_LOCAL_GO    = $(AT_LOCAL_GO_$(V))
AT_LOCAL_GO_0  = @echo "  HOST GO    \n"$1;
AT_LOCAL_GO_1  =
AT_DOCKER_GO   = $(AT_DOCKER_GO_$(V))
AT_DOCKER_GO_0 = @echo "  DOCKER GO  \n"$1;
AT_DOCKER_GO_1 =
AT_RUN         = $(AT_RUN_$(V))
AT_RUN_0       = @echo "  RUN        \n"$@;
AT_RUN_1       =

define BUILD_RULE
$1: pre-build
ifeq ($(DOCKER),true)
	$(AT_DOCKER_GO)docker run --rm \
		-v "$(GOPATH)":/go:z \
		-v $(BINDIR):/artifacts:z \
		-e "GOPATH=/go" \
		-w /go/src/$(REPO) \
		$(BUILDER_REPO):latest sh -c "\
			go build $(GO_BUILD_OPTS) -o /artifacts/$1 --ldflags '$(GO_LDFLAGS)' $(REPO)/cmd/$1"
else
	$(AT_LOCAL_GO)go build $(GO_BUILD_OPTS) -o $(GOPATH)/bin/$1 -ldflags '$(GO_LDFLAGS)' $(REPO)/cmd/$1
	@install -c $(GOPATH)/bin/$1 $(BINDIR)
endif
endef

COMPONENTS = $(shell ls -d cmd/*/ | xargs -n 1 basename)

ALL_CMDS = \
	default \
	all \
	pre-build \
	clean \
	setup \
	remove \
	setup-database \
	remove-database

.PHONY: $(COMPONENTS) $(ALL_CMDS)

default: all

pre-build:
	@mkdir -p $(BINDIR)

all: $(COMPONENTS)

$(foreach component, $(COMPONENTS), $(eval $(call BUILD_RULE,$(component))))

clean:
	rm -f $(BINDIR)/*

setup: setup-database

remove: remove-database

setup-database:
	$(AT_DOCKER)if ! docker ps | grep postgres.mcdex; then \
		docker run --name postgres.mcdex \
			-p 5432:5432 \
			-d --tmpfs /run:rw,noexec,nosuid,size=512m \
			-e POSTGRES_DB=mcdex \
			-e POSTGRES_USER=mcdex \
			-e POSTGRES_PASSWORD=mcdex \
			-e POSTGRES_HOST_AUTH_METHOD=trust \
			--restart "always" \
			-d postgres:9.6 \
			-c fsync=off -c full_page_writes=off; \
		echo 'Waiting 10 sec for database to initialize ...'; \
		sleep 10; \
	fi

remove-database:
	$(AT_DOCKER)docker rm -f postgres.mcdex
