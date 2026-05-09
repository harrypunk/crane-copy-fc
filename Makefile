IMAGE := crane-copy
CONTAINER := copy-debug
TAG ?= latest
LOCAL_IMAGE := $(IMAGE):$(TAG)

TCR_PREFIX := ccr.ccs.tencentyun.com/service12/
REMOTE_IMAGE := $(TCR_PREFIX)$(LOCAL_IMAGE)

.PHONY: build run test stop clean push-tcr

build:
	podman build -t $(LOCAL_IMAGE) .

run:
	podman run -d --name $(CONTAINER) -p 7070:8080 $(LOCAL_IMAGE)

test:
	curl -s -X POST http://localhost:7070/invoke \
		-H 'Content-Type: application/json' \
		-d @build/test.json

stop:
	podman stop $(CONTAINER)

clean: stop
	podman rm $(CONTAINER)

push-tcr: build
	podman tag $(LOCAL_IMAGE) $(REMOTE_IMAGE)
	podman push $(REMOTE_IMAGE)
