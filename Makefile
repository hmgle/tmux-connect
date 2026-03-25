APP := tmux-connect
CMD := ./cmd/tmux-connect

ALL_PLATFORMS := telegram feishu slack discord whatsapp weixin
COMMA := ,

_EXCLUDE_TAGS :=
EFFECTIVE_PLATFORMS := $(ALL_PLATFORMS)

ifneq ($(strip $(PLATFORMS_INCLUDE)),)
ifneq ($(strip $(EXCLUDE)),)
$(error EXCLUDE and PLATFORMS_INCLUDE are mutually exclusive)
endif
endif

ifdef PLATFORMS_INCLUDE
  _WANTED_PLATFORMS := $(subst $(COMMA), ,$(PLATFORMS_INCLUDE))
  _UNKNOWN_INCLUDE_PLATFORMS := $(filter-out $(ALL_PLATFORMS),$(_WANTED_PLATFORMS))
ifneq ($(_UNKNOWN_INCLUDE_PLATFORMS),)
$(error unknown platform(s) in PLATFORMS_INCLUDE: $(_UNKNOWN_INCLUDE_PLATFORMS))
endif
  _EXCLUDE_PLATFORMS := $(filter-out $(_WANTED_PLATFORMS),$(ALL_PLATFORMS))
  _EXCLUDE_TAGS += $(addprefix no_,$(_EXCLUDE_PLATFORMS))
  EFFECTIVE_PLATFORMS := $(_WANTED_PLATFORMS)
endif

ifdef EXCLUDE
  _EXCLUDE_PLATFORMS_DIRECT := $(subst $(COMMA), ,$(EXCLUDE))
  _UNKNOWN_EXCLUDE_PLATFORMS := $(filter-out $(ALL_PLATFORMS),$(_EXCLUDE_PLATFORMS_DIRECT))
ifneq ($(_UNKNOWN_EXCLUDE_PLATFORMS),)
$(error unknown platform(s) in EXCLUDE: $(_UNKNOWN_EXCLUDE_PLATFORMS))
endif
  _EXCLUDE_TAGS += $(addprefix no_,$(_EXCLUDE_PLATFORMS_DIRECT))
  EFFECTIVE_PLATFORMS := $(filter-out $(_EXCLUDE_PLATFORMS_DIRECT),$(ALL_PLATFORMS))
endif

BUILD_TAGS := $(strip $(_EXCLUDE_TAGS))
TAGS_FLAG := $(if $(BUILD_TAGS),-tags '$(BUILD_TAGS)',)
EFFECTIVE_PLATFORMS_DISPLAY := $(if $(strip $(EFFECTIVE_PLATFORMS)),$(EFFECTIVE_PLATFORMS),<none>)

.PHONY: build test clean print-tags help platforms print-platforms print-selected-platforms

help:
	@printf '%s\n' \
		'Targets:' \
		'  make build                                 Build all compiled-in platforms' \
		'  make build EXCLUDE=feishu,whatsapp         Exclude specific platforms via negative build tags' \
		'  make build PLATFORMS_INCLUDE=telegram      Keep only specific platforms' \
		'                                             EXCLUDE and PLATFORMS_INCLUDE are mutually exclusive' \
		'  make platforms                             List supported platforms without building' \
		'  make print-platforms                       Print supported platform names, one per line' \
		'  make print-selected-platforms              Print selected platform names, one per line' \
		'  make test                                  Run the default test suite' \
		'  make clean                                 Remove the built binary' \
		'  make print-tags                            Show the effective Go build tags'

build:
	go build $(TAGS_FLAG) -o $(APP) $(CMD)

test:
	go test ./...

clean:
	rm -f $(APP)

platforms:
	@printf '%s\n' \
		'Supported remote platforms:' \
		$(foreach platform,$(ALL_PLATFORMS),'  - $(platform)')
	@printf 'Selected for this invocation: %s\n' "$(EFFECTIVE_PLATFORMS_DISPLAY)"

print-platforms:
	@printf '%s\n' $(ALL_PLATFORMS)

print-selected-platforms:
	@printf '%s\n' $(EFFECTIVE_PLATFORMS)

print-tags:
	@printf 'BUILD_TAGS=%s\n' "$(BUILD_TAGS)"
