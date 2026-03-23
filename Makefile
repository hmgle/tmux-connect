APP := tmux-connect
CMD := ./cmd/tmux-connect

ALL_PLATFORMS := telegram feishu slack discord whatsapp
COMMA := ,

_EXCLUDE_TAGS :=

ifdef PLATFORMS_INCLUDE
  _WANTED_PLATFORMS := $(subst $(COMMA), ,$(PLATFORMS_INCLUDE))
  _EXCLUDE_PLATFORMS := $(filter-out $(_WANTED_PLATFORMS),$(ALL_PLATFORMS))
  _EXCLUDE_TAGS += $(addprefix no_,$(_EXCLUDE_PLATFORMS))
endif

ifdef EXCLUDE
  _EXCLUDE_TAGS += $(addprefix no_,$(subst $(COMMA), ,$(EXCLUDE)))
endif

BUILD_TAGS := $(strip $(_EXCLUDE_TAGS))
TAGS_FLAG := $(if $(BUILD_TAGS),-tags '$(BUILD_TAGS)',)

.PHONY: build test clean print-tags help

help:
	@printf '%s\n' \
		'Targets:' \
		'  make build                                 Build all compiled-in platforms' \
		'  make build EXCLUDE=feishu,whatsapp         Exclude specific platforms via negative build tags' \
		'  make build PLATFORMS_INCLUDE=telegram      Keep only specific platforms' \
		'  make test                                  Run the default test suite' \
		'  make clean                                 Remove the built binary' \
		'  make print-tags                            Show the effective Go build tags'

build:
	go build $(TAGS_FLAG) -o $(APP) $(CMD)

test:
	go test ./...

clean:
	rm -f $(APP)

print-tags:
	@printf 'BUILD_TAGS=%s\n' "$(BUILD_TAGS)"
