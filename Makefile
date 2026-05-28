IMAGE := crane-copy
CONTAINER := copy-debug
TAG ?= latest
LOCAL_IMAGE := $(IMAGE):$(TAG)
ENGINE ?= podman

TCR_HOST := ccr.ccs.tencentyun.com
TCR_PREFIX := $(TCR_HOST)/service12/
REMOTE_IMAGE := $(TCR_PREFIX)$(LOCAL_IMAGE)

# Load registry credentials from .env if present.
# NOTE: Do NOT wrap values in quotes. Make's include treats quotes as literal
# characters, so REG_PWD='secret' would include the quotes in the password.
# Write plain values: REG_PWD=secret
-include .env
export

.PHONY: build run test stop clean login-tcr push-tcr

build:
	$(ENGINE) build -t $(LOCAL_IMAGE) .

run:
	$(ENGINE) run -d --name $(CONTAINER) -p 7070:8080 $(LOCAL_IMAGE)

test:
	curl -s -X POST http://localhost:7070/invoke \
		-H 'Content-Type: application/json' \
		-d @build/test.json

stop:
	$(ENGINE) stop $(CONTAINER)

clean: stop
	$(ENGINE) rm $(CONTAINER)

login-tcr:
	echo $$REG_PWD | $(ENGINE) login $(TCR_HOST) -u $$REG_USER --password-stdin

push-tcr: build login-tcr
	$(ENGINE) tag $(LOCAL_IMAGE) $(REMOTE_IMAGE)
	$(ENGINE) push $(REMOTE_IMAGE)
