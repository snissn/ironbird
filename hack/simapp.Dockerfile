ARG IMG_TAG=latest
ARG GO_IMAGE=golang:1.25-alpine

# Compile the simapp binary
FROM ${GO_IMAGE} AS simd-builder
ARG GIT_SHA
RUN echo "Ironbird building with SHA: $GIT_SHA"

WORKDIR /src/

ENV PACKAGES="curl make git libc-dev bash file build-base linux-headers eudev-dev"
RUN apk add --no-cache $PACKAGES

ARG CHAIN_TAG
ARG CHAIN_SRC=https://github.com/cosmos/cosmos-sdk
ARG REPLACE_CMD

RUN git clone $CHAIN_SRC /src/app && \
    cd /src/app && \
    git checkout $CHAIN_TAG

WORKDIR /src/app/simapp
RUN if [ -n "$REPLACE_CMD" ]; then \
	        go mod tidy && \
	        echo "After go mod tidy, applying module replacements:" && \
	        for replace in $REPLACE_CMD; do \
	            echo "go mod edit -replace ${replace}" && \
	            go mod edit -replace "$replace"; \
	        done && \
	        echo "Final go.mod:" && \
	        cat go.mod && \
        echo "Updating go.sum with replaced modules:" && \
        go get ./... && \
        echo "Done updating go.sum"; \
    else \
        go mod tidy; \
    fi
WORKDIR /src/app

RUN make build

FROM alpine:$IMG_TAG
RUN apk add --no-cache build-base jq
RUN addgroup -g 1025 nonroot
RUN adduser -D nonroot -u 1025 -G nonroot
ARG IMG_TAG
COPY --from=simd-builder  /src/app/build/simd /usr/bin/simd
EXPOSE 26656 26657 1317 9090 26660
USER nonroot

ENTRYPOINT ["simd", "start"]
