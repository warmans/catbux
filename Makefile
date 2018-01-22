.PHONY: build
build:
	go build -o build/node ./cmd/server

keys:
   mkdir -p .ecdsa && ssh-keygen -t ecdsa -o .ecdsa/id_ecdsa