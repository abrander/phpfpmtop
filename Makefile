BINARY := phpfpmtop
TARGETS := \
	$(BINARY)_linux_amd64 \
	$(BINARY)_darwin_amd64 \

os = $(word 2, $(subst _, ,$@))
arch = $(word 3, $(subst _, ,$@))

# A bare 'make' should simply build the binary as expected.
$(BINARY): *.go
	go build -o '$@' .

# Build for all architectures.
$(TARGETS): *.go go.mod
	CGO_ENABLED=0 GOOS=$(os) GOARCH=$(arch) go build -o '$(BINARY)_$(os)_$(arch)' .

release: deps
	# We call make again to make sure to always execute the rule. We
	# need this because dependencies may have changed after 'deps'.
	$(MAKE) --always-make \
		--no-print-directory \
		clean \
		$(TARGETS)

deps: go.*
	go mod download
	go mod verify

clean:
	rm -f $(BINARY) $(TARGETS)
