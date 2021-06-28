export build = $(abspath ./build)

cmd=build

dst=$(addprefix $(build)/,$(cmd))

version=0.1

all: $(dst)

build_flags=-ldflags "-s -w -X main.VERSION=$(version)"

$(dst): $(shell find ./src -name \*.go) | $(build)
	cd ./src/; go build -o $(abspath $@) $(build_flags)

install: all
	sudo cp -t /usr/local/bin $(dst)

$(build):
	mkdir -p $@
