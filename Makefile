all: icasefs

clean:
	@rm -f icasefs

fmt:
	@go fmt *.go

icasefs: *.go
	@go build -o icasefs *.go

.PHONY: all clean fmt test
