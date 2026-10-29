FROM golang:alpine AS build
WORKDIR /src
COPY go.mod *.go ./
RUN go vet ./... && go test ./... \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pause . \
    && mkdir /out/empty

FROM scratch
COPY --from=build /out/ /
CMD ["/pause"]
