IMAGE := crane-copy
CONTAINER := copy-debug

.PHONY: build run test stop clean

build:
	podman build -t $(IMAGE) .

run:
	podman run -d --name $(CONTAINER) -p 7070:8080 $(IMAGE)

test:
	curl -s -X POST http://localhost:7070/invoke \
		-H 'Content-Type: application/json' \
		-d @build/test.json | jq .

stop:
	podman stop $(CONTAINER)
clean: stop
	podman rm $(CONTAINER)

