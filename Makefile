IMAGE := crane-copy
CONTAINER := copy-debug

.PHONY: build run test stop clean push-tcr

build:
	podman build -t $(IMAGE) .

run:
	podman run -d --name $(CONTAINER) -p 7070:8080 $(IMAGE)

test:
	curl -s -X POST http://localhost:7070/invoke \
		-H 'Content-Type: application/json' \
		-d @build/test.json

stop:
	podman stop $(CONTAINER)
clean: stop
	podman rm $(CONTAINER)

TCR-URL := ccr.ccs.tencentyun.com/service12/$(IMAGE)
push-tcr:
	podman tag localhost/$(IMAGE) $(TCR-URL)
	podman push $(TCR-URL)
