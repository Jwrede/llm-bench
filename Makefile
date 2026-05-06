.PHONY: build discover clean

build:
	go build -o discover ./cmd/discover/

discover: build
	./discover -o probes.yml

clean:
	rm -f discover
