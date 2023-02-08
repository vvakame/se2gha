FROM golang:1.19.5 AS builder

ENV GOOS linux
ENV GOARCH amd64

WORKDIR /project

ENV GO111MODULE on

# for layer caching
COPY ./go.* ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 go build -o bin/server .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

COPY --from=builder /project/bin/server /bin/server

CMD ["/bin/server"]
