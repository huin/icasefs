all: icasefs

clean:
	@rm -f icasefs

fmt:
	@go fmt *.go

icasefs:
	@go build -o icasefs *.go

.PHONY: all clean fmt test
