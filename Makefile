.PHONY: test short-test

test:
	go test -count=1 ./...

short-test:
	go test -short -count=1 ./...
