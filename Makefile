.PHONY: generate
generate:
	@go generate ./...
	@make format

.PHONY: format
format:
	go fmt ./...

.PHONY: update
update:
	@go get -u -t ./...

pkgs:=$(shell go list ./... | egrep -v '_test')
cover_pkgs:=$(shell echo $(pkgs) | tr " " ",")
.PHONY: test
test: format
	@go test $(test_args) -count=1 -failfast -coverprofile=coverage.out -coverpkg=$(cover_pkgs) $(pkgs)
	@go tool cover -func=coverage.out  | grep total: | awk '{ print $3 }'

.PHONY: race
race: format
	@go test $(test_args) -count=32 -failfast -p=8 ./...
%:
	@:
