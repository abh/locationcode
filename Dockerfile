FROM golang:1.21.0-alpine3.18 AS build
RUN apk --no-cache add git

WORKDIR /go/src/
ADD . /go/src/
RUN go install -v ./...


FROM alpine:3.18
USER root
RUN apk --no-cache add ca-certificates
RUN apk --no-cache upgrade

RUN addgroup lc && adduser -D -G lc lc
WORKDIR /
COPY --from=build /go/bin/locationcode  /bin/

USER lc

RUN mkdir /tmp/data

CMD ["/bin/locationcode", "-data-dir=/tmp/data"]
